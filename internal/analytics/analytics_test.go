package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

func TestRecord(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.Open(ctx, t.TempDir(), "p")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = Record(ctx, s.DB, Event{Channel: "mcp", Tool: "memory_search", ResponseBytes: 10, Duration: time.Millisecond, Reused: 3}); err != nil {
		t.Fatal(err)
	}
	r, err := Stats(ctx, s.DB)
	if err != nil {
		t.Fatal(err)
	}
	if r.Measured.MCPRequests != 1 || r.Estimated.ResponseTokens != 4 || r.VerifiedSavings != nil || r.Estimated.Savings != nil {
		t.Fatalf("%+v", r)
	}
}

func TestCacheHitRateIsNullWithoutCacheEvents(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.Open(ctx, t.TempDir(), "p")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = Record(ctx, s.DB, Event{Channel: "cli", Tool: "index"}); err != nil {
		t.Fatal(err)
	}
	if r, _ := Stats(ctx, s.DB); r.QueryCacheHitRate != nil {
		t.Fatalf("rate without cache events: %v", *r.QueryCacheHitRate)
	}
	hit, miss := true, false
	for _, c := range []*bool{&hit, &miss, &miss, &miss} {
		if err = Record(ctx, s.DB, Event{Channel: "mcp", Tool: "memory_search", CacheHit: c}); err != nil {
			t.Fatal(err)
		}
	}

	r, err := Stats(ctx, s.DB)

	if err != nil || r.QueryCacheHitRate == nil || *r.QueryCacheHitRate != 0.25 || r.Measured.CacheHits != 1 || r.Measured.CacheMisses != 3 {
		t.Fatalf("%+v %v", r, err)
	}
}
