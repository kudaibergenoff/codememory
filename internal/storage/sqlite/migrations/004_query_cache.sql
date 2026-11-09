-- Response cache bound to one index generation (SPEC §5 query_cache). Times are
-- Unix nanoseconds so comparisons are numeric, not string-ordered.
CREATE TABLE query_cache (
 cache_key TEXT PRIMARY KEY, generation INTEGER NOT NULL, response TEXT NOT NULL,
 created_at_ns INTEGER NOT NULL, expires_at_ns INTEGER NOT NULL
);
-- NULL for tools without a cache, 1 hit, 0 miss.
ALTER TABLE events ADD COLUMN cache_hit INTEGER;
