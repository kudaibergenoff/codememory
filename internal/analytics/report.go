package analytics

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

// Row aggregates measured events for one UTC day, agent, channel and tool.
type Row struct {
	Day             string `json:"day"`
	Agent           string `json:"agent"`
	Channel         string `json:"channel"`
	Tool            string `json:"tool"`
	Requests        int    `json:"requests"`
	Errors          int    `json:"errors"`
	CacheHits       int    `json:"query_cache_hits"`
	CacheMisses     int    `json:"query_cache_misses"`
	ResponseBytes   int64  `json:"response_bytes"`
	EstimatedTokens int64  `json:"estimated_response_tokens"`
	LatencyUS       int64  `json:"total_latency_us"`
}

// Daily returns per-day aggregates, oldest first.
func Daily(ctx context.Context, db *sql.DB, f Filter) ([]Row, error) {
	where, args := f.where()
	rows, err := db.QueryContext(ctx, `SELECT date(at_ns/1000000000,'unixepoch') AS day,agent,channel,tool,count(*),coalesce(sum(failed),0),coalesce(sum(cache_hit=1),0),coalesce(sum(cache_hit=0),0),coalesce(sum(response_bytes),0),coalesce(sum(estimated_tokens),0),coalesce(sum(latency_us),0)
 FROM events WHERE `+where+` GROUP BY day,agent,channel,tool ORDER BY day,agent,channel,tool`, args...)
	if err != nil {
		return nil, fmt.Errorf("daily metrics: %w", err)
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.Day, &r.Agent, &r.Channel, &r.Tool, &r.Requests, &r.Errors, &r.CacheHits, &r.CacheMisses, &r.ResponseBytes, &r.EstimatedTokens, &r.LatencyUS); err != nil {
			return nil, fmt.Errorf("scan daily metrics: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

var csvHeader = []string{"day", "agent", "channel", "tool", "requests", "errors", "query_cache_hits", "query_cache_misses", "response_bytes", "estimated_response_tokens", "total_latency_us"}

// WriteCSV writes rows with a header. Estimated tokens use Estimator.
func WriteCSV(w io.Writer, rows []Row) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{r.Day, r.Agent, r.Channel, r.Tool, strconv.Itoa(r.Requests), strconv.Itoa(r.Errors), strconv.Itoa(r.CacheHits), strconv.Itoa(r.CacheMisses),
			strconv.FormatInt(r.ResponseBytes, 10), strconv.FormatInt(r.EstimatedTokens, 10), strconv.FormatInt(r.LatencyUS, 10)}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
