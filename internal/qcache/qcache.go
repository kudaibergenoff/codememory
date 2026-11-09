// Package qcache caches encoded tool responses per index generation. A hit
// saves local retrieval work; it does not reduce the tokens a client receives.
package qcache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kudaibergenoff/codememory/internal/scanner"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const (
	defaultTTL        = time.Hour
	defaultMaxEntries = 500
)

// ResultsVersion is part of every key. Bump it whenever ranking or response
// shaping changes, so an upgraded binary never serves entries computed by
// the old logic for an unchanged index generation.
// 2: compound identifiers (memory_context) weight exact phrase matches.
const ResultsVersion = 2

type Cache struct {
	Store      *sqlite.Store
	TTL        time.Duration
	MaxEntries int
}

func New(s *sqlite.Store) Cache {
	return Cache{Store: s, TTL: defaultTTL, MaxEntries: defaultMaxEntries}
}

// Key hashes the tool name and every input that shapes its response. The
// index generation is not part of the key; entries are bound to it on Put.
func Key(tool string, parts ...any) string { return keyFor(ResultsVersion, tool, parts...) }

func keyFor(version int, tool string, parts ...any) string {
	b, err := json.Marshal(append([]any{version, tool}, parts...))
	if err != nil {
		// Inputs are plain structs. Should one ever fail to encode, use a
		// key that can never match again rather than risk a shared entry.
		b = []byte(fmt.Sprintf("%s:uncacheable:%d", tool, time.Now().UnixNano()))
	}
	return scanner.Hash(b)
}

// Get decodes the entry for key into dst when it belongs to generation.
func (c Cache) Get(ctx context.Context, key string, generation int64, dst any) (bool, error) {
	payload, ok, err := c.Store.CacheGet(ctx, key, generation, time.Now().UnixNano())
	if err != nil || !ok {
		return false, err
	}
	if err = json.Unmarshal(payload, dst); err != nil {
		return false, fmt.Errorf("decode cached response: %w", err)
	}
	return true, nil
}

// Put stores v for key at generation.
func (c Cache) Put(ctx context.Context, key string, generation int64, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode response for cache: %w", err)
	}
	now := time.Now()
	return c.Store.CachePut(ctx, key, generation, payload, now.UnixNano(), now.Add(c.TTL).UnixNano(), c.MaxEntries)
}
