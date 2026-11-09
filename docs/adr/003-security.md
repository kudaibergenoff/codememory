# ADR-003: deny first, bounded local reads, explicit freshness

Accepted 2026-11-09. Canonical explicit root; reject home and filesystem root.
Skip symlinks and nonregular files. Root-scoped filesystem reads prevent escape
even when a source symlink is replaced between walk and open. Hard-denied paths
are rejected before content reads. Ignore files are bounded, nested and ordered;
unsupported ignore syntax must fail closed rather than silently include files.
Content secret checks are heuristic and omit the entire detected file.

No project code execution, shell commands, telemetry or networking in indexing
and MCP handlers. Source comments and README are untrusted data, never commands.
Queries use bound SQL and literal FTS tokens. File and aggregate limits abort a
scan without publishing a partial generation. Hash all eligible files: mtime
alone is not a correctness check.

The private state directory is checked for symlinks. SQLite opens paths itself:
hostile concurrent mutation of `.codememory` by another process with the same OS
identity is outside this first slice's isolation guarantee. Hard links and local
users with filesystem access also require OS-level protections. Windows ACL
hardening needs native validation. A repository root is a trust choice.

Freshness is a point-in-time scan, not a promise against future edits. Search
must not label an unchecked or mismatched index fresh. No filesystem snapshot
is claimed. Full scans trade latency for correctness in the first slice.
