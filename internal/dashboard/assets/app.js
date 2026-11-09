"use strict";
// All values come from /api/summary on this machine. Text is set with
// textContent only: paths and task ids are untrusted data.
function el(tag, text, cls) {
  const e = document.createElement(tag);
  if (text !== undefined && text !== null) e.textContent = String(text);
  if (cls) e.className = cls;
  return e;
}
function fmt(v, digits) {
  if (v === null || v === undefined) return "N/A";
  if (typeof v === "number") return v.toLocaleString(undefined, { maximumFractionDigits: digits ?? 0 });
  return String(v);
}
function tile(parent, label, value, kind) {
  const t = el("div", null, "tile");
  t.append(el("div", label, "label"), el("div", value, "value " + (value === "N/A" ? "na" : "")));
  if (kind) t.append(el("span", kind, "tag " + kind));
  parent.append(t);
}
function table(node, headers, rows) {
  node.replaceChildren();
  if (rows.length === 0) { node.append(el("caption", "No data yet.", "empty")); return; }
  const head = el("tr");
  headers.forEach(([h, cls]) => head.append(el("th", h, cls)));
  node.append(head);
  rows.forEach(r => {
    const tr = el("tr");
    r.forEach((v, i) => tr.append(el("td", v, headers[i][1])));
    node.append(tr);
  });
}
function render(d) {
  document.getElementById("project").textContent = "· " + d.project;
  document.getElementById("note").textContent = d.note;
  const s = d.stats, m = s.measured, tiles = document.getElementById("tiles");
  tiles.replaceChildren();
  tile(tiles, "Requests", fmt(m.requests));
  tile(tiles, "MCP requests", fmt(m.mcp_requests));
  tile(tiles, "Errors", fmt(m.errors));
  tile(tiles, "Query cache hit rate", s.query_cache_hit_rate === null ? "N/A" : fmt(s.query_cache_hit_rate * 100, 1) + "%");
  tile(tiles, "Response tokens", fmt(s.estimated.response_tokens), "estimated");
  tile(tiles, "Verified token savings", fmt(s.verified_token_savings), "na");
  table(document.getElementById("daily"),
    [["Day", "text"], ["Agent", "text"], ["Tool", "text"], ["Requests"], ["Errors"], ["Cache hits"], ["Est. tokens"]],
    d.daily.map(r => [r.day, r.agent, r.tool, fmt(r.requests), fmt(r.errors), fmt(r.query_cache_hits), fmt(r.estimated_response_tokens)]));
  const b = d.benchmark;
  document.getElementById("bench-note").textContent = b ? b.note + " Suite: " + b.suite + (b.commit ? " @ " + b.commit.slice(0, 7) : "") : "No benchmark yet. Run: codememory benchmark";
  table(document.getElementById("bench"),
    [["Task", "text"], ["Kind", "text"], ["Pass", "text"], ["Recall"], ["Context"], ["Baseline"], ["Oracle"], ["Est. savings"]],
    b ? b.results.map(r => [r.id, r.kind, r.pass ? "yes" : "no", r.recall === undefined ? "N/A" : fmt(r.recall, 2), fmt(r.context_tokens), fmt(r.baseline_tokens), fmt(r.oracle_tokens), r.estimated_savings_pct === null ? "N/A" : fmt(r.estimated_savings_pct, 1) + "%"]) : []);
  const f = d.footprint, st = document.getElementById("state");
  st.replaceChildren();
  tile(st, "Indexed files", fmt(f.files));
  tile(st, "Chunks", fmt(f.chunks));
  tile(st, "Memories", fmt(f.memories));
  tile(st, "Database size", fmt(f.database_bytes / 1048576, 1) + " MiB");
}
fetch("/api/summary", { cache: "no-store" })
  .then(r => { if (!r.ok) throw new Error("HTTP " + r.status); return r.json(); })
  .then(render)
  .catch(e => { document.getElementById("note").textContent = "Could not load measurements: " + e.message; });
