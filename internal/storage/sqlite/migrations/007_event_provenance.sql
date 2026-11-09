-- Spec §11.1 event fields: configuration version, whether the call re-indexed
-- files (invalidating cached responses), a keyed request fingerprint for repeat
-- counting (never the request text) and the metric write's own cost.
ALTER TABLE events ADD COLUMN config_version TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN invalidated INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN request_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN overhead_us INTEGER NOT NULL DEFAULT 0;
-- Per-database random key: fingerprints cannot be matched against guessed queries.
ALTER TABLE project ADD COLUMN metrics_key TEXT NOT NULL DEFAULT '';
UPDATE project SET metrics_key = lower(hex(randomblob(16)));
