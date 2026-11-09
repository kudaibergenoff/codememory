package contextpack

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

func fakeMatches(files, perFile, lines int) []sqlite.Match {
	var out []sqlite.Match
	for f := 0; f < files; f++ {
		for c := 0; c < perFile; c++ {
			start := c*lines + 1
			var body []string
			for l := 0; l < lines; l++ {
				body = append(body, fmt.Sprintf("\tif x < %d && y > %d { return \"line %d\" }", l, c, l))
			}
			out = append(out, sqlite.Match{
				Source:  sqlite.Source{Path: fmt.Sprintf("pkg/file%02d.go", f), Start: start, End: start + lines - 1, Hash: strings.Repeat("a", 64)},
				Symbol:  fmt.Sprintf("Func%d", c),
				Snippet: strings.Join(body, "\n"),
				Score:   -float64(files*perFile - f*perFile - c),
			})
		}
	}
	return out
}

func baseFor(budget int) Response {
	return Response{Envelope: search.NewEnvelope("/tmp/project"), Terms: []string{"refund", "payment"}, TokenBudget: budget, Estimator: analytics.Estimator}
}

func mustPack(t *testing.T, base Response, candidates []sqlite.Match, budget int) Response {
	t.Helper()
	r, err := pack(base, candidates, nil, budget)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func encodedTokens(t *testing.T, r Response) int {
	t.Helper()
	b, err := search.Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	return analytics.Estimate(len(b))
}

func TestPackNeverExceedsBudgetIncludingJSON(t *testing.T) {
	candidates := fakeMatches(12, 4, 40)
	for _, budget := range []int{minTokens, 400, 750, 1500, 3000, maxTokens} {
		got := mustPack(t, baseFor(budget), candidates, budget)

		actual := encodedTokens(t, got)
		if actual > budget || got.EstimatedTokens > budget || got.EstimatedTokens < actual {
			t.Fatalf("budget %d: encoded %d, reported %d", budget, actual, got.EstimatedTokens)
		}
		if len(got.Snippets) == 0 {
			t.Fatalf("budget %d: no snippets packed", budget)
		}
		if len(got.Sources) != len(got.Snippets) {
			t.Fatalf("sources %d != snippets %d", len(got.Sources), len(got.Snippets))
		}
	}
}

func TestPackTruncatesInsteadOfDroppingTopResult(t *testing.T) {
	candidates := fakeMatches(1, 1, 200)

	got := mustPack(t, baseFor(minTokens), candidates, minTokens)

	if len(got.Snippets) != 1 || !got.Snippets[0].Truncated {
		t.Fatalf("expected one truncated snippet: %+v", got.Snippets)
	}
	s := got.Snippets[0]
	if lines := strings.Count(s.Snippet, "\n") + 1; s.End-s.Start+1 != lines {
		t.Fatalf("line range %d-%d does not match %d lines", s.Start, s.End, lines)
	}
}

func TestPackCapsSnippetsPerFileAndCountsOmitted(t *testing.T) {
	candidates := fakeMatches(2, 5, 3)

	got := mustPack(t, baseFor(maxTokens), candidates, maxTokens)

	perFile := map[string]int{}
	for _, s := range got.Snippets {
		perFile[s.Path]++
	}
	for path, n := range perFile {
		if n > maxPerFile {
			t.Fatalf("%s has %d snippets", path, n)
		}
	}
	if got.Omitted != len(candidates)-len(got.Snippets) {
		t.Fatalf("omitted %d, want %d", got.Omitted, len(candidates)-len(got.Snippets))
	}
	if len(got.Files) != 2 || got.Files[0] != "pkg/file00.go" {
		t.Fatalf("files %v", got.Files)
	}
}

func TestPackDropsOverlappingChunks(t *testing.T) {
	m := fakeMatches(1, 1, 10)[0]
	overlap := m
	overlap.Start, overlap.End = m.Start+2, m.End+2

	got := mustPack(t, baseFor(maxTokens), []sqlite.Match{m, overlap}, maxTokens)

	if len(got.Snippets) != 1 {
		t.Fatalf("overlapping chunk kept: %d", len(got.Snippets))
	}
}

func TestTaskTerms(t *testing.T) {
	cases := []struct {
		task string
		want []string
	}{
		{"Add a refund endpoint for payments", []string{"refund", "endpoint", "payments"}},
		{"добавь endpoint оплаты", []string{"endpoint", "оплаты"}},
		{"fix the RefundPayment handler", []string{"refundpayment", "handler"}},
		{"add the new", []string{"add", "the", "new"}}, // only stopwords: keep them
	}
	for _, c := range cases {
		got, err := taskTerms(c.task)
		if err != nil || strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%q: got %v %v, want %v", c.task, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "  !!! ", "\xff", strings.Repeat("x ", maxTaskBytes)} {
		if _, err := taskTerms(bad); err == nil {
			t.Errorf("accepted %q", bad[:min(len(bad), 10)])
		}
	}
}

func TestPackRejectsBudgetSmallerThanEnvelope(t *testing.T) {
	base := baseFor(minTokens)
	base.Project = strings.Repeat("\x01", 400) // 6 bytes each once JSON-escaped

	if _, err := pack(base, fakeMatches(1, 1, 5), nil, minTokens); err == nil {
		t.Fatal("over-budget envelope accepted")
	}
}
