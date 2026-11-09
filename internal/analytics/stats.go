package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Filter selects events at or after Since (zero: whole retention window) and,
// when Agent is set, from that agent only.
type Filter struct {
	Since time.Time
	Agent string
}

func (f Filter) where() (string, []any) {
	since := int64(0)
	if !f.Since.IsZero() {
		since = f.Since.UnixNano()
	}
	return `at_ns>=? AND (?='' OR agent=?)`, []any{since, f.Agent, f.Agent}
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
	// StaleInvalidations counts calls whose refresh re-indexed files.
	StaleInvalidations int `json:"stale_invalidations"`
	// RepeatedRequests counts calls identical to an earlier one in the window.
	RepeatedRequests  int   `json:"repeated_requests"`
	MetricsOverheadUS int64 `json:"metrics_overhead_us"`
}

type Estimated struct {
	Method         string `json:"method"`
	ResponseTokens int64  `json:"response_tokens"`
	Savings        *int64 `json:"token_savings"`
}

type Report struct {
	Since             string    `json:"since,omitempty"`
	Agent             string    `json:"agent,omitempty"`
	Measured          Measured  `json:"measured"`
	Estimated         Estimated `json:"estimated"`
	ActualUsage       *int64    `json:"actual_usage"`
	VerifiedSavings   *int64    `json:"verified_token_savings"`
	QueryCacheHitRate *float64  `json:"query_cache_hit_rate"`
	// MetricsOverheadPct is recording cost relative to the measured request latency.
	MetricsOverheadPct *float64 `json:"metrics_overhead_pct"`
	Note               string   `json:"note"`
}

const statsNote = "Local events only (30-day retention, at most 10000). null means unavailable. File reuse is avoided parsing and cache hits are avoided retrieval work; neither is token savings. Response tokens exclude client prompts and protocol envelopes."

// Stats aggregates measured events. Actual usage and verified savings stay
// null: no supported client exposes per-request usage to local tools.
func Stats(ctx context.Context, db *sql.DB, f Filter) (Report, error) {
	r := Report{Agent: f.Agent, Estimated: Estimated{Method: Estimator}, Note: statsNote}
	if !f.Since.IsZero() {
		r.Since = f.Since.UTC().Format(time.RFC3339)
	}
	where, args := f.where()
	m := &r.Measured
	err := db.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(channel='mcp'),0),coalesce(sum(failed),0),coalesce(sum(response_bytes),0),coalesce(sum(latency_us),0),coalesce(sum(indexed_files),0),coalesce(sum(reused_files),0),coalesce(sum(estimated_tokens),0),coalesce(sum(cache_hit=1),0),coalesce(sum(cache_hit=0),0),coalesce(sum(invalidated),0),coalesce(sum(request_hash<>''),0)-count(DISTINCT nullif(request_hash,'')),coalesce(sum(overhead_us),0) FROM events WHERE `+where, args...).
		Scan(&m.Requests, &m.MCPRequests, &m.Errors, &m.ResponseBytes, &m.LatencyUS, &m.IndexedFiles, &m.ReusedFiles, &r.Estimated.ResponseTokens, &m.CacheHits, &m.CacheMisses, &m.StaleInvalidations, &m.RepeatedRequests, &m.MetricsOverheadUS)
	if err != nil {
		return r, fmt.Errorf("aggregate metrics: %w", err)
	}
	if lookups := m.CacheHits + m.CacheMisses; lookups > 0 {
		rate := float64(m.CacheHits) / float64(lookups)
		r.QueryCacheHitRate = &rate
	}
	if m.LatencyUS > 0 {
		pct := float64(m.MetricsOverheadUS) / float64(m.LatencyUS) * 100
		r.MetricsOverheadPct = &pct
	}
	return r, nil
}
