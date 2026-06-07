# Web viewer gaps vs. the Python original — analysis & implementation plan

Status: **plan only** (for a follow-up session to implement). No viewer code is
changed by this document.

## TL;DR

Our `graph.html` (`internal/export/html.go`) renders a colored, community-clustered
force-directed graph and little else. The Python original
([safishamsi/graphify](https://github.com/safishamsi/graphify), `graphify/export.py`,
`to_html()`) ships a genuinely usable explorer: a sidebar with **search**, a
**click-to-inspect detail panel with clickable neighbors**, **confidence-aware edge
styling**, and **neighborhood highlighting**. Our version is "a picture of the graph";
theirs is "a tool for navigating the graph."

The good news: every piece of data the richer viewer needs is *already* in our
`model.Graph` and `graph.json` (`relation`, `confidence`/`confidence_score`,
`file_type`, `source_location`, degree, directed neighbors). The gap is entirely in
`internal/export/html.go` — the viewer throws this data away instead of surfacing it.
**This is a presentation-layer fix; no changes to extract/build/cluster/analyze are
required.**

---

## What the Python web view does

From `graphify/export.py` `to_html()` (a custom vis.js renderer; they dropped pyvis):

- **Layout:** full-screen canvas + a fixed ~280px **right sidebar** with four
  stacked panels: search, node info, community legend, stats.
- **Search box (live):** filters nodes by label as you type, shows a dropdown of the
  top ~20 matches; clicking a result focuses + selects that node and clears the box.
- **Inspect / info panel (on node click):** shows `label`, `type` (`file_type`),
  `community name`, `source_file`, `degree`, and a list of **clickable neighbors** —
  each neighbor rendered with a left border in its own community color; clicking a
  neighbor re-focuses the graph and updates the panel. This is the headline feature:
  you navigate the graph from the panel.
- **Community legend + filter:** color dot + member count per community; clicking a
  legend row hides/shows that community (dims the row to 0.35 opacity).
- **Node sizing:** `size = 10 + 30*(deg/maxDeg)`; labels hidden (`font 0`) below
  `0.15*maxDeg`, shown on hover. (We already match this.)
- **Edge styling encodes confidence:** `EXTRACTED` → solid, width 2, opacity 0.7;
  `INFERRED`/`AMBIGUOUS` → **dashed**, width 1, opacity 0.35. Edge tooltips show
  relation + confidence.
- **Selection highlighting:** clicking a node highlights it and its connected edges
  and dims the rest (neighborhood focus), rather than isolating a whole community.
- **Physics:** forceAtlas2Based, `gravitationalConstant -60`, `springLength 120`,
  `springConstant 0.08`, `avoidOverlap 0.8`, `stabilization {iterations:200,
  fit:true}`, then physics off. (We already match this almost exactly.)
- **Safety cap:** hard-fails above 5,000 nodes (`ValueError`) — they never aggregate;
  they always render node-level up to the cap.

## What our Go viewer does today

`internal/export/html.go`:

- Two modes: node-level below `metaThreshold` (500) and an **aggregated community
  meta-graph** above it (one circle per community). The meta view is a *Go-only* idea
  and is reasonable for huge graphs, but there's no drill-down from a meta circle to
  its nodes (already noted in `GOALS.md` → Viewer).
- Node-level: degree sizing + hover title (`source_file source_location`), community
  color, a legend with show/hide checkboxes.
- **Click a node → isolate its whole community** (hide everything else). There is **no
  detail panel**, **no search**, **no neighbor navigation**.
- Edges are **uniform** `width: 1`, single color, no confidence styling, no tooltips —
  `vedge` only carries `{from, to, width}`, so `relation`/`confidence` never reach the
  page even though `model.Edge` has them.
- `vnode` carries `{id,label,title,color,size,font,comm}` — no `file_type`, no degree,
  no separated source fields, no search key.

## Gap table

| # | Capability | Python | Go today | Data already present? |
|---|------------|:------:|:--------:|:---------------------:|
| 1 | Live node search box | ✅ | ❌ | yes (`label`/`norm_label`) |
| 2 | Click-to-inspect detail panel | ✅ | ❌ | yes |
| 3 | Clickable neighbors in panel | ✅ | ❌ | yes (`adj`/`Edges`) |
| 4 | Neighbors grouped by relation (calls/called-by/imports/contains) | partial | ❌ | yes (`relation` + direction) — *we can beat the original here; mirrors our `explain` CLI* |
| 5 | Confidence-aware edge styling (dashed/width/opacity) | ✅ | ❌ | yes (`confidence`) |
| 6 | Edge tooltips (relation + confidence) | ✅ | ❌ | yes |
| 7 | Neighborhood highlight-on-select (dim others) | ✅ | ⚠️ isolates whole community instead | yes |
| 8 | Stats panel (node/edge/community counts) | ✅ | ⚠️ banner only in meta mode | yes |
| 9 | Node `file_type` + degree shown to user | ✅ | ❌ (degree only used for sizing) | yes |
| 10 | Community legend with filter | ✅ | ✅ | — |
| 11 | Degree sizing + hover labels | ✅ | ✅ | — |
| 12 | Meta-graph for very large graphs | ❌ (hard cap 5000) | ✅ | — (Go extra; keep) |

Items **1–3** are the core of "the Go version is lacking." 4–9 are the polish that
makes the panel and canvas actually informative.

---

## Implementation plan (single follow-up session)

All work is in `internal/export/html.go` (data plumbing + template). No other package
needs to change. Keep the existing meta-graph mode for `NumNodes() > metaThreshold`;
this plan upgrades the **node-level** view to Python parity (and applies search +
stats to both modes where it makes sense).

### Phase 1 — get the data onto the page

Extend the structs the template is fed:

- `vnode`: add `FileType string`, `Degree int`, `Norm string` (lowercased label for
  search; reuse `export.normLabel`), and split the current combined `Title` so the
  panel can show `source_file` and `source_location` separately. Keep `Title` for the
  hover tooltip.
- `vedge`: add `Relation string`, `Confidence string`, and replace the uniform width
  with confidence-derived `Width float64` + `Dashes bool`. Set per the Python rule:
  `EXTRACTED → {width:2, dashes:false, opacity:0.7}`, else `{width:1, dashes:true,
  opacity:0.35}`. (vis.js supports per-edge `dashes` and per-edge `color.opacity`.)
- Build a small JS-side adjacency map (or rely on `network.getConnectedNodes`) plus a
  `{edgeKey → {relation, confidence}}` lookup so the panel can label each neighbor by
  relation and direction. Cheapest correct approach: emit a `neighbors` array per node
  server-side: `[{id, relation, dir}]`, grouped later in JS. Reuse `g.Edges()` for
  direction (source→target) and `relation`.

### Phase 2 — sidebar UI (template)

Replace the three floating panels with a right sidebar containing:

1. **Search** — text input + results `<div>`. On `input`, filter `RAW` by `norm`
   (substring), show top 20, click → `focusNode(id)`.
2. **Info panel** — populated by `showNode(id)`: label, type, community, source
   file:location, degree, then neighbor links grouped under **Calls / Called by /
   Imports / Contains** headings (mirror `internal/query`'s `explain` grouping so the
   web view and CLI tell the same story). Each neighbor is a clickable chip with a
   community-colored left border → `focusNode`.
3. **Legend** — keep current checkbox show/hide behavior.
4. **Stats** — `N nodes · M edges · K communities` (works in both modes; replaces the
   meta-only banner, or keep banner too).

`focusNode(id)`: `net.focus(id,{scale:1.4,animation:true})`, `net.selectNodes([id])`,
then `showNode(id)`.

### Phase 3 — selection behavior

**Copy the Python behavior.** Change `net.on("click")` for node-level mode from
*isolate-community* to *inspect + highlight-neighborhood*: on node click, dim all
nodes/edges except the clicked node, its neighbors, and their connecting edges, and
open the info panel. Click empty space resets (un-dim everything, clear the panel).

Keep community-isolate available, but as a *secondary* action so it doesn't displace
inspect — the legend's existing show/hide checkboxes already cover hiding a whole
community, so a separate isolate gesture is optional; only add one (e.g. a "focus this
community" affordance) if it falls out cheaply. The primary click is inspect, matching
the original.

### Phase 4 — verify

`internal/export` currently has **no tests** (see `GOALS.md` → test coverage gap).
Minimum bar:

- Add a small `html_test.go` that builds a tiny graph, calls `ToHTML`, and asserts the
  emitted HTML contains the new hooks: a search input, `function showNode`, at least
  one `dashes` edge for a non-`EXTRACTED` confidence, and neighbor entries. This guards
  the data-plumbing, which is where regressions will hide.
- Manual: `go run ./cmd/graphify build .` then open `graphify-out/graph.html` — confirm
  search focuses nodes, the panel lists grouped clickable neighbors, inferred edges
  render dashed, and selection dims the rest.
- `go build ./... && go vet ./... && go test ./...` stays green.

### Effort / sequencing

Phases 1→2→3 are strictly ordered (each needs the prior). Phase 1 is ~½ the work
(struct + marshal changes, ~40 lines). Phases 2–3 are template/JS (~120 lines of
inline JS/HTML in the `htmlTemplate` const). Phase 4 is a small test + manual check.
Single session is realistic.

## Explicitly out of scope (don't pull these in)

- LLM/semantic extraction, Obsidian/Neo4j/SVG/GraphML exports, MCP server, wiki/serve —
  intentionally not ported (`GOALS.md` → Out of scope).
- Meta→node drill-down and `metaThreshold` tuning — real follow-ups, but tracked
  separately under `GOALS.md` → Viewer; not part of closing the Python parity gap.
- Reworking extraction/resolution/clustering — not needed; this is viewer-only.

## Key references

- Go viewer: `internal/export/html.go` (`ToHTML`, `buildNodeLevel`, `htmlTemplate`).
- Go data available: `internal/model/model.go` (`Edge.Relation`, `Edge.Confidence`,
  `Node.FileType`, `Graph.Degree`, `Graph.Neighbors`, `Graph.Edges`); `graph.json`
  fields in `internal/export/export.go`.
- CLI grouping to mirror in the panel: `internal/query` (`explain`).
- Python original: `safishamsi/graphify` → `graphify/export.py` `to_html()`.
