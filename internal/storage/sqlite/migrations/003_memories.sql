-- Curated architecture memories with provenance (SPEC §5, §7 memory_remember).
-- Status is the state at write time; staleness is derived on read by comparing
-- sources_json hashes with the current index.
CREATE TABLE memories (
 id INTEGER PRIMARY KEY, scope TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL,
 sources_json TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('sourced','unverified')),
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(scope, title)
);
-- Bumped on every memory write so caches that include memories can be keyed on it.
ALTER TABLE project ADD COLUMN memory_revision INTEGER NOT NULL DEFAULT 0;
