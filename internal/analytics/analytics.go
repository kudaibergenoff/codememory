// Package analytics records local measurements without retaining queries or source text.
package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const Estimator = "utf8-bytes-div-3-ceil-v1"

const (
	retention = 30 * 24 * time.Hour
	maxEvents = 10000

	AgentClaude  = "claude"
	AgentCodex   = "codex"
	AgentUnknown = "unknown"
)

// Estimate is a size proxy, not a provider tokenizer or a guaranteed upper bound.
func Estimate(bytes int) int { return (bytes + 2) / 3 }

// AgentFrom maps an MCP clientInfo name to a coarse agent category. The raw
// name is never stored.
func AgentFrom(clientName string) string {
	name := strings.ToLower(clientName)
	switch {
	case strings.Contains(name, "claude"):
		return AgentClaude
	case strings.Contains(name, "codex"):
		return AgentCodex
	default:
		return AgentUnknown
	}
}

type Event struct {
	At              time.Time // zero means now
	Channel, Tool   string
	Agent           string // AgentClaude, AgentCodex or AgentUnknown; empty means unknown
	Generation      int64
	ResponseBytes   int
	Duration        time.Duration
	Failed          bool
	Indexed, Reused int
	CacheHit        *bool // nil for tools without a query cache
}

// Record stores one event and prunes events beyond the retention window or count.
func Record(ctx context.Context, db *sql.DB, e Event) error {
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	agent := e.Agent
	if agent == "" {
		agent = AgentUnknown
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin metric write: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO events(at,at_ns,agent,channel,tool,generation,response_bytes,estimated_tokens,latency_us,failed,indexed_files,reused_files,cache_hit) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		at.UTC().Format(time.RFC3339Nano), at.UnixNano(), agent, e.Channel, e.Tool, e.Generation, e.ResponseBytes, Estimate(e.ResponseBytes), e.Duration.Microseconds(), e.Failed, e.Indexed, e.Reused, e.CacheHit)
	if err != nil {
		return fmt.Errorf("insert metric: %w", err)
	}
	// Bound local telemetry even for frequently called MCP tools.
	if _, err = tx.ExecContext(ctx, `DELETE FROM events WHERE at_ns < ? OR id <= (SELECT coalesce(max(id),0)-? FROM events)`, time.Now().Add(-retention).UnixNano(), maxEvents); err != nil {
		return fmt.Errorf("prune metrics: %w", err)
	}
	return tx.Commit()
}
