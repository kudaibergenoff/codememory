package qcache

import (
	"context"
	"testing"
	"time"

	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

type payload struct {
	Value string `json:"value"`
}

func store(t *testing.T) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(context.Background(), t.TempDir(), "p")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestKeyDependsOnEveryPart(t *testing.T) {
	a := Key("memory_search", map[string]any{"query": "pay"}, int64(1))
	if a != Key("memory_search", map[string]any{"query": "pay"}, int64(1)) {
		t.Fatal("key not deterministic")
	}
	for _, other := range []string{
		Key("memory_context", map[string]any{"query": "pay"}, int64(1)),
		Key("memory_search", map[string]any{"query": "pays"}, int64(1)),
		Key("memory_search", map[string]any{"query": "pay"}, int64(2)),
	} {
		if other == a {
			t.Fatal("distinct inputs share a key")
		}
	}
}

func TestKeyDependsOnResultsVersion(t *testing.T) {
	parts := []any{map[string]any{"query": "pay"}}

	if Key("memory_search", parts...) != keyFor(ResultsVersion, "memory_search", parts...) {
		t.Fatal("Key must use the current results version")
	}
	if keyFor(ResultsVersion, "memory_search", parts...) == keyFor(ResultsVersion-1, "memory_search", parts...) {
		t.Fatal("entries from an older ranking would be served")
	}
}

func TestHitOnlyForSameGenerationBeforeExpiry(t *testing.T) {
	ctx := context.Background()
	s := store(t)
	c := Cache{Store: s, TTL: time.Hour, MaxEntries: 10}
	if err := c.Put(ctx, "k", 3, payload{"v"}); err != nil {
		t.Fatal(err)
	}

	var got payload
	if hit, err := c.Get(ctx, "k", 3, &got); err != nil || !hit || got.Value != "v" {
		t.Fatalf("same generation: hit=%v %+v %v", hit, got, err)
	}
	if hit, _ := c.Get(ctx, "k", 4, &got); hit {
		t.Fatal("hit across generations")
	}
	expired := Cache{Store: s, TTL: -time.Second, MaxEntries: 10}
	if err := expired.Put(ctx, "old", 3, payload{"x"}); err != nil {
		t.Fatal(err)
	}
	if hit, _ := c.Get(ctx, "old", 3, &got); hit {
		t.Fatal("expired entry served")
	}
}

func TestPutPrunesOtherGenerationsAndCapsEntries(t *testing.T) {
	ctx := context.Background()
	s := store(t)
	c := Cache{Store: s, TTL: time.Hour, MaxEntries: 3}
	if err := c.Put(ctx, "old-gen", 1, payload{"a"}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"a", "b", "c", "d"} {
		if err := c.Put(ctx, k, 2, payload{k}); err != nil {
			t.Fatal(err)
		}
	}

	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM query_cache`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("entries %d %v", n, err)
	}
	var got payload
	if hit, _ := c.Get(ctx, "a", 2, &got); hit {
		t.Fatal("oldest entry not evicted")
	}
}
