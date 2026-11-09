// Package analytics records local measurements without retaining queries or source text.
package analytics

import (
	"context"
	"database/sql"
	"time"
)

const Estimator = "utf8-bytes-div-3-ceil-v1"

// Estimate is a size proxy, not a provider tokenizer or a guaranteed upper bound.
func Estimate(bytes int) int { return (bytes + 2) / 3 }

type Event struct {
	Channel, Tool   string
	Generation      int64
	ResponseBytes   int
	Duration        time.Duration
	Failed          bool
	Indexed, Reused int
	CacheHit        *bool // nil for tools without a query cache
}

func Record(ctx context.Context, db *sql.DB, e Event) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO events(at,channel,tool,generation,response_bytes,estimated_tokens,latency_us,failed,indexed_files,reused_files,cache_hit) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, time.Now().UTC().Format(time.RFC3339Nano), e.Channel, e.Tool, e.Generation, e.ResponseBytes, Estimate(e.ResponseBytes), e.Duration.Microseconds(), e.Failed, e.Indexed, e.Reused, e.CacheHit)
	if err != nil {
		return err
	}
	// Bound local telemetry even for frequently called MCP tools.
	if _, err = tx.ExecContext(ctx, `DELETE FROM events WHERE at < ? OR id <= (SELECT coalesce(max(id),0)-10000 FROM events)`, time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

type Measured struct {
	Requests      int   `json:"requests"`
	MCPRequests   int   `json:"mcp_requests"`
	Errors        int   `json:"errors"`
	ResponseBytes int64 `json:"response_bytes"`
	LatencyUS     int64 `json:"total_latency_us"`
	IndexedFiles  int   `json:"indexed_files"`
	ReusedFiles   int   `json:"unchanged_files_reused"`
	CacheHits     int   `json:"query_cache_hits"`
	CacheMisses   int   `json:"query_cache_misses"`
}
type Estimated struct {
	Method         string `json:"method"`
	ResponseTokens int64  `json:"response_tokens"`
	Savings        *int64 `json:"token_savings"`
}
type Report struct {
	Measured          Measured  `json:"measured"`
	Estimated         Estimated `json:"estimated"`
	ActualUsage       *int64    `json:"actual_usage"`
	VerifiedSavings   *int64    `json:"verified_token_savings"`
	QueryCacheHitRate *float64  `json:"query_cache_hit_rate"`
	Note              string    `json:"note"`
}

func Stats(ctx context.Context, db *sql.DB) (Report, error) {
	r := Report{Estimated: Estimated{Method: Estimator}, Note: "Last 30 days, capped at 10000 events. null means unavailable. File reuse is avoided parsing and cache hits are avoided retrieval work; neither is token savings. Response tokens exclude client prompts and protocol envelopes."}
	err := db.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(channel='mcp'),0),coalesce(sum(failed),0),coalesce(sum(response_bytes),0),coalesce(sum(latency_us),0),coalesce(sum(indexed_files),0),coalesce(sum(reused_files),0),coalesce(sum(estimated_tokens),0),coalesce(sum(cache_hit=1),0),coalesce(sum(cache_hit=0),0) FROM events WHERE at>=?`, time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339Nano)).Scan(&r.Measured.Requests, &r.Measured.MCPRequests, &r.Measured.Errors, &r.Measured.ResponseBytes, &r.Measured.LatencyUS, &r.Measured.IndexedFiles, &r.Measured.ReusedFiles, &r.Estimated.ResponseTokens, &r.Measured.CacheHits, &r.Measured.CacheMisses)
	if lookups := r.Measured.CacheHits + r.Measured.CacheMisses; err == nil && lookups > 0 {
		rate := float64(r.Measured.CacheHits) / float64(lookups)
		r.QueryCacheHitRate = &rate
	}
	return r, err
}
