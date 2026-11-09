package analytics

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

func TestAgentFromClientName(t *testing.T) {
	cases := map[string]string{"claude-code": AgentClaude, "Claude Code": AgentClaude, "codex-mcp-client": AgentCodex, "": AgentUnknown, "cursor": AgentUnknown}
	for name, want := range cases {
		if got := AgentFrom(name); got != want {
			t.Errorf("%q: got %s, want %s", name, got, want)
		}
	}
}

func seeded(t *testing.T) (context.Context, *sqlite.Store, time.Time) {
	t.Helper()
	ctx := context.Background()
	s, err := sqlite.Open(ctx, t.TempDir(), "p")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	now := time.Now().UTC()
	hit := true
	events := []Event{
		{At: now.Add(-40 * 24 * time.Hour), Channel: "mcp", Tool: "memory_search", Agent: AgentClaude, ResponseBytes: 300},
		{At: now.Add(-2 * time.Hour), Channel: "mcp", Tool: "memory_search", Agent: AgentClaude, ResponseBytes: 30, CacheHit: &hit},
		{At: now.Add(-1 * time.Hour), Channel: "mcp", Tool: "memory_context", Agent: AgentCodex, ResponseBytes: 60, Failed: true},
		{At: now.Add(-25 * time.Hour), Channel: "cli", Tool: "index", Agent: AgentUnknown},
	}
	for _, e := range events {
		if err := Record(ctx, s.DB, e); err != nil {
			t.Fatal(err)
		}
	}
	return ctx, s, now
}

func TestStatsFiltersByWindowAndAgent(t *testing.T) {
	ctx, s, now := seeded(t)
	cases := []struct {
		name   string
		filter Filter
		want   int
	}{
		{"last 30 days", Filter{Since: now.Add(-30 * 24 * time.Hour)}, 3},
		{"last day", Filter{Since: now.Add(-24 * time.Hour)}, 2},
		{"claude only", Filter{Since: now.Add(-30 * 24 * time.Hour), Agent: AgentClaude}, 1},
		{"codex only", Filter{Since: now.Add(-30 * 24 * time.Hour), Agent: AgentCodex}, 1},
	}
	for _, c := range cases {
		r, err := Stats(ctx, s.DB, c.filter)
		if err != nil || r.Measured.Requests != c.want {
			t.Errorf("%s: requests %d (want %d) %v", c.name, r.Measured.Requests, c.want, err)
		}
	}
}

func TestDailyReportJSONAndCSV(t *testing.T) {
	ctx, s, now := seeded(t)

	rows, err := Daily(ctx, s.DB, Filter{Since: now.Add(-30 * 24 * time.Hour)})

	if err != nil || len(rows) != 3 {
		t.Fatalf("rows %+v %v", rows, err)
	}
	var buf bytes.Buffer
	if err = WriteCSV(&buf, rows); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil || len(records) != 4 || records[0][0] != "day" {
		t.Fatalf("csv %v %v", records, err)
	}
	for _, r := range rows {
		if r.Tool == "memory_context" && (r.Errors != 1 || r.Agent != AgentCodex || r.Day != now.Add(-time.Hour).Format(time.DateOnly)) {
			t.Fatalf("context row %+v", r)
		}
	}
}

func TestRetentionUsesNumericTime(t *testing.T) {
	ctx, s, _ := seeded(t)
	// Each Record prunes events older than the retention window.
	cutoff := time.Now().Add(-retention).UnixNano()
	var stale int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE at_ns < ?`, cutoff).Scan(&stale); err != nil || stale != 0 {
		t.Fatalf("events older than retention kept: %d %v", stale, err)
	}
}

func TestEventProvenanceRepeatsAndOverhead(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.Open(ctx, t.TempDir(), "p")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	key := strings.Repeat("ab", 32)
	for _, e := range []Event{
		{Channel: "mcp", Tool: "memory_search", ConfigVersion: "cfg1", RequestKey: key, Invalidated: true, Duration: time.Millisecond},
		{Channel: "mcp", Tool: "memory_search", ConfigVersion: "cfg1", RequestKey: key, Duration: time.Millisecond},
		{Channel: "mcp", Tool: "memory_status", ConfigVersion: "cfg1", Duration: time.Millisecond},
	} {
		if err := Record(ctx, s.DB, e); err != nil {
			t.Fatal(err)
		}
	}

	r, err := Stats(ctx, s.DB, Filter{})

	m := r.Measured
	if err != nil || m.RepeatedRequests != 1 || m.StaleInvalidations != 1 || m.MetricsOverheadUS <= 0 || r.MetricsOverheadPct == nil {
		t.Fatalf("%+v %v", r, err)
	}
	var stored, version string
	if err := s.DB.QueryRow(`SELECT request_hash,config_version FROM events WHERE request_hash<>'' LIMIT 1`).Scan(&stored, &version); err != nil {
		t.Fatal(err)
	}
	if stored == key || strings.Contains(key, stored) || len(stored) != 32 || version != "cfg1" {
		t.Fatalf("request fingerprint must be keyed, not the raw key: %q %q", stored, version)
	}
}
