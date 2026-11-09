// Package benchmark measures retrieval quality and response size against a
// lexical baseline. It estimates; it never claims billed-token savings.
package benchmark

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/contextpack"
	"github.com/kudaibergenoff/codememory/internal/indexer"
	"github.com/kudaibergenoff/codememory/internal/scanner"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const (
	KindPositive    = "positive"
	KindNoMatch     = "no_match"
	KindStaleMemory = "memory_status"

	method = "Per task: memory_context response (estimated tokens of the encoded JSON) vs. a lexical baseline that reads every permitted file containing a task term in full, and an oracle that reads only the expected files. Savings are estimated as (baseline - context) / baseline * 100. Quality is recall of expected files among returned files and snippets. Client prompts, tool overhead and follow-up reads are not included."
	note   = "Estimates from local response sizes, not billed tokens. verified_token_savings needs comparable client usage data and stays null."
)

type Result struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Pass           bool     `json:"pass"`
	Recall         *float64 `json:"recall,omitempty"`
	Found          []string `json:"found,omitempty"`
	Missed         []string `json:"missed,omitempty"`
	Snippets       int      `json:"snippets"`
	ContextTokens  int      `json:"context_tokens"`
	BaselineTokens int      `json:"baseline_tokens"`
	BaselineFiles  int      `json:"baseline_files"`
	OracleTokens   int      `json:"oracle_tokens"`
	SavingsPct     *float64 `json:"estimated_savings_pct"`
	LatencyMS      float64  `json:"latency_ms"`
	CacheHit       bool     `json:"cache_hit"`
}

type Summary struct {
	Tasks            int      `json:"tasks"`
	Passed           int      `json:"passed"`
	MeanRecall       *float64 `json:"mean_recall"`
	MedianSavingsPct *float64 `json:"median_estimated_savings_pct"`
	ContextTokens    int      `json:"context_tokens"`
	BaselineTokens   int      `json:"baseline_tokens"`
	OracleTokens     int      `json:"oracle_tokens"`
}

type Report struct {
	Suite           string   `json:"suite"`
	Commit          string   `json:"commit,omitempty"`
	IndexGeneration int64    `json:"index_generation"`
	Method          string   `json:"method"`
	Estimator       string   `json:"estimator"`
	Results         []Result `json:"results"`
	Summary         Summary  `json:"summary"`
	VerifiedSavings *int64   `json:"verified_token_savings"`
	Note            string   `json:"note"`
}

// RunSuite refreshes the index and evaluates every task against root.
func RunSuite(ctx context.Context, s *sqlite.Store, root string, cfg config.Config, suite Suite, name string) (Report, error) {
	refreshed, err := indexer.Index(ctx, s, root, cfg)
	if err != nil {
		return Report{}, fmt.Errorf("refresh index: %w", err)
	}
	snap, err := scanner.Scan(ctx, root, cfg, nil)
	if err != nil {
		return Report{}, err
	}
	corpus := make(map[string]string, len(snap.Files))
	for _, f := range snap.Files {
		corpus[f.Path] = strings.ToLower(string(f.Content))
	}
	report := Report{Suite: name, Commit: gitHead(root), IndexGeneration: refreshed.Generation, Method: method, Estimator: analytics.Estimator, Results: []Result{}, Note: note}
	for _, t := range suite.Tasks {
		r, err := runTask(ctx, s, root, cfg, corpus, t)
		if err != nil {
			return Report{}, fmt.Errorf("task %s: %w", t.ID, err)
		}
		report.Results = append(report.Results, r)
	}
	report.Summary = summarize(report.Results)
	return report, nil
}

func runTask(ctx context.Context, s *sqlite.Store, root string, cfg config.Config, corpus map[string]string, t Task) (Result, error) {
	terms, err := contextpack.Terms(t.Task)
	if err != nil {
		return Result{}, err
	}
	started := time.Now()
	resp, err := contextpack.Build(ctx, s, root, cfg, contextpack.Input{Task: t.Task, MaxTokens: t.MaxTokens})
	if err != nil {
		return Result{}, err
	}
	r := Result{ID: t.ID, LatencyMS: float64(time.Since(started).Microseconds()) / 1000, Snippets: len(resp.Snippets)}
	if hit := resp.CacheHit(); hit != nil {
		r.CacheHit = *hit
	}
	payload, err := search.Encode(resp)
	if err != nil {
		return Result{}, err
	}
	r.ContextTokens = analytics.Estimate(len(payload))
	r.BaselineTokens, r.BaselineFiles = baseline(corpus, terms)
	for _, p := range t.ExpectFiles {
		r.OracleTokens += analytics.Estimate(len(corpus[p]))
	}
	judge(&r, t, resp)
	return r, nil
}

// judge sets kind, quality and pass for the task's single expectation.
func judge(r *Result, t Task, resp contextpack.Response) {
	switch {
	case t.ExpectNoMatch:
		r.Kind, r.Pass = KindNoMatch, len(resp.Snippets) == 0
	case t.ExpectMemoryStatus != "":
		r.Kind = KindStaleMemory
		for _, m := range resp.Architecture {
			r.Pass = r.Pass || m.Status == t.ExpectMemoryStatus
		}
	default:
		r.Kind = KindPositive
		returned := map[string]bool{}
		for _, f := range resp.Files {
			returned[f] = true
		}
		for _, sn := range resp.Snippets {
			returned[sn.Path] = true
		}
		for _, p := range t.ExpectFiles {
			if returned[p] {
				r.Found = append(r.Found, p)
			} else {
				r.Missed = append(r.Missed, p)
			}
		}
		recall := float64(len(r.Found)) / float64(len(t.ExpectFiles))
		r.Recall, r.Pass = &recall, len(r.Missed) == 0
		r.SavingsPct = savingsPct(r.BaselineTokens, r.ContextTokens)
	}
}

// baseline estimates reading every file that contains any term in full.
func baseline(corpus map[string]string, terms []string) (tokens, files int) {
	for _, content := range corpus {
		for _, term := range terms {
			if strings.Contains(content, term) {
				tokens += analytics.Estimate(len(content))
				files++
				break
			}
		}
	}
	return tokens, files
}

// savingsPct is (baseline - actual) / baseline * 100; nil without a baseline.
// Negative values mean the context was larger than the baseline.
func savingsPct(baseline, actual int) *float64 {
	if baseline <= 0 {
		return nil
	}
	v := float64(baseline-actual) / float64(baseline) * 100
	return &v
}

func summarize(results []Result) Summary {
	var s Summary
	var recalls, savings []float64
	for _, r := range results {
		s.Tasks++
		if r.Pass {
			s.Passed++
		}
		s.ContextTokens += r.ContextTokens
		s.BaselineTokens += r.BaselineTokens
		s.OracleTokens += r.OracleTokens
		if r.Recall != nil {
			recalls = append(recalls, *r.Recall)
		}
		if r.SavingsPct != nil {
			savings = append(savings, *r.SavingsPct)
		}
	}
	if len(recalls) > 0 {
		mean := 0.0
		for _, v := range recalls {
			mean += v / float64(len(recalls))
		}
		s.MeanRecall = &mean
	}
	if len(savings) > 0 {
		sort.Float64s(savings)
		median := savings[len(savings)/2]
		if len(savings)%2 == 0 {
			median = (savings[len(savings)/2-1] + median) / 2
		}
		s.MedianSavingsPct = &median
	}
	return s
}

var commitSHA = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

const (
	maxRefBytes    = 256
	maxPackedBytes = 1 << 20
)

// gitHead returns the checked-out commit without running git, or "". It reads
// only regular files inside .git through os.Root, accepts refs under refs/ and
// returns nothing that is not a commit hash, so a crafted HEAD cannot leak or
// block on other files.
func gitHead(root string) string {
	r, err := os.OpenRoot(filepath.Join(root, ".git"))
	if err != nil {
		return ""
	}
	defer r.Close()
	head := readSmall(r, "HEAD", maxRefBytes)
	ref, isRef := strings.CutPrefix(head, "ref: ")
	if !isRef {
		return validSHA(head)
	}
	if !strings.HasPrefix(ref, "refs/") || strings.Contains(ref, "..") || path.Clean(ref) != ref {
		return ""
	}
	if sha := validSHA(readSmall(r, ref, maxRefBytes)); sha != "" {
		return sha
	}
	for _, line := range strings.Split(readSmall(r, "packed-refs", maxPackedBytes), "\n") {
		if sha, name, ok := strings.Cut(line, " "); ok && name == ref {
			return validSHA(sha)
		}
	}
	return ""
}

func validSHA(s string) string {
	if commitSHA.MatchString(s) {
		return s
	}
	return ""
}

// readSmall returns a regular file's trimmed content, or "" if it is missing,
// not regular or larger than limit.
func readSmall(r *os.Root, name string, limit int64) string {
	info, err := r.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return ""
	}
	f, err := r.Open(name)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
