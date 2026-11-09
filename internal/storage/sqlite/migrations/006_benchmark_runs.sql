-- Last benchmark reports for dashboard and optimize (bounded on write).
CREATE TABLE benchmark_runs (
 id INTEGER PRIMARY KEY, at_ns INTEGER NOT NULL, suite TEXT NOT NULL, report TEXT NOT NULL
);
