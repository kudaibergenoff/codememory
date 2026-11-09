package analytics

import (
	"codememory/internal/storage/sqlite"
	"context"
	"testing"
	"time"
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
