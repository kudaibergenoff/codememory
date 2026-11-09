package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kudaibergenoff/codememory/internal/analytics"
)

// maxWindow matches the analytics retention: older events no longer exist.
const maxWindow = 30 * 24 * time.Hour

// window holds the shared stats/report selection flags.
type window struct {
	today  bool
	last   string
	agent  string
	format string
}

func (w window) filter(now time.Time) (analytics.Filter, error) {
	switch w.agent {
	case "", analytics.AgentClaude, analytics.AgentCodex, analytics.AgentUnknown:
	default:
		return analytics.Filter{}, fmt.Errorf("--agent must be claude, codex or unknown")
	}
	f := analytics.Filter{Agent: w.agent}
	if w.today && w.last != "" {
		return f, errors.New("use either --today or --last")
	}
	if w.today {
		f.Since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}
	if w.last != "" {
		d, err := parseLast(w.last)
		if err != nil {
			return f, err
		}
		f.Since = now.Add(-d)
	}
	return f, nil
}

// parseLast accepts Go durations plus a day suffix, e.g. 30d, 12h, 90m.
func parseLast(s string) (time.Duration, error) {
	var d time.Duration
	var err error
	if days, ok := strings.CutSuffix(s, "d"); ok {
		var n int
		n, err = strconv.Atoi(days)
		if n < 1 || n > int(maxWindow/(24*time.Hour)) {
			n = -1 // rejected below; never multiply an unchecked count
		}
		d = time.Duration(n) * 24 * time.Hour
	} else {
		d, err = time.ParseDuration(s)
	}
	if err != nil || d <= 0 || d > maxWindow {
		return 0, fmt.Errorf("--last must be a positive window up to 30d, e.g. 7d or 12h")
	}
	return d, nil
}

// rawOutput is written as-is instead of being JSON-encoded.
type rawOutput []byte

type dailyReport struct {
	Basis           string          `json:"basis"`
	Estimator       string          `json:"estimator"`
	Since           string          `json:"since,omitempty"`
	Agent           string          `json:"agent,omitempty"`
	Rows            []analytics.Row `json:"rows"`
	VerifiedSavings *int64          `json:"verified_token_savings"`
}

const reportBasis = "Measured local events per UTC day, agent, channel and tool. Estimated tokens are a size proxy of responses only; verified savings need comparable usage data, which clients do not expose, so they are null."

func report(ctx context.Context, db *sql.DB, w window) (any, error) {
	f, err := w.filter(time.Now())
	if err != nil {
		return nil, err
	}
	if w.format != "json" && w.format != "csv" {
		return nil, errors.New("--format must be json or csv")
	}
	rows, err := analytics.Daily(ctx, db, f)
	if err != nil {
		return nil, err
	}
	if w.format == "csv" {
		var buf bytes.Buffer
		if err := analytics.WriteCSV(&buf, rows); err != nil {
			return nil, err
		}
		return rawOutput(bytes.TrimSuffix(buf.Bytes(), []byte("\n"))), nil
	}
	r := dailyReport{Basis: reportBasis, Estimator: analytics.Estimator, Agent: f.Agent, Rows: rows}
	if !f.Since.IsZero() {
		r.Since = f.Since.UTC().Format(time.RFC3339)
	}
	return r, nil
}
