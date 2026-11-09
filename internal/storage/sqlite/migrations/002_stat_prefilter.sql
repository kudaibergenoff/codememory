-- Size/mtime prefilter: rows from version 1 get mtime_ns 0 (unsettled), so the
-- next scan re-reads each file once and records a trusted stat.
ALTER TABLE files ADD COLUMN mtime_ns INTEGER NOT NULL DEFAULT 0;
-- Files excluded by content inspection (binary, secrets); cached so unchanged
-- ones are not re-read on every freshness check.
CREATE TABLE skipped_files (
 path TEXT PRIMARY KEY, size_bytes INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 reason TEXT NOT NULL CHECK(reason IN ('content','secret'))
);
