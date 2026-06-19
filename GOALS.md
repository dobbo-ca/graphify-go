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
- [x] Outputs: `graph.json` (NetworkX node-link, upstream-compatible), `GRAPH_REPORT.md`.
- [x] `query` / `explain` / `path` commands for graph-first navigation.
- [x] Security: SSRF URL guard, graph-path containment, file-size caps, label sanitisation, sensitive-file skip.
- [x] CI: `ci.yml` (build/vet/test) + `graph.yml` (regenerate + commit graph on merge to main).
- [x] Claude skill (`skills/graphify/SKILL.md`) + "use the graph, not grep" block in `CLAUDE.md`.
- [x] Release + Homebrew: `release.yml` (Uplift → cgo build on native macOS/Linux runners → GitHub release → repository_dispatch to `dobbo-ca/homebrew-taps`), `.cliff.toml`, `graphify version`. Formula template added to homebrew-taps.

## Follow-ups

Roughly priority order.

### Language coverage
- [x] **Terraform / HCL extractor** (done — higher priority than Python/Rust).
- [x] **Python extractor** (functions, classes + methods, imports, calls). Tree-sitter Python `v0.25.0`.
- [x] **Rust extractor** (functions, struct/enum/union/trait/type, impl methods, `use` imports, call + macro edges; descends into `mod`). Tree-sitter Rust `v0.24.2`.
- [x] **13 more extractors** (in-spirit parity push): C, C++, Java, C#, Ruby, PHP, Bash, Scala, Julia, Verilog/SystemVerilog, Kotlin, Lua, Zig — every tree-sitter grammar with a Go binding. Node kinds verified against each grammar's `node-types.json`; each ships a unit test. Coverage is now **19 language families**.
- [x] Wired each into `detect.SupportedExtensions`, `extract.File` dispatch (`langFamily` already had `.py`/`.rs`).
- Deferred (no Go tree-sitter binding): Elixir, PowerShell, Fortran, Swift, Obj-C. Structural-only (no fn/type/call model): JSON, Markdown.

### Performance (original goal stage 3 — partly outstanding)
- [x] **Parallelize extraction.** `cmdBuild` fans out `extract.File` across a `runtime.NumCPU()` worker pool (`extractAll`); fixed result slots preserve file order so graph output stays byte-identical to the sequential path.
- [x] **Incremental rebuild** (`graphify update`): caches each file's extraction result keyed by content hash (`internal/cache`, `graphify-out/.graphify_cache.json`) and re-parses only changed files, reusing cached results for the rest. Output is byte-identical to a full build (verified by test). Plus `graphify watch` (poll-based) and `graphify hook install` (post-commit/merge/checkout hooks).

### Correctness / coverage
- [x] **Respect `.gitignore`** in `detect.CollectFiles` — pure-Go matcher (`internal/detect/gitignore.go`) covering nested files, negation, anchoring, dir-only, and `*`/`?`/`**` globs; expectations cross-checked against `git check-ignore`.
- [x] **Test coverage** for `cluster`, `analyze`, `report`, `export` (now ~87–90% each; covered across the pipeline: idutil, security, model, detect, extract, graph, query, cluster, analyze, report, export).
- [x] **Improve cross-file call resolution precision** — when a called name has several definitions, `Resolve` disambiguates by the caller's imports, then by same-directory (same package), instead of skipping. Only ever adds uniquely-determined edges.
- [x] cloudposse null-label awareness: tag + computed_name search + context-chain reconstruction

### New commands (in-spirit parity push)
- [x] `graphify export <graphml|dot|csv>` — extra exports for downstream tooling.
- [x] `graphify affected [file...]` — change blast radius: nodes in changed files + their transitive dependents (callers/importers); no args → git diff vs HEAD.
- [x] `graphify validate` — structural check of `graph.json` (dangling edges, duplicate/empty ids); non-zero exit gates CI.
- [x] `graphify serve` — agent-first MCP stdio server (JSON-RPC over stdio): loads `graph.json` once into a resident process and answers the 7 upstream tools (`query_graph`, `get_node`, `get_neighbors`, `get_community`, `god_nodes`, `graph_stats`, `shortest_path`) over the existing query/analyze primitives, so an agent issues many parseable queries without re-paying the load cost per shell-out. Stdio-only (HTTP/api-key/hot-reload skipped).
- [ ] Deferred dep-light commands: `graphify global` (multi-repo registry + merge), PR analysis, `cluster`-only recluster, `diagnostics` (marginal — `build` already clusters and `GRAPH_REPORT.md` already reports stats).

### Operational (release pipeline)
- [ ] Confirm the GH App (`GH_PUB_APP_CLIENT_ID` / `GH_PUB_APP_PEM`) is installed on **graphify-go** and **homebrew-taps** and visible to graphify-go (needs org admin). Without it, `release.yml` and the tap dispatch can't run.
- [ ] After the first release, verify `brew install dobbo-ca/taps/graphify-go` works on macOS arm64/amd64 + linux.
- [ ] `release.yml` and `graph.yml` both fire on push to `main`; both push back with `[skip ci]` (no loops) but two concurrent pushes can race. If it bites, fold the graph commit into the release flow.

## Out of scope (intentionally not ported)

The features that need external services or a far larger surface than the
minimal port targets: LLM-based semantic extraction and the LLM backends,
video/audio transcription, image vision, Office/Postgres/Google
ingest, Neo4j / Obsidian / SVG exports, and the multi-assistant installers.
The exotic language extractors without a Go tree-sitter binding stay out too
(see Language coverage above).
