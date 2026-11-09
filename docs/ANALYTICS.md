# Analytics

Collection is local SQLite only. No full queries, source snippets, credentials,
client conversation history or provider usage are stored in metric events.

Measured: command/tool counts, MCP count, response JSON payload bytes, handler
latency, failures, parsed files and unchanged files reused by incremental index.
Request counts cover handlers reached after SDK schema validation; malformed
protocol frames, unknown tools and pre-handler validation failures are not counted.
Events expire after 30 days, with a 10000-event cap. Stats apply the time filter
even if the last collection happened more than 30 days ago.

Estimated: ceil(UTF-8 response bytes / 3), method `utf8-bytes-div-3-ceil-v1`.
This is a replaceable size proxy, not a tokenizer, not a guaranteed upper bound.
It excludes JSON-RPC wrappers, tool definitions, prompts and further file reads.
Token savings are null until an explicit comparable baseline exists.

Actual usage and verified token savings are null (display as N/A): a local MCP
server does not receive provider usage from an ordinary tool call. No undocumented
client logs are scraped. No currency conversion is made for subscription plans.
Repeated responses still cost context; file cache reuse is avoided work only.
Query cache, context efficiency, agent/session attribution, CSV/dashboard and
reproducible provider-usage comparisons remain on the roadmap.

`query_cache_hits`, `query_cache_misses` and `query_cache_hit_rate` count lookups by
`memory_search`/`memory_context`; the rate is `null` until a lookup happens. A hit
avoids local retrieval work only: the client still receives the same bytes, so it
is not token savings.

Events carry a numeric timestamp (`at_ns`, schema v5; older rows are backfilled
from the text column) and an agent category derived from MCP `clientInfo`
(`claude`, `codex`, otherwise `unknown`; CLI calls are `unknown`). Raw client
names are not stored. `stats` and `report` accept `--today` or `--last <=30d`
and `--agent`; `report --format csv` writes one row per UTC day, agent, channel
and tool. Verified savings remain `null` in every output.
