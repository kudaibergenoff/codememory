-- Numeric event time: RFC3339Nano strings drop trailing zeros, so string
-- comparison misorders values within one second. Backfill from the text column.
ALTER TABLE events ADD COLUMN at_ns INTEGER NOT NULL DEFAULT 0;
-- Unparsable text yields 0 (pruned by retention) instead of failing the migration.
UPDATE events SET at_ns = COALESCE(CAST(round((julianday(at) - 2440587.5) * 86400000.0) AS INTEGER) * 1000000, 0);
CREATE INDEX events_at_ns ON events(at_ns);
-- Calling agent category (claude, codex, unknown) from MCP clientInfo; raw
-- client names are not stored.
ALTER TABLE events ADD COLUMN agent TEXT NOT NULL DEFAULT 'unknown';
