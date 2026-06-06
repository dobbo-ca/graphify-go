# GOALS

Reference for future sessions. Tracks the original objective, what's done, and
what's left.

## Objective

Port the Python project [graphify](https://github.com/safishamsi/graphify) to Go
as [dobbo-ca/graphify-go](https://github.com/dobbo-ca/graphify-go): turn a
codebase into a queryable knowledge graph, used two ways —

1. **CI artifact** — regenerate the graph on merge to `main` and commit it.
2. **Agent navigation** — a Claude skill + CLAUDE.md block so the agent queries
   the graph instead of grepping.

Guiding constraints (from the original request): attribute the source (MIT),
simplest possible implementation, security-first, find performance wins.

## Done

- [x] Public repo under dobbo-ca, MIT preserving the original copyright, attribution in `NOTICE.md`.
- [x] Clean-room reimplementation of the pipeline: `detect → extract → build → cluster → analyze → report → export`.
- [x] Tree-sitter extractors for **Go, JavaScript, TypeScript** (files, functions, types, methods; `contains` / `calls` / `imports` / `imports_from` edges).
- [x] Tree-sitter extractor for **Terraform / HCL** (`.tf`/`.tfvars`/`.hcl`): resources, data sources, modules, variables, outputs, providers, locals; `contains` / `references` / `depends_on` edges, directory-scoped so cross-file references resolve.
- [x] Whole-corpus call + import resolution (calls resolve to definitions; relative imports resolve to files).
- [x] Louvain community detection (gonum) with oversized-community splitting.
- [x] Analysis: god nodes, surprising connections, file-level import cycles.
- [x] Outputs: `graph.json` (NetworkX node-link, upstream-compatible), `GRAPH_REPORT.md`, `graph.html`.
- [x] `query` / `explain` / `path` commands for graph-first navigation.
- [x] Security: SSRF URL guard, graph-path containment, file-size caps, label sanitisation, sensitive-file skip.
- [x] CI: `ci.yml` (build/vet/test) + `graph.yml` (regenerate + commit graph on merge to main).
- [x] Claude skill (`skills/graphify/SKILL.md`) + "use the graph, not grep" block in `CLAUDE.md`.
- [x] Release + Homebrew: `release.yml` (Uplift → cgo build on native macOS/Linux runners → GitHub release → repository_dispatch to `dobbo-ca/homebrew-taps`), `.cliff.toml`, `graphify version`. Formula template added to homebrew-taps.
- [x] HTML viewer: aggregated community meta-graph for large graphs; degree-sized node view with `avoidOverlap` for small graphs; off-screen-solve-then-freeze (no spin); legend with show/hide; click-to-isolate-community.

## Follow-ups

Roughly priority order.

### Language coverage
- [x] **Terraform / HCL extractor** (done — higher priority than Python/Rust).
- [ ] **Python extractor** (next, per original scope). Tree-sitter Python binding available.
- [ ] **Rust extractor**. Tree-sitter Rust binding available.
- [ ] Wire each into `detect.SupportedExtensions`, `extract.File` dispatch, and `langFamily` in `internal/graph/build.go`.

### Performance (original goal stage 3 — partly outstanding)
- [ ] **Parallelize extraction.** `cmdBuild` extracts files sequentially; fan out across a worker pool (files are independent; `Resolve` already runs once afterward). Biggest easy win on large repos.
- [ ] **Incremental rebuild** (`graphify update`): re-extract only changed files and merge into the existing `graph.json`, instead of a full rebuild. The original has `build_merge` + a file watcher.

### Correctness / coverage
- [ ] **Respect `.gitignore`** in `detect.CollectFiles` (currently only a fixed skip-dir/skip-file list). Generated/ignored files can pollute the graph.
- [ ] **Test coverage** for `cluster`, `analyze`, `report`, `export` (currently covered: idutil, security, model, detect, extract, graph, query).
- [ ] Improve cross-file call resolution precision (current rule: same-file def, else unique global by name — ambiguous names are skipped).

### Viewer
- [ ] Drill-down from a community circle (meta view) to that community's node-level subgraph.
- [ ] Tune `metaThreshold` (currently 500) if the macro view kicks in too early/late.

### Operational (release pipeline)
- [ ] Confirm the GH App (`GH_PUB_APP_CLIENT_ID` / `GH_PUB_APP_PEM`) is installed on **graphify-go** and **homebrew-taps** and visible to graphify-go (needs org admin). Without it, `release.yml` and the tap dispatch can't run.
- [ ] After the first release, verify `brew install dobbo-ca/taps/graphify-go` works on macOS arm64/amd64 + linux.
- [ ] `release.yml` and `graph.yml` both fire on push to `main`; both push back with `[skip ci]` (no loops) but two concurrent pushes can race. If it bites, fold the graph commit into the release flow.

## Out of scope (intentionally not ported)

LLM-based semantic extraction, Obsidian / Neo4j / SVG exports, MCP server,
AI-assistant installers, the 30+ other language extractors.
