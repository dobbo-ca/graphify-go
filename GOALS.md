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

## Parity backlog — upstream audit 2026-07-09

Audited graphify-go against upstream `safishamsi/graphify` (now `Graphify-Labs/graphify`,
release v0.9.11, `0.9.12` unreleased) across six dimensions — CLI, extractors, cross-file
resolution, MCP serve, graph.json schema/report/export, detect/security — each with an
adversarial verification pass. **Verdict: parity is strong for the agent-first scope.** Most
recent upstream churn is extractor/resolution correctness that does not reproduce here (Go's
resolver emits no type-reference or inheritance edges and drops-on-ambiguity rather than
guessing, so it sidesteps whole bug classes upstream keeps patching: #1726, #1713, #1603,
#1707/#1729/#1744, #1581, #1668, #1638 are all confirmed Go-safe). The confirmed in-scope
gaps are below. Anything not listed is at parity or a deliberate non-port (see "Out of scope").

> **STATUS 2026-07-10 — all 16 beads implemented.** Landed on stacked branches
> `parity-p0` → `parity-p1` → `parity-p2` → `parity-p3` (the `parity-p3` tip contains
> everything: 20 feature commits). Each bead went through an implement → adversarial
> review → fix → build/vet/test loop; suite grew 207 → **250** tests, `graph.json` output
> verified deterministic. Epic `graphify-go-1fz` closed. **Not yet merged to `main`** —
> open a PR from `parity-p3` (or squash-merge the stack) and commit this GOALS.md update.

Each item is tracked as a bead under epic **`graphify-go-1fz`** (`bd list`). Label map
(`grp:*` = collision group, `tier:*` = priority):

| Item | Bead | Item | Bead | Item | Bead |
|------|------|------|------|------|------|
| H1 | `graphify-go-1fz.1` | E4 | `graphify-go-1fz.7` | P-1 | `graphify-go-1fz.11` |
| H2 | `graphify-go-1fz.2` | C1 | `graphify-go-1fz.8` | P-2 | `graphify-go-1fz.12` |
| R1 | `graphify-go-1fz.3` | C2 | `graphify-go-1fz.9` | P-3 | `graphify-go-1fz.13` |
| E1 | `graphify-go-1fz.4` | S1 | `graphify-go-1fz.10` | P-4 | `graphify-go-1fz.14` |
| E2 | `graphify-go-1fz.5` |    |      | P-5 | `graphify-go-1fz.15` |
| E3 | `graphify-go-1fz.6` |    |      | P-6 | `graphify-go-1fz.16` |

`bd ready` surfaces unblocked work; P-2 is blocked-by H2. Priorities map to `bd` `-p 0..3`.

### P0 — data integrity (do first)

- [x] **H1 · detect: exempt real source from the sensitive-file drop (#1666).** ✅ `parity-p0-a0fea0`
  `internal/detect/detect.go` `isSensitive` applies the `(credential|secret|passwd|password|private_key)s?`
  pattern to the basename unconditionally, silently dropping real auth/secret *code*
  (`password_reset.go`, `passwords_controller.rb`, `credentials_controller.rb`, `secret_store.py`)
  from the graph. Port upstream's source-code exemption (`classify_file==CODE` and ext not a
  secret-prone data ext) plus the load-bearing word-count gate. Files: `detect.go`. Effort M.
- [x] **H2 · export: anti-shrink / fail-safe `graph.json` overwrite guard (#479).** ✅ `parity-p0-a0fea0`
  `internal/export/export.go` `ToJSON` does an unconditional `os.WriteFile` — no node-count
  compare, no read-back, no `force`. A transient walk error, corrupt prior file, or missing
  cache chunk silently clobbers the CI-committed, agent-consumed graph. Before writing: parse
  the existing non-empty target, refuse on shrink or unparseable-existing unless `force`; wire
  `force` from `build`/`update`/`watch`. Files: `export.go`, `cmd/graphify/main.go`. Effort M.

### P1 — correctness

- [x] **R1 · resolve: filter cross-language `calls` edges (#1718).** ✅ `parity-p1`
  `internal/graph/build.go` `langFamily` covers only go/rust/js/py, so the ~15 other extracted
  languages bypass the cross-language filter — a Python call binding to a unique Kotlin/Java/C++/Ruby
  def survives as a phantom `calls` edge. The Python import-guided path (`resolve.go`) also emits
  `EXTRACTED` confidence, which the `INFERRED`-only backstop never filters. Fix: complete the
  `langFamily` map (S) and, better, add a resolver-time family guard in `disambiguate` /
  `resolveImportGuided` so the edge is never emitted (M). Files: `build.go`, `internal/extract/resolve.go`.

### P1 — deterministic-extraction parity (default no-LLM graph diverges from upstream)

- [x] **E1 · extract: Java/Kotlin enum-constant nodes + `case_of` edges (#1719/#1700).** ✅ `parity-p1`
  `java.go`/`kotlin.go` skip `enum_constant`/`enum_entry`; no `case_of` kind exists. Breaks
  "where is `EnumType.X` used". Files: `java.go`, `kotlin.go`. Effort S.
- [x] **E2 · extract: deterministic rationale + doc_ref/cites post-pass.** ✅ `parity-p1`
  Upstream scans `# NOTE:/WHY:/HACK:` + `// NOTE:` comments and `ADR-NNNN`/`RFC NNNN` tokens
  into `rationale`/`doc_ref` nodes with **no LLM**; Go produces these only via the opt-in
  `--semantic` backend, so the default graph has none. Files: `python.go`, `javascript.go`. Effort M.
- [x] **E3 · extract: Vue/Svelte/Astro extractors.** ✅ `parity-p1`
  Run the existing JS/TS grammar over the `<script>` block + import regex fallback — no new
  binding needed. Files: `extract.go`, `detect.go`. Effort M.
- [x] **E4 · extract: non-Cargo package-manifest ingestion.** ✅ `parity-p1`
  Only Cargo is ingested today (opt-in `--cargo`); own `go.mod` isn't indexed. Add a canonical
  `type=package` + `depends_on` ingester for pyproject.toml / go.mod / pom.xml. Files: new
  ingester alongside `cargo.go`, wired in `main.go`. Effort L.

### P2 — agent UX / commands

- [x] **C1 · serve: annotate MCP `shortest_path` with per-hop relation/confidence + same-node guard.** ✅ `parity-p2`
  `toolShortestPath` emits a bare label join; `query.Path` already walks real edges. Files:
  `cmd/graphify/serve.go`, `internal/query/query.go`. Effort M.
- [x] **C2 · affected: add `--depth`/`--relation` scoping (keep the git-diff auto-anchor).** ✅ `parity-p2`
  Files: `cmd/graphify/main.go`, `internal/query`. Effort M.
- [x] **S1 · security: honor `GRAPHIFY_MAX_GRAPH_BYTES` env override.** ✅ `parity-p2`
  `MaxGraphFileBytes` is a hardcoded const; large monorepos hit an unraisable 512 MiB cap.
  Files: `internal/security/security.go`. Effort S.

### P3 — polish

- [x] ✅`parity-p3` **P-1 · detect: surface walk errors + seen-but-skipped / no-extractor files (#1692/#1689/0.9.11).**
  `CollectFiles` swallows `WalkDir` errors (`return nil`) and drops unsupported files with no
  count/warning — silent partial graphs. Files: `detect.go`, `main.go`. Effort S.
- [x] ✅`parity-p3` **P-2 · update: `--force` + node-count shrink guard (optional `--no-cluster`).** Folds into H2. Files: `main.go`. Effort S.
- [x] ✅`parity-p3` **P-3 · detect/extract: discover `.mts`/`.cts` + shebang extensionless dispatch (#1607/#1683).** Files: `detect.go`, `extract.go`. Effort S/M.
- [x] ✅`parity-p3` **P-4 · cli: `--graph` override for `explain`/`path`** (the primitive already exists for `ask`/`diff`). Files: `main.go`. Effort S.
- [x] ✅`parity-p3` **P-5 · (weight emitted; edge `context` deferred — needs extractor plumbing) export: emit edge `context`/`weight` fields** (informational; add only if a downstream consumer keys on them). Files: `export.go`. Effort S.
- [x] ✅`parity-p3` **P-6 · extract: markdown depth** — heading sub-nodes, reference-style/wikilink syntax, backtick code-symbol `references` edges. Files: `markdown.go`. Effort M.

### Execution strategy — implement / review / fix / verify workflow

Run each bead through a closed loop until it is provably done:

```
implement (isolated worktree) → adversarial review → apply fixes → verify → repeat until green
```

- **implement** — one agent per bead, `isolation: worktree` so parallel edits don't collide.
- **review** — a second agent adversarially reviews the diff against the upstream reference and
  the bead's definition of done (below); it does not trust the implementer.
- **fix** — apply review findings; re-review. Loop until the reviewer returns no blocking findings.
- **verify** — `go build ./... && go vet ./... && go test ./...` must pass, plus the item-specific
  check below. Only then close the bead.

**Definition of done (every item):** upstream behavior matched; a unit test added that mirrors the
upstream fix's scenario (the file that *should* now be kept/extracted/annotated); `go test ./...`
green; and for anything touching the build pipeline, `graph.json` output stays deterministic
(re-running `build` twice is byte-identical) — this is a hard invariant of the incremental-cache design.

**Batching for a single pass — group by file collision, sequence within a group, parallelize across:**

| Group | Beads (in order) | Primary files |
|-------|------------------|---------------|
| `write`   | H2 → P-2 → S1 → P-5      | `export.go`, `main.go`, `security.go` |
| `detect`  | H1 → P-1 → P-3 → E3      | `detect.go`, `extract.go` |
| `resolve` | R1                      | `build.go`, `resolve.go` |
| `java`    | E1                      | `java.go`, `kotlin.go` |
| `jspy`    | E2                      | `python.go`, `javascript.go` |
| `serve`   | C1 → C2 → P-4           | `serve.go`, `query.go`, `main.go` |
| `manifest`| E4                      | new ingester, `main.go` |
| `md`      | P-6                     | `markdown.go` |

Groups are independent (disjoint file sets), so all eight run in parallel; within a group the
beads are sequential to avoid same-file merge conflicts. `main.go` is touched by three groups —
land those beads' `main.go` edits behind small, non-overlapping hunks, or serialize the final
merge of `write`/`serve`/`manifest`. Do P0 (H2, H1) first regardless of parallelism; they are the
data-integrity fixes and everything else builds on a trustworthy `graph.json`.

## Out of scope (intentionally not ported)

The features that need external services or a far larger surface than the
minimal port targets: LLM-based semantic extraction and the LLM backends,
video/audio transcription, image vision, Office/Postgres/Google
ingest, Neo4j / Obsidian / SVG exports, and the multi-assistant installers.
The exotic language extractors without a Go tree-sitter binding stay out too
(see Language coverage above).
