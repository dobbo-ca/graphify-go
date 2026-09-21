# Upstream delta backlog — graphify-go vs graphify v0.9.65

Baseline: last parity audit closed 11 gaps vs upstream v0.9.16/0.9.17 (go commit 797ba38, 2026-07-15).
Upstream now v0.9.65 (726 commits of drift). Generated 2026-09-21 by the graphify-delta-analysis workflow.

## Summary

47 raw delta items against upstream v0.9.65 collapse to 44 beads after merging three cross-area duplicates (reflect, cross-repo global graph + cross-repo call/type joins, URL ingest into the LLM omnibus). 26 are build, 18 are skip, none are remove — the human HTML viewer was already deleted and no other human-facing surface was ever ported, so the agent-first stance shows up here as refusals rather than removals: no HTTP MCP transport, no benchmark ratio report, no Neo4j/Obsidian/SVG/wiki exports, no querylog, no LLM path. The stance also drives what IS built: six p0 items are all cases where the tool answers confidently and wrongly to an agent that cannot see the source to check it — a same-name method collision binding calls to the wrong type (internal/extract/resolve.go:34, verified still map[string]string), a stat-fastpath race that serves a stale graph, a --force flag that does not bypass the cache, a false TS import cycle from type-only imports, an ambiguous `explain Foo` resolving to an arbitrary file (internal/query/query.go:306), and unbounded get_neighbors/get_community dumps into the context window. The p1 tier is capability that agents actually reach for and cannot get today: inheritance edges (no implements/inherits relation exists anywhere in internal/), TS alias imports (@/ specifiers drop to external concept nodes), absolute/./-prefixed `affected` seeds, cwd-independent graphify-out resolution, and cache invalidation on binary upgrade. Skips are justified by absence of a host subsystem (merge-graphs, query-memory), by go's whole-corpus 87ms rebuild making upstream's lock/queue machinery pointless, or by GOALS.md exclusions that still hold at v0.9.65. Verified: the collision, idutil fold-order, missing IndexedAtNs, first-match resolve, bare-ToSlash affected seeds, argument-less cmdServe, and the absence of type_only/implements/dispatches_to. Not verified by me: the per-item empirical repros the area agents reported (stale-cache update, false TS cycle, model.md node loss) — I took those on their stated evidence; and the beads tracker is unreadable, so "already-tracked" reflects GOALS.md only.

## Beads (from synthesis)

### [synth] p0 `build` `core-cli` — Fix same-name method collision that emits wrong calls edges

**type** bug · **labels** delta, core-cli, languages-extraction, correctness · **goals** new

WHAT: internal/extract/resolve.go:34 indexes definitions file-locally as `local[d.File+"\x00"+d.Name] = d.ID` — a plain map[string]string, last write wins (verified still present). When one file declares two types that each own a same-named method (routine in Ruby/PHP/Kotlin/Scala/C#), every unqualified call to that name in that file binds to whichever type parsed LAST. Reproduced upstream-side: a two-class C# file where A.Build() calls A.Helper() emits `x_a_build -> x_b_helper`.

WHY (agent value): calls edges are the substrate explain/path/affected read. This is not a missing edge, it is a silently wrong one — an agent tracing a call chain is misdirected with no signal. Note GOALS.md's parity section claims go 'drops-on-ambiguity rather than guessing'; that claim is false for the file-local index, so this also corrects the documented invariant.

FILES: internal/extract/resolve.go (index at :34, lookup at :76, existing `unique` drop-on-ambiguity convention at :414); test in internal/extract/resolve_test.go.

UPSTREAM: graphify/extract.py commits 58c497d ('Keep every method a duplicate index key maps to, not just the last one'), 8c7b667 ('Refuse to pool methods across two unrelated same named types'); graphify/extractors/resolution.py.

ACCEPTANCE: a two-type single-file fixture where A.Build calls A.Helper yields A.Build -> A.Helper and no edge to B.Helper.

**Rationale:** Wrong data is worse than absent data for an agent, and the fix is a map type change plus a length check — smaller than any mitigation. Highest-severity item in the whole delta.

**Minimal change:** Change `local` to map[string][]string, append instead of assign; at the lookup take the single entry or fall through to the existing `disambiguate` path when len != 1. Do NOT port upstream's type-scoped method pooling — plain drop-on-ambiguity is the lazy correct answer and already this file's convention.

### [synth] p0 `build` `core-cli` — Close the racily-clean hole in the stat fastpath

**type** bug · **labels** delta, core-cli, build-incremental-watch, correctness, cache · **goals** new

WHAT: the stat sidecar skips re-reading a file when (size, mtime_ns) match the previous run. StatEntry (internal/cache/cache.go:33-38, verified: Size/MtimeNs/Hash only) carries no freshness proof, so a same-length edit inside the same mtime tick leaves both fields untouched and `graphify update` reuses the stale cached extraction. Area agent reproduced it: rewrite func AAAA -> func BBBB at identical length, restore mtime_ns, update reports '1 files (0 reparsed, 1 reused)', `query BBBB` returns no matches while `query AAAA` still resolves.

WHY (agent value): `update` is what the installed git hook and every agent session run constantly; a silently stale graph poisons every downstream query/explain/affected answer. Currently unrecoverable from the CLI because --force does not bypass the cache either (see that bead).

FILES: internal/cache/cache.go only (StatEntry, HashFile, the fastpath gate); test in internal/cache/cache_test.go.

UPSTREAM: graphify/cache.py:196-286 (_MTIME_GRANULARITY_NS, _mtime_granularity_ns, _stat_sig_fresh, _stat_entry_for); commit 26245b0 'fix(cache): implement racily-clean guard for file hashing'.

ACCEPTANCE: same-size rewrite + os.Chtimes back to the original mtime produces a changed hash.

**Rationale:** Reproducible silent wrong-answer bug with no workaround in go, fixed in ~15 lines in one file. The guard's cost is bounded to files touched in the last 2s — exactly the files that changed and must be read anyway.

**Minimal change:** Add `IndexedAtNs int64 `json:"indexed_at_ns"`` to StatEntry; capture now := time.Now().UnixNano() BEFORE os.ReadFile and stamp it; gate the fastpath on prev.IndexedAtNs != 0 && mtime+granularity <= prev.IndexedAtNs. Granularity: package const 2e9 read through a small func honouring GRAPHIFY_MTIME_GRANULARITY_MS (0 restores old behaviour). Old sidecars decode with 0 and get one re-read each — no migration.

### [synth] p0 `build` `core-cli` — Make --force / GRAPHIFY_FORCE bypass cache reads, not just the shrink guard

**type** bug · **labels** delta, core-cli, build-incremental-watch, cache · **goals** new

WHAT: in go, --force is parsed into buildOpts.force and threaded only to writeOutputs -> export.CheckShrink; assemble() never sees it. Verified empirically by the area agent: against the stale-cache repro, both `graphify update . --force` and `GRAPHIFY_FORCE=1 graphify update .` still report '0 reparsed, 1 reused' and leave the wrong graph. Upstream's --force means 'full re-scan, cache reads skipped' and is the documented recovery lever its own error messages point users at.

WHY (agent value): it is the flag an agent or CI job passes to get a trustworthy rebuild. Without it the only remedy for a poisoned cache is knowing to rm graphify-out/.graphify_cache.json, which is undiscoverable from --help.

FILES: cmd/graphify/main.go (assemble signature, cmdBuild/cmdUpdate call sites, envForce() at :266); test in cmd/graphify/update_test.go.

UPSTREAM: graphify/cli.py:2404-2412 (update --force), :3466-3485 (extract --force), graphify/cache.py:196-201, 1297-1309.

ACCEPTANCE: hand-corrupt a cache entry; plain update keeps it, --force replaces it.

**Rationale:** ~5 lines — one parameter through an existing call chain — and it is the escape hatch that makes every other cache bug survivable. Land it before the two cache-correctness fixes so any residual staleness already has a user-facing remedy.

**Minimal change:** Give assemble() a `force bool`; when set skip both the prev-cache hit branch and the stat fastpath. cmdBuild passes false, cmdUpdate passes opts.force || envForce(). Reuse the existing envForce() — do not add a second env reader.

### [synth] p0 `build` `core-cli` — Exclude type-only TS imports from import-cycle detection

**type** bug · **labels** delta, core-cli, analysis-quality, typescript · **goals** new

WHAT: a TypeScript `import type { T } from './m'` is erased at compile time and cannot be a runtime cycle. go treats it as a value import; verified there is no `type_only` or `deferred` mention anywhere under internal/. Area agent reproduced end to end: a.ts with `import type { B } from './b'` plus b.ts with a value `import { mkA } from './a'` makes `graphify build` emit '- 2-file cycle: b.ts -> a.ts -> b.ts' in GRAPH_REPORT.md.

WHY (agent value): import-cycle output is something an agent reads and acts on ('you have a circular dependency'). A false cycle is a wrong answer, and TS/TSX is a headline extractor, so every type-only round-trip in a TS repo is a false positive today.

FILES: internal/extract/extract.go:43 (Imp), internal/extract/javascript.go:36 (import_statement + export_statement re-exports), internal/model/model.go:21 (Edge), internal/extract/resolve.go:101-107 (stamp the imports_from edge), internal/analyze/analyze.go:124 (ImportCycles skip).

UPSTREAM: graphify/analyze.py:679-690 (find_import_cycles type_only/deferred skip, commit b84b4a7, #3123); graphify/extractors/resolution.py edge stamping.

ACCEPTANCE: the two-file TS corpus yields zero cycles; a value-import cycle still reports.

**Rationale:** Reproduced false positive on the most-used extractor, four small edits, no new packages. `type_only,omitempty` keeps graph.json byte-identical for corpora with no type-only imports so the determinism goldens are untouched.

**Minimal change:** Detect via a direct `type` child of the import_statement — tree-sitter-typescript puts the keyword inside the specifier for a mixed `import { type A, B }`, so a direct-child check gives upstream's mixed-import behaviour for free.

### [synth] p0 `build` `core-cli` — Disambiguate duplicate node labels in query resolution (path::Symbol form)

**type** bug · **labels** delta, core-cli, agent-interfaces-serving, query · **goals** new

WHAT: (*Graph).resolve returns the FIRST exact-label match in node order (verified at internal/query/query.go:306-310), so `graphify explain Foo` in a repo with three Foos silently answers about an arbitrary one. The substring tier does detect ambiguity but returns a bare `no node matching %q` with no candidate list and no way to disambiguate. Upstream refuses to guess, lists candidate files, and accepts a `path::Symbol` form.

WHY (agent value): this is the lookup every agent-facing command routes through — explain, path, and MCP get_node/get_neighbors/shortest_path. A silently-wrong answer is worse than an error for an agent that cannot see the source to check.

FILES: internal/query/query.go:301-322 (resolve; copy the SameNodeError/MaxHopsError pattern for a new AmbiguousError), cmd/graphify/main.go explain/path handlers, cmd/graphify/serve.go toolGetNode/toolGetNeighbors (both currently flatten every error to "No node matching '%s' found.").

UPSTREAM: graphify/serve.py _find_node_tiers/_resolve_single_node (~L1410-1650); commits f9b6f21, 2ca565a, 72edfc0; tests/test_explain_cli.py.

ACCEPTANCE: two same-labelled nodes produce an ambiguity error listing both file:line, and `dir/f.go::Foo` selects one.

**Rationale:** Produces confidently wrong output rather than an error, and the fix is confined to one function plus error surfacing at three call sites.

**Minimal change:** Before the existing tiers, split on the last `::` and match label==symbol AND SourceFile has the path part as a suffix. Change the exact-label loop to collect all hits and return AmbiguousError with the candidate file:line list on >1.

### [synth] p0 `build` `core-cli` — Honor token_budget on MCP get_neighbors and get_community

**type** bug · **labels** delta, core-cli, agent-interfaces-serving, mcp · **goals** new

WHAT: toolDefs() declares token_budget only on query_graph (verified at cmd/graphify/serve.go:381); get_neighbors and get_community have no budget and their handlers (serve.go:238-276) emit every neighbour / every community member unconditionally. Upstream defaults both to 2000 and prints the truncation notice at the TOP of the output so the caller sees it before reading.

WHY (agent value): output budgeting is the most agent-specific concern in an MCP surface — the tool exists to feed a context window, and calling get_neighbors on a god node or get_community on the largest community dumps the whole list into it.

FILES: cmd/graphify/serve.go (two schemas in toolDefs() at L386-394, toolGetNeighbors/toolGetCommunity); reuse the ~3-chars-per-token cut and truncation wording already in internal/query/ask.go:438.

UPSTREAM: graphify/serve.py tool schemas ~L1897-1921, _tool_get_neighbors/_tool_get_community; commits fef9dbb, deb2620.

ACCEPTANCE: a 500-neighbour node with token_budget=200 returns a truncated list whose first line reads 'Truncated: N of M shown'.

**Rationale:** Cheap — the budgeting logic already exists in ask.go — and it protects the exact resource the whole project optimizes for.

**Minimal change:** Accumulate line lengths, stop at argInt(args,"token_budget",2000)*3 chars, prepend the notice. Reuse the existing constant; do not introduce a second tokenizer estimate.

### [synth] p1 `build` `core-cli` — Emit inherits/implements edges from OO type declarations

**type** feature · **labels** delta, core-cli, languages-extraction, edges · **goals** new

WHAT: go emits no inheritance relation at all — verified, no `implements`/`inherits` edge relation exists under internal/ (only terraform's inherits_context and JSON's tsconfig `extends`). java.go:javaType, csharp.go:csType, php.go:phpType, scala.go:scalaType, kotlin.go, ruby.go:rubyType and python.go all read only the `name` field and the body, discarding the superclass / base_list / super_interfaces field the tree-sitter node already carries.

WHY (agent value): 'what implements this interface' and 'what subclasses this base' are primary navigation queries an agent asks, and today they are unanswerable from the go graph. `explain` already renders arbitrary relations, so the edges are usable the moment they exist. This also unblocks the dispatches_to bead.

FILES: internal/extract/java.go (superclass/interfaces), csharp.go (bases), python.go (superclasses), php.go (base_clause/class_interface_clause), kotlin.go, scala.go (extends_clause), ruby.go (superclass); route name->id through the existing global/disambiguate path in internal/extract/resolve.go.

UPSTREAM: graphify/extractors/engine.py lines 150-153, 928-935, 3847, 3956-3960, 4169-4187, 4285 (also mixes_in for Ruby).

ACCEPTANCE: a Java fixture with `class B extends A implements I` yields inherits B->A and implements B->I, resolving cross-file.

**Rationale:** Highest structural value per line in the extraction area: one extra field read plus one edge per extractor, reusing the existing bare-name Defs index. Nothing else in the port substitutes for it.

**Minimal change:** Start with java + csharp + python; the rest are copies. Do not add a new index — reuse the global/disambiguate lookup in resolve.go. Skip Ruby mixes_in until someone asks.

### [synth] p1 `build` `core-cli` — Resolve JS/TS alias imports (tsconfig paths, @/ root fallback)

**type** bug · **labels** delta, core-cli, languages-extraction, typescript · **goals** new

WHAT: internal/extract/resolve.go:430 resolveRelImport returns "" for any specifier not starting with `.` or `/`, so every aliased import becomes an external dependency concept node (resolve.go:110-118) instead of an imports_from edge to the real corpus file. In a Next.js/Vite/modern-TS repo `@/components/Foo` is the dominant import form, so the go import graph — and the import-cycle analysis on top of it — is near-empty on exactly those repos.

WHY (agent value): imports_from feeds `affected`, import-cycle analysis and dependency tracing. A blank import graph on TS repos is a silent capability hole, not a cosmetic one.

FILES: internal/extract/resolve.go (Resolve import loop, resolveRelImport); internal/extract/json.go:31 already parses tsconfig.json/jsconfig.json if full paths mapping is needed.

UPSTREAM: graphify/extract.py:1782-1787 (@/ project-root fallback, commit 5aef7f4); graphify/extractors/resolution.py (tsconfig paths/baseUrl; Node subpath imports 2d54b05).

ACCEPTANCE: a corpus with `import { x } from '@/lib/x'` and lib/x.ts at the root yields an imports_from edge, not an external concept node.

**Rationale:** Largest coverage win available for mainstream repos. The @/ fallback alone is a handful of lines and covers most real cases.

**Minimal change:** Ship the @/ -> repo-root fallback first (3 lines, no config parsing); add full tsconfig paths/baseUrl only if a target repo needs it. Skip package.json `imports` (#-prefixed subpaths) — YAGNI.

### [synth] p1 `build` `core-cli` — Namespace the extraction cache by extractor version so a binary upgrade invalidates it

**type** bug · **labels** delta, core-cli, build-incremental-watch, cache · **goals** new

WHAT: the cache file is a bare map[string]Entry of {hash, result} with no version, schema or extractor identity (verified internal/cache/cache.go:27-41). After `brew upgrade graphify-go` ships an extractor fix, every already-cached file keeps its OLD extraction result forever; the fix only reaches files whose content happens to change. Upstream deliberately refuses to read entries written by any other version or layout, storing them under cache/ast/v{version}-s{schema}/.

WHY (agent value): determines whether the graph an agent queries reflects the installed binary's extractors. Silent version skew is the same wrong-answer class as the stat race — and this repo's tip commit is literally 'fix(parity): close 11 gaps', i.e. extractor-output changes that currently reach nobody's existing graph.

FILES: internal/cache/cache.go (on-disk shape, Load/Save signatures), caller in cmd/graphify/main.go (ldflags `version` var at :35); test in internal/cache/cache_test.go.

UPSTREAM: graphify/cache.py:29-43 (_EXTRACTOR_VERSION, _AST_CACHE_SCHEMA), :933-995 (cache_dir, load_cached 'deliberately not consulted').

ACCEPTANCE: a round-trip with a mismatched stamp yields an empty Cache.

**Rationale:** Without it, shipping extractor fixes does not fix anybody's existing graph, which quietly negates the parity work. A version field plus one comparison is far cheaper than upstream's directory-namespacing scheme since go keeps a single file it can simply discard.

**Minimal change:** Change the shape to {"v":"<version>-s1","files":{...}}; Load returns an empty Cache on any mismatch, which already degrades to a full rebuild — no migration path. Pass the stamp into Load/Save rather than importing main. Drop the stat sidecar on mismatch too.

### [synth] p1 `build` `core-cli` — Resolve graphify-out upward from cwd, honour GRAPHIFY_OUT, and record the scan root

**type** feature · **labels** delta, core-cli, build-incremental-watch, agent-first · **goals** new

WHAT: every read command uses the fixed relative literal graphify-out/graph.json (cmd/graphify/main.go:31) and build/update default their root to ".". Verified: `graphify query Affected` from graphify-go/internal fails with 'open graphify-out/graph.json: no such file or directory'; `graphify update` from a subdirectory would silently build a stray second graph rooted there. Git hooks are NOT affected — hookInstall bakes an absolute root into the script (maintenance.go:71) — so this is purely the interactive/agent invocation path.

WHY (agent value): agents change directory constantly and this repo's own conventions mandate absolute paths; a tool that only works from the repo root fails the primary use case. GRAPHIFY_OUT also unblocks the worktree-per-session workflow where several checkouts want distinct or shared output dirs.

FILES: cmd/graphify/main.go (replace defaultGraphPath const with outDir(); route load() and the ~21 non-test "graphify-out" literals across cmd/ and internal/ through it; writeOutputs writes <outDir>/.graphify_root; cmdUpdate reads it when no path arg is given).

UPSTREAM: graphify/paths.py:1-27, 359-376 (GRAPHIFY_OUT, out_path, default_graph_json); graphify/cli.py:2424-2437 (.graphify_root recovery); graphify/watch.py:385,1955,2172.

ACCEPTANCE: query/explain/path succeed from a nested subdirectory; update from a subdirectory updates the repo graph, not a new one.

**Rationale:** Highest ratio of agent-visible breakage to lines of code in this area. The upward walk alone fixes all read commands; .graphify_root is two lines on top.

**Minimal change:** outDir(): honour GRAPHIFY_OUT (absolute or relative name), else walk up from cwd for the first ancestor containing graphify-out/graph.json, else fall back to "graphify-out". Strip a UTF-8 BOM on read (upstream #3028).

### [synth] p1 `build` `core-cli` — Accept ./relative and absolute paths as `affected` seeds

**type** bug · **labels** delta, core-cli, build-incremental-watch, query · **goals** new

WHAT: internal/query/affected.go:40-42 keys the seed set with a bare filepath.ToSlash and matches node.SourceFile by exact string equality (verified). Against this repo, `graphify affected internal/query/affected.go` returns 5 changed nodes while `./internal/query/affected.go` and the absolute path both return 'no graph nodes are defined in those files'.

WHY (agent value): `affected` is a blast-radius primitive an agent calls with whatever path form it already holds — and agents overwhelmingly hold absolute paths. A false 'nothing depends on this' looks like a valid answer; upstream's own commit names the failure: 'a tool answering "nothing depends on this" about a file with sixteen dependents'.

FILES: internal/query/affected.go (normalize in the seed loop — do it in query.Affected, not cmdAffected, so the MCP path in cmd/graphify/serve.go inherits the fix); test in internal/query/affected_test.go.

UPSTREAM: graphify/affected.py:70-100 (_as_repo_relative), :138 (resolve_seed root param); commits a05b408, 243a180 (#2706).

ACCEPTANCE: the same seed in bare, ./-prefixed and absolute form returns the identical result.

**Rationale:** Silent false negative on the most natural calling convention, fixed by a ~10-line normalization helper.

**Minimal change:** TrimPrefix "./"; for filepath.IsAbs make it relative to the graph root (pair with the outDir() bead; until then filepath.Rel against cwd, dropping the seed unchanged if Rel escapes upward). Leave non-path seeds untouched so label matching is unaffected.

### [synth] p1 `build` `core-cli` — Pick the richer node on an ID collision instead of last-write-wins

**type** bug · **labels** delta, core-cli, analysis-quality, correctness · **goals** new

WHAT: model.Graph.AddNode is plain last-write-wins, so a provenance-less node appended later silently destroys a fully-attributed one. Reproduced: a corpus with root-level model.md (markdown concept node id `model`, label 'Model Doc', source_file model.md, L1) plus pkg/a.py containing `import model` (external concept node id `model`, source_file "") yields exactly one node — {'id':'model','label':'model','file_type':'concept','source_file':''}. The documentation node is gone, because internal/extract/resolve.go appends external concept nodes after all per-file nodes (:56-58 then :111) and internal/graph/build.go:20 feeds them to AddNode in order.

WHY (agent value): the collision victim loses label, file_type, source_file and source_location, so `graphify explain <id>` answers '(external)' instead of the real file:line.

FILES: internal/graph/build.go:18-21 only.

UPSTREAM: graphify/dedup.py:419-473 (_collision_rank, _same_source_entity, _merge_missing_attributes); commits 5e6c2be, 3c17238, 5a4b207, 866d503.

ACCEPTANCE: the demo corpus keeps source_file=model.md on the surviving node.

**Rationale:** ~15 lines in one function and it stops the graph silently destroying real provenance.

**Minimal change:** Before g.AddNode(n), if that ID is present keep whichever record has a non-empty SourceFile, filling empty SourceLocation/Label/FileType from the loser only when both share the same SourceFile. No port of _collision_rank's lifecycle/path tiebreaks — go IDs embed the full path. Do NOT renamespace the external-import ID; that would rewrite graph.json IDs and break upstream byte-compat. Honest side effect: the surviving markdown node keeps the a.py--imports-->model edge (slightly wrong), but today we lose the node AND keep the edge, so this is strictly better.

### [synth] p1 `build` `core-cli` — Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs)

**type** feature · **labels** delta, core-cli, agent-interfaces-serving, mcp · **goals** already-tracked

WHAT: upstream serve.py exposes three tools backed by graphify/prs.py — list_prs (open PRs with CI status, review state, and which graph communities each touches), get_pr_impact (files changed by PR #N mapped onto communities plus node count = blast radius), and triage_prs. prs.py shells out to `gh pr list --json` / `gh pr diff --name-only`, matches changed paths against node source_file, folds them into community ids. Go has no prs package and no PR tools (toolHandlers at cmd/graphify/serve.go:369-377 carries 7 tools).

WHY (agent value): 'which open PR already touches the code I am about to change' is a question an agent asks and parses; the graph-to-diff mapping is exactly the value graphify adds over plain gh.

FILES: new internal/prs/prs.go; register three handlers in toolHandlers and three schemas in toolDefs() in cmd/graphify/serve.go, reusing the community map serve already loads.

UPSTREAM: graphify/prs.py (fetch_prs, fetch_pr_files, compute_pr_impact, attach_graph_impact); graphify/serve.py L1954-1998; commits a361228, 09151a6.

ACCEPTANCE: get_pr_impact on a real PR returns the touched communities and a node count.

**Rationale:** Largest genuinely-missing agent capability in the serving area, and GOALS.md already lists 'PR analysis' as a deferred dep-light command — this is that bead, scoped to the agent-facing half.

**Minimal change:** exec.Command("gh",...) with Stdin = nil (upstream a361228 hit stdin hangs), a suffix-comparison pathMatch, and a files->communities fold. Skip render_dashboard/render_conflicts/render_worktrees and triage_with_opus (which shells to the claude CLI) — that is the human dashboard.

### [synth] p1 `build` `core-cli` — Respect edge direction by default in path/shortest_path, add --undirected opt-out

**type** bug · **labels** delta, core-cli, agent-interfaces-serving, query · **goals** new

WHAT: go's bfsPath is undirected-only (internal/query/query.go:263; both Path L155-156 and PathEdges L216-221 document 'undirected'). Upstream now builds a digraph from the true _src/_tgt directions and searches directed by default, with --undirected (CLI) / undirected=true (MCP) opting out and a plain 'no directed route' message instead of a reversed one. Go does render each hop's real direction (e.Forward in toolShortestPath) so the output is not a lie — but the route may traverse call edges backwards, which for a dependency question is the wrong answer dressed as the right one.

WHY (agent value): `path` is one of the four documented agent navigation commands; an agent tracing a call chain gets a route that does not exist as a call chain.

FILES: internal/query/query.go (bfsPath, Path, PathEdges signatures), cmd/graphify/main.go cmdPath (--undirected), cmd/graphify/serve.go (shortest_path schema + toolShortestPath).

UPSTREAM: graphify/serve.py _shortest_path/_tool_shortest_path; commit 94ebee1 (#2487); tests/test_path_cli.py.

ACCEPTANCE: a pair reachable only against edge direction returns 'no directed path; retry with --undirected', and does return a route with the flag.

**Rationale:** Behavioural drift from upstream on a core command; the fix is one bool threaded through one BFS.

**Minimal change:** Default directed=true; skip reverse traversal when set. Emit the retry hint on failure so an agent can self-correct in one step.

### [synth] p2 `build` `core-cli` — Make node-ID normalization idempotent and caseless-stable

**type** bug · **labels** delta, core-cli, languages-extraction, correctness · **goals** new

WHAT: internal/idutil/idutil.go clean() runs NFKC -> non-word filter -> casefold with the fold LAST (verified at :45-49). Upstream's graphify/ids.py runs casefold+NFKC to a FIXPOINT FIRST, then the filter, precisely because casefold can expand a character into a base letter plus a combining mark the filter then never sees. In go, MakeID("İslemYap") = "i̇slemyap" (contains U+0307) and NormalizeID of that returns "i_slemyap" — not idempotent. Greek ypogegrammeni sequences also break caseless stability.

WHY (agent value): NormalizeID reconciles edge endpoints against node IDs, so a non-ASCII identifier can split one entity into two disconnected nodes — upstream's #2614/#811 ghost-node class — breaking query/explain/path for that symbol with no visible error.

FILES: internal/idutil/idutil.go (clean); tests in internal/idutil/idutil_test.go; re-run the determinism goldens.

UPSTREAM: graphify/ids.py.

ACCEPTANCE: NormalizeID(MakeID(x)) == MakeID(x) and MakeID(x) == MakeID(fold(x)) for "İslemYap"; ASCII IDs unchanged.

**Rationale:** Six-line reorder that restores a documented invariant. Narrow blast radius (non-ASCII identifiers only) is why it is p2, not a reason to leave it.

**Minimal change:** Loop `s = norm.NFKC.String(fold.String(s))` up to ~6 times until stable, THEN apply nonWord/underscores/Trim, and drop the trailing fold.

### [synth] p2 `build` `core-cli` — Link single-implementer interface methods to their implementation (dispatches_to)

**type** feature · **labels** delta, core-cli, languages-extraction, edges, blocked · **goals** new

WHAT: upstream's graphify/csharp_dispatch.py adds a dispatches_to edge from an interface's method to the implementing method when the interface has exactly one implementer owning exactly one same-named method. Verified absent in go (no dispatches_to anywhere). Without it, a call through a constructor-injected dependency (_report.Build() where _report is IReport) terminates at the interface node and every directed walk is cut there — on a DI-heavy .NET service that is most chains.

WHY (agent value): call-graph reachability. `path` and `explain` return truncated chains, the failure an agent is least able to detect.

BLOCKED ON: the inherits/implements bead — there are no implements edges to walk today.

FILES: internal/extract/resolve.go or a new dispatch.go in the same package (~40 lines), called from the existing Resolve() pass sequence at :21.

UPSTREAM: graphify/csharp_dispatch.py; graphify/ruby_resolution.py and pascal_resolution.py use the same single-owner pattern.

ACCEPTANCE: a C# fixture with IReport/Report and one Build() yields dispatches_to IReport.Build -> Report.Build; two implementers yield none.

**Rationale:** Build the dispatch pass; do NOT port resolver_registry.py — that registry exists only to tame an 8300-line extract.py, while go's Resolve() is already a small explicit sequence and one more call there is a smaller diff than an interface with one implementation.

**Minimal change:** Index implements-edge targets, keep only interfaces with exactly one implementer, match method labels by bare name, emit dispatches_to with INFERRED confidence. Port the C# case only — ruby_resolution.py and pascal_resolution.py are 520/130 lines for languages where go has no heritage edges and no pascal extractor at all.

### [synth] p2 `build` `core-cli` — Write graph.json and the cache files atomically

**type** bug · **labels** delta, core-cli, build-incremental-watch, correctness · **goals** new

WHAT: internal/export/export.go:122 writes graph.json with a plain os.WriteFile, as do cache.Save/SaveStat and the GRAPH_REPORT.md write in writeOutputs. A reader that opens graph.json mid-write sees a truncated file; a crash or full disk destroys the only copy. Not hypothetical for this design: `graphify hook install` wires post-commit/post-merge/post-checkout to run update, so a rebuild fires while an agent may be running query/explain/ask against the same file.

WHY (agent value): graph.json is the single artifact every agent-facing command reads; a torn or lost read is a hard failure of the primary data path.

FILES: a writeAtomic helper in internal/export (or a tiny internal/fsutil if cache must not import export), swapped into export.go:122, cache.Save, cache.SaveStat, and the GRAPH_REPORT.md write in cmd/graphify/main.go writeOutputs.

UPSTREAM: graphify/paths.py:29-99 (os_replace_with_fallback), :101-196 (_atomic_replace, write_text_atomic, write_json_atomic); commits f38e980, 16fe8f3.

ACCEPTANCE: four call sites use the helper; no behaviour change on the happy path.

**Rationale:** Four call sites, one helper, no happy-path change — and it is the prerequisite that lets the rebuild-lock bead stay skipped.

**Minimal change:** os.CreateTemp in filepath.Dir(path) with a .gfy- prefix, write, Close, os.Rename, defer os.Remove on the error path. Skip upstream's Windows PermissionError/WinError-17 copy fallback — go's os.Rename already maps to MoveFileEx with REPLACE_EXISTING; add it only if a Windows user reports it. Leave export/formats.go and okf.go alone — nothing reads them concurrently.

### [synth] p2 `build` `core-cli` — Ship `graphify install` to register the Claude Code skill and search nudge

**type** feature · **labels** delta, core-cli, build-incremental-watch, agent-first, adoption · **goals** new

WHAT: upstream's install.py registers graphify with the agent host — installs the skill, writes a CLAUDE.md block, and adds PreToolUse hook entries matching "Bash|Grep" and "Read|Glob" so the agent is nudged toward the graph before reaching for grep. In the drift window the nudge moved to Claude Code's dedicated Grep tool (0224bca) and gained an opt-in strict mode plus a guard timeout (689dd6c, 575f829). graphify-go ships skills/graphify/SKILL.md and a CLAUDE.md snippet but has no installer — `graphify hook ...` manages git hooks only (main.go:904). Adoption is entirely manual copy-paste.

WHY (agent value): this is the mechanism by which an agent actually uses the graph instead of grepping. Under an agent-first framing it is closer to the product than most commands.

FILES: new cmd/graphify/install.go; go:embed of skills/graphify/SKILL.md; reuse the hookMarker idempotency pattern from cmd/graphify/maintenance.go.

UPSTREAM: graphify/install.py:325-360 (PreToolUse matchers), :1152-1160; commits 0224bca, 689dd6c, 575f829, 05ee568.

ACCEPTANCE: install copies the skill and merges one tagged PreToolUse entry; re-running is a no-op; uninstall removes exactly that entry.

**Rationale:** The repo's whole value proposition is 'agent queries the graph instead of grepping', and nothing currently makes that happen for a new user. GOALS.md excludes 'multi-assistant installers' — this is deliberately Claude Code only, so it is not that.

**Minimal change:** `graphify install [--global]`: copy the embedded SKILL.md to ~/.claude/skills/graphify/ and merge ONE PreToolUse entry with matcher "Bash|Grep" into ~/.claude/settings.json. Non-negotiable: refuse to write if settings.json exists and does not parse (upstream 05ee568 is a clobbered-settings postmortem). Skip the CLAUDE.md-block registration — the skill covers it. Do not chase upstream's Codex/Antigravity/Codebuddy matrix or its settings-backup machinery.

### [synth] p2 `build` `core-cli` — Filter JSON key nodes and builtin/framework labels out of god nodes

**type** bug · **labels** delta, core-cli, analysis-quality · **goals** new

WHAT: upstream god_nodes drops two extra classes of mechanical hub before ranking — generic JSON key labels when the source file is .json (_JSON_NOISE_LABELS: name, id, type, version, dependencies, devDependencies, properties, items...) and stdlib/framework identifiers (_BUILTIN_NOISE_LABELS: str, int, Path, Optional, os, sys, json...). go's GodNodes (internal/analyze/analyze.go:26-39) only excludes file-hub and concept nodes, and isConceptNode() returns false for package.json keys because the file has an extension.

WHY (agent value): god nodes are the 'core abstractions' answer an agent asks for when orienting in an unfamiliar repo; a ranking topped by `dependencies` from package.json or `Path` from a Python import is a wrong answer.

HONESTY: no corpus was found where this visibly polluted go's top-10 (this repo's own report is clean) — the impact is inferred from the extractor shape, not measured. Both upstream filters predate v0.9.17, so this is older drift the last parity audit missed, not new drift.

FILES: internal/analyze/analyze.go only; tests in internal/analyze/analyze_test.go.

UPSTREAM: graphify/analyze.py:11-24, :91-107, :133-137.

ACCEPTANCE: one case per noise set shows the node excluded from the ranking.

**Rationale:** ~25 lines including the two sets, in one file, on the first command an agent runs against an unfamiliar repo.

**Minimal change:** Two package-level map[string]bool literals plus two continue guards (lowercase the label; check strings.HasSuffix(SourceFile, ".json") for the JSON set). Drop the Swift/Foundation/SwiftUI entries — go has no Swift extractor.

### [synth] p2 `build` `core-cli` — Let serve target a graph other than the cwd default (--graph flag + per-tool project_path)

**type** feature · **labels** delta, core-cli, agent-interfaces-serving, mcp · **goals** new

WHAT: dispatch is `case "serve": err = cmdServe(defaultGraphPath)` (verified cmd/graphify/main.go:79-80) — no arguments accepted at all — and no tool schema carries project_path (cmd/graphify/serve.go:374-407). Upstream's serve takes a positional graph path plus a --graph alias and injects an optional project_path into EVERY tool schema, so one running server answers about any project containing graphify-out/graph.json, with an LRU-bounded context cache (GRAPHIFY_MAX_CONTEXTS, default 8).

WHY (agent value): an agent working across a multi-repo workspace has exactly one MCP server configured; without project_path it can only ever see the repo the server was launched in.

FILES: cmd/graphify/main.go:79 (pass os.Args[2:] to cmdServe, parse positional path + --graph); cmd/graphify/serve.go (one project_path insertion in the toolDefs() loop since all schemas are built there; pop it in callTool L166-190 and swap s.g/s.god/s.communities).

UPSTREAM: graphify/serve.py _main argparse L2619-2660, project_path injection L2002-2020, _select_graph; commit b4865ff (#2268).

ACCEPTANCE: serve launched in repo A answers a query_graph carrying project_path=B about B.

**Rationale:** Small and it removes a hard ceiling on multi-repo agent sessions.

**Minimal change:** Lazily-loaded plain map of loaded graphs with a hard size cap — do NOT port the LRU until someone actually serves more than 8 projects.

### [synth] p2 `build` `core-cli` — Add `graphify save-result` to write Q&A memory docs back into the graph

**type** feature · **labels** delta, core-cli, agent-interfaces-serving, agent-first, memory · **goals** new

WHAT: `graphify save-result --question Q --answer A [--type T] [--nodes N...] [--outcome useful|dead_end|corrected] [--correction TEXT]` writes a markdown file with YAML frontmatter into graphify-out/memory/, which the markdown extractor picks up on the next update — so what an agent learned from a query becomes graph content for the next session. Filenames carry a uuid segment to survive concurrent saves. Go has no save-result case and no memory/ convention.

WHY (agent value): this is the agent WRITE path; every other go command is read-only. It is the mechanism by which an agent's findings persist, and it unblocks the reflect bead.

FILES: new ~70-line cmd/graphify/save_result.go; register `case "save-result"` in cmd/graphify/main.go's switch (~L79) and one usage line (~L913); reuse internal/security's label sanitizer for YAML string escaping.

UPSTREAM: graphify/ingest.py save_query_result L275-330 (OUTCOMES); graphify/cli.py L1461-1490; commit 2f743ae (unique filenames for concurrent saves).

ACCEPTANCE: save-result then `graphify update` then `graphify query <slug>` finds the new node.

**Rationale:** Go already ships the markdown extractor that consumes these files, so the whole feature is one small file writer — highest value-per-line item in the serving area.

**Minimal change:** Just build the frontmatter block and write graphify-out/memory/query_<ts>_<8 hex>_<slug>.md. No new package, no index, no reader — reflect is a separate, later bead.

### [synth] p2 `build` `core-cli` — Reconcile GRAPH_REPORT.md headline community counts with the render loop

**type** bug · **labels** delta, core-cli, outputs-human-viewers, report · **goals** new

WHAT: internal/report/report.go:31 prints len(communities) raw for the Summary line and :75 prints len(communities) again for the 'Communities (N total)' header, while the thin-community guards at :111 and :133 use a separately-computed realLabels()/minCommunitySize check. That is the same 'two different predicates for the same figure' shape upstream had to fix twice (#2129 residual, then #3548): a community with zero real nodes is excluded from the render but still counted in the headline.

WHY (agent value): GRAPH_REPORT.md is the audit-trail artifact an agent reads alongside graph.json to sanity-check coverage. Internal consistency of its headline numbers is correctness of a CLI-consumed output, not a visualization concern.

FILES: internal/report/report.go only.

UPSTREAM: graphify/report.py _real_count (fce26fc, #3148 and follow-up #3548); tests/test_report_gap_thresholds.py.

ACCEPTANCE: a corpus with an empty community reports the same N in the Summary line, the section header, and the rendered list.

**Rationale:** One helper, one file, mirrors an upstream fix already validated by tests, and avoids a report that visibly contradicts itself.

**Minimal change:** Add one realCount(communities) helper reusing the existing realLabels() logic and use it in both places instead of len(communities).

### [synth] p2 `build` `core-cli` — Surface unclassified-file corpus coverage in GRAPH_REPORT.md

**type** feature · **labels** delta, core-cli, outputs-human-viewers, report · **goals** new

WHAT: upstream adds an 'Unclassified' line to the Corpus Check section naming the count of files detect() saw but could not classify (no supported extension/shebang) plus the biggest offending extensions, so a corpus mostly in an unsupported language does not get the same 'well covered' verdict as a fully-extracted one. Go's report never surfaces this even though internal/detect/detect.go:315 already tracks the count (mirrored in detect_test.go:204).

WHY (agent value): coverage-confidence signal for an agent deciding whether to trust the graph for a given repo — a self-diagnostic, not decoration.

FILES: thread the existing unclassified count/extension breakdown from internal/detect/detect.go through to report.Generate and add one line to the existing Corpus Check section in internal/report/report.go.

UPSTREAM: graphify/report.py (c9da36d, toward #3511); graphify/detect.py.

ACCEPTANCE: a corpus with 40 .swift files reports them as unclassified with the extension named.

**Rationale:** Pure wiring — the data already exists in go, report.go just never consumes it. No new detection logic.

**Minimal change:** One line of report output plus one field through the existing call. Do not add a new detect pass.

### [synth] p3 `build` `core-cli` — Widen markdown code-span mentions to qualified symbols

**type** feature · **labels** delta, core-cli, languages-extraction, markdown · **goals** new

WHAT: resolveMarkdown (internal/extract/resolve.go:194) resolves a backtick span to a code definition only when the span is a bare identifier — mdCodeSymbol at :187 is ^[A-Za-z_][A-Za-z0-9_]*$ — so every dotted or path-qualified span is discarded. Upstream's markdown_resolution.py (new in this window, c805c6f) resolves dotted mentions (pkg.Widget, Widget.render) by requiring every qualifier to appear on the candidate's contains/method owner chain or in its source path, which is also what keeps time.sleep off a repo's own sleep.

WHY (agent value): doc-to-code `references` edges answer 'which docs describe this function', a real agent query, and go already ships the edge — this is recall on an existing capability.

FILES: internal/extract/resolve.go (mdCodeSymbol at :187, uniqueCodeDef at :264).

UPSTREAM: graphify/markdown_resolution.py; commits c805c6f, 8cfebfd, 3eb2486.

ACCEPTANCE: `pkg.Widget` in a doc resolves to pkg's Widget and does NOT resolve when the qualifier does not match.

**Rationale:** Current behaviour is under-inclusive but not wrong, so this is recall improvement rather than a correctness fix — hence p3.

**Minimal change:** Widen mdCodeSymbol to allow . and ::, split on the separator, match the last segment, keep a candidate only when each preceding qualifier is a label on its owner chain or a path segment of its source file. Skip the path-cited file::Sym::Sym form and the builtin table — long tails of a 231-line upstream module, and the qualifier check already covers the time.sleep case they were added for.

### [synth] p3 `build` `core-cli` — Skip the post-checkout rebuild when HEAD did not move

**type** bug · **labels** delta, core-cli, build-incremental-watch, hooks · **goals** new

WHAT: hookInstall writes one identical script to all three managed hooks (cmd/graphify/maintenance.go:27, :71): `exec graphify update <abs-root>`. git invokes post-checkout with $1=prev HEAD, $2=new HEAD, $3=1 for a branch checkout / 0 for a file checkout — so `git checkout -- somefile`, `git checkout .`, and re-checking-out the current branch each trigger a full corpus walk plus rebuild.

WHY (agent value): behaviour of the installed hook that keeps the agent's graph fresh. Cheap here, not free: `graphify update .` measures 0.087s for 360 files, but it is an unconditional whole-tree walk on commands run in tight loops.

FILES: cmd/graphify/maintenance.go hookInstall; test following the cmd/graphify/hook_mergedriver_status_test.go pattern.

UPSTREAM: graphify/hooks.py; commit 5c88af7.

ACCEPTANCE: the post-checkout script contains the guards; post-commit and post-merge do not.

**Rationale:** One line of shell removing a whole class of pointless rebuilds. Deliberately excluding the rest of upstream's hook hardening (detached launcher, SIGALRM watchdog, killing orphaned workers) — that machinery exists to stop a multi-minute Python rebuild hanging a commit, and at 87ms go does not have that problem.

**Minimal change:** Stop writing one script for all three. Keep the current body for post-commit/post-merge; for post-checkout prepend `[ "$1" = "$2" ] && exit 0` and `[ "$3" = "0" ] && exit 0`, keeping hookMarker first so the existing 'not written by graphify' guard and hookStatus keep working.

### [synth] p3 `build` `core-cli` — Sanitize computed_name metadata before it enters graph.json

**type** bug · **labels** delta, core-cli, analysis-quality, security · **goals** new

WHAT: go has security.SanitizeLabel (control chars + 256-char cap) but never applies it to ComputedName, which carries raw markdown frontmatter description/tags and Terraform null-label values. internal/extract/markdown.go:68 passes computedMeta(fm) straight through, and computedMeta (:179-188) joins the raw frontmatter description and tags with no cap. By contrast json.go:93 and mcpconfig.go:68 DO wrap labels in SanitizeLabel, so this is an inconsistency, not a policy.

WHY (agent value): computed_name is emitted into graph.json and surfaces in query/ask/explain output an agent parses; unbounded untrusted text there is a context-budget and output-integrity problem.

FILES: internal/extract/markdown.go:68 and internal/extract/nulllabel_resolve.go:244 — wrap both ComputedName assignments.

UPSTREAM: graphify/security.py:412-461 (sanitize_metadata); callers at graphify/extract.py:753, 813.

ACCEPTANCE: a 10KB frontmatter description is capped and control-stripped in graph.json.

**Rationale:** One-line fix at two sites, small blast radius. Hygiene, not a reproduced correctness bug — hence p3.

**Minimal change:** No sanitize_metadata port: go's ComputedName is a flat string, not a nested map, so SanitizeLabel covers control chars and length. Skip the HTML escaping upstream does — go dropped the HTML viewer, so there is no HTML sink.

### [synth] p3 `build` `core-cli` — Add context/relation filtering to ask and MCP query_graph

**type** feature · **labels** delta, core-cli, agent-interfaces-serving, query · **goals** new

WHAT: upstream's query_graph accepts context_filter: ["call","field",...] and the CLI accepts repeatable --context, restricting traversal to those edge relations so an agent can ask 'only the call graph around X' without import/contains noise. go's query.Ask(g, question, dfs, depth, tokenBudget) (internal/query/ask.go:21) takes no filter, the comment at ask.go:20 states 'context-filter machinery is intentionally omitted', cmdAsk has no --context (main.go:493), and the MCP schema omits it (serve.go:378-382).

WHY (agent value): relation filtering is context-window economy — it narrows a subgraph BEFORE the token budget has to truncate it, which is strictly better than truncating.

FILES: internal/query/ask.go (relations []string param, skip non-matching links during expansion), cmd/graphify/main.go cmdAsk (repeatable --context, copying the existing --relation loop in cmdAffected at L719-724), cmd/graphify/serve.go (context_filter array on query_graph + toolQueryGraph).

UPSTREAM: graphify/serve.py _query_graph_text context_filters, tool schema L1874-1880; graphify/cli.py query L1202-1240.

ACCEPTANCE: --context calls on a node with import and call neighbours returns only the call ones.

**Rationale:** The go omission is documented as deliberate, but it was a simplification, not a design decision — upstream's flag is a one-parameter filter on an edge loop, not 'machinery'. Update that comment when landing.

**Minimal change:** One slice parameter and one membership check. Reuse cmdAffected's existing repeatable-flag parsing rather than adding a flag package.

### [synth] p3 `build` `core-cli` — Ingest Cargo.toml through the canonical package-manifest path

**type** bug · **labels** delta, core-cli, agent-interfaces-serving, extraction · **goals** new

WHAT: manifestEcosystems covers only pyproject.toml, go.mod and pom.xml (internal/extract/manifest.go:20-24). Rust is handled instead by internal/extract/cargo.go, opt-in behind --cargo, using a separate crate:<name> id namespace with FileType 'code' (cargo.go:58,71) and explicitly skipping registry deps ('registry dep or unknown crate — not an internal edge', cargo.go:96). So external crate dependencies are invisible and Cargo.toml contributes nothing on a default build. Upstream added cargo.toml to PACKAGE_MANIFEST_NAMES so a crate always emits one canonical pkg:<name> node plus depends_on edges to every declared dependency.

WHY (agent value): dependency edges are structural graph data an agent traverses, identical in kind to the go.mod/pyproject edges go already emits.

FILES: internal/extract/manifest.go — add "cargo.toml": "cargo" to manifestEcosystems and a parseCargo alongside parseGoMod/parsePyproject, reusing the loadTOML decoder already in internal/extract/cargo.go.

UPSTREAM: graphify/manifest_ingest.py PACKAGE_MANIFEST_NAMES L28-35, _parse_cargo L228; commit 638d9e2 (#2434).

ACCEPTANCE: a default build of a Rust crate emits pkg:<name> with depends_on edges to registry deps.

**Rationale:** Closes an inconsistency go created itself: every other ecosystem's manifest is always-on and canonical, Rust's is opt-in and namespaced apart.

**Minimal change:** Leave cargo.go's --cargo crate graph alone — it answers a different question (internal workspace topology). Skip apm.yml; manifest.go:17-19 already documents that omission as deliberate (no YAML dependency).

### [synth] p3 `build` `core-cli` — Expose surprising connections through serve

**type** feature · **labels** delta, core-cli, agent-interfaces-serving, mcp · **goals** new

WHAT: upstream's MCP server implements the resources protocol with six URIs (report, stats, god-nodes, surprises, audit, questions); go advertises only tools in its initialize capabilities (cmd/graphify/serve.go:131-136) and answers resources/list with -32601. Four of the six are already reachable in go as tools (graph_stats covers stats+audit, god_nodes covers god-nodes) or as a file (GRAPH_REPORT.md), but analyze.Surprising (internal/analyze/analyze.go:62) is computed at build time and never reachable from serve at all.

WHY (agent value): surprising cross-community edges are a discovery primitive — 'what couples these two subsystems unexpectedly' — which an agent cannot reach via query/explain because it does not know the names to ask for.

FILES: cmd/graphify/serve.go — one surprising_connections entry in toolDefs() and toolHandlers calling analyze.Surprising(s.god, s.communities, topN) (analyze is already imported for toolGodNodes). ~15 lines.

UPSTREAM: graphify/serve.py list_resources L2271-2282, read_resource L2283-2330; graphify/analyze.py surprising_connections.

ACCEPTANCE: the tool returns the same list GRAPH_REPORT.md shows.

**Rationale:** Build the one genuinely-unreachable analysis as a TOOL; do NOT implement the resources protocol — most MCP clients surface tools automatically and resources only on explicit user action, so resources are the less agent-accessible half of the surface.

**Minimal change:** Skip resources/list + resources/read entirely, and skip suggest_questions — go has no equivalent and an agent forms its own questions.

### [synth] p3 `skip` `core-cli` — Skip the rebuild lock and .pending_changes queue for concurrent updates

**type** chore · **labels** delta, core-cli, build-incremental-watch, skip · **goals** new

WHAT: upstream guards every rebuild with an fcntl.flock advisory lock on graphify-out/.rebuild.lock (PID inside for external pollers) and, when contended, queues changed paths into .pending_changes that the lock-holder drains over up to 20 passes. go has neither (grep for flock/lock/pending under cmd/ and internal/ returns only dependency lock-file names in detect.go), so two concurrent `graphify update` runs race on graph.json and the cache file.

WHY SKIPPED: the queue exists upstream because their rebuild is slow and PARTIAL — a dropped rebuild loses the specific files that changed. go re-assembles the entire corpus from cache on every update (measured 87ms for 360 files), so a racing rebuild loses nothing: last writer wins and both writers compute the same graph. Once writes are atomic (see that bead), the remaining exposure is a torn cache file, which cache.Load already handles by degrading to a full rebuild.

UPSTREAM: graphify/watch.py:22-82 (_queue_pending, _drain_pending), :166-224 (_rebuild_lock).

REVISIT WHEN: update time on a large corpus grows past a few seconds.

**Rationale:** Depends on the atomic-writes bead landing; with that, the failure this machinery prevents does not exist in go's whole-corpus design.

**Minimal change:** If ever needed: syscall.Flock(LOCK_EX|LOCK_NB) on <outDir>/.rebuild.lock at the top of cmdUpdate, exiting 0 when contended. Do not port .pending_changes — it is meaningless for whole-corpus reassembly.

### [synth] p3 `skip` `core-cli` — Do not port MinHash/LSH fuzzy near-duplicate entity merging

**type** chore · **labels** delta, core-cli, analysis-quality, skip · **goals** new

WHAT: upstream dedup.py runs a three-pass fuzzy entity merge — exact label normalization, then MinHash+band-LSH blocking (_minhash.py, a scipy-free datasketch clone) over an entropy-gated candidate set, then Jaro/Jaro-Winkler at threshold 92 with a same-community boost and five false-merge blockers, union-find merge, and edge/hyperedge rewiring. go has none of it.

WHY SKIPPED: upstream's own gates make this near-empty for go. dedup.py:253-265 (_is_code) excludes every file_type==code node from both passes, and :730-760 gates cross-file exact merges to concept nodes with a non-empty source_file. Every non-code node go emits is already uniquely keyed: markdown concept IDs are the repo-relative path (markdown.go:52), headings are MakeID(conceptID,title) with a line-number disambiguator (markdown.go:124-127), rationale nodes are MakeID(stem,"rationale",line) (extract.go:274), and external concept nodes from resolve.go:113/161 have source_file=="" so upstream's provenance gate would refuse them anyway. That is ~600 lines of port whose failure mode is destroying distinct nodes, for roughly nothing to merge — and 11 of the post-v0.9.17 dedup commits are fixes for over-merges this design causes.

UPSTREAM: graphify/dedup.py (esp. :253-265, :515-900); graphify/_minhash.py.

DO INSTEAD: the exact-ID survivorship bead — that is the part of dedup.py that actually applies to go.

**Rationale:** Classified core-cli precisely so the skip is meaningful: it is in scope as a build-pipeline quality stage and still not worth building.

**Minimal change:** Nothing to write. If an LLM/semantic path ever lands in internal/semantic and emits label-keyed nodes without stable IDs, revisit — and then port only Pass 1 (exact normalized label, same source_file) into internal/graph/build.go, not the LSH machinery.

### [synth] p4 `build` `core-cli` — Add exclude_hubs_percentile to the god_nodes MCP tool

**type** feature · **labels** delta, core-cli, agent-interfaces-serving, mcp · **goals** new

WHAT: upstream's god_nodes tool takes exclude_hubs_percentile (0-100), suppressing nodes whose degree exceeds that percentile so the result matches the hub exclusion cluster() applies — without it the top-N is dominated by the same mega-hubs every time. go's schema exposes only top_n (cmd/graphify/serve.go:395-397) and analyze.GodNodes(g, topN) takes no percentile. go does have file-hub exclusion (TestGodNodesExcludeFileHubs, analyze_test.go:48) but it is not caller-tunable.

WHY (agent value): god_nodes is the orientation tool an agent calls first on an unfamiliar repo; being able to look past the obvious hubs refines the first result.

FILES: internal/analyze/analyze.go:26 (percentile param, 0 = off), cmd/graphify/serve.go:395 schema + :279 handler passing argInt(args,"exclude_hubs_percentile",0); update existing GodNodes call sites in internal/report.

UPSTREAM: graphify/serve.py tool schema L1922-1930, _tool_god_nodes; commit 6f9b79a (#3205).

**Rationale:** Small but low leverage — go's built-in file-hub exclusion already removes the worst offenders, so this only refines an already-usable result. Bundle it with whichever serve.go change lands first rather than scheduling it alone.

**Minimal change:** Sort degrees, compute the cutoff, drop above it. Zero means off so every existing call site passes 0 and behaviour is unchanged.

### [synth] p4 `skip` `core-cli` — Skip the cross-repo global graph and its call/type join passes

**type** chore · **labels** delta, core-cli, build-incremental-watch, languages-extraction, skip · **goals** already-tracked

MERGED from two area reports (global graph; cross-repo call/type joins).

WHAT: global_graph.py maintains a machine-wide graph at ~/.graphify/global-graph.json with a side manifest tracking each registered repo by tag and file hash (global_add/global_remove/global_list/global_path). Two merge-time passes ride on it: cross_repo_calls.py parks unresolvable member calls on the caller as metadata.unresolved_calls and finishes them after graphs are composed (#3152, c604f48), and cross_repo_types.py links identically-declared types across repos with a same_type_as edge (#3007). Both run only from global_graph.py and cli.py's merge-graphs / global add. go has no global surface of any kind and nothing references same_type_as or unresolved_calls; its only merge-adjacent feature is the graph.json union merge DRIVER for git (cmd/graphify/maintenance.go:100), which solves an unrelated problem.

WHY SKIPPED: not drift — equally absent at the v0.9.17 baseline, so it is a feature decision, not a parity regression. The two passes are ~320 lines that cannot run without first porting merge-graphs and global add (global manifest lifecycle, per-repo tagging/hashing, cross-repo id namespacing). GOALS.md already lists `graphify global` as a deferred dep-light command.

UPSTREAM: graphify/global_graph.py:79-193; graphify/cross_repo_calls.py; graphify/cross_repo_types.py; commit c604f48 (#3152).

**Rationale:** Largest surface in the delta for a use case nothing in GOALS.md or actual usage has raised. Flagged so a future audit does not rediscover it as new.

**Minimal change:** Deliberately none. If a concrete multi-repo need appears, the cheap first move is `graphify query --graph <path>` against several graphs and merging in the caller, not a stateful ~/.graphify registry. Do not speculatively add the unresolved_calls field to resolve.go.

### [synth] p4 `skip` `core-cli` — Skip `graphify reflect` until saved memory docs exist

**type** chore · **labels** delta, core-cli, agent-interfaces-serving, skip, blocked · **goals** new

MERGED from two area reports (reflect skip; reflect build-blocked).

WHAT: graphify/reflect.py (~880 lines) is a deterministic, LLM-free pass over graphify-out/memory/ that scores each cited source node with a time-decayed signed value (useful positive; dead_end/corrected negative, half-life so a fresh dead end outweighs a stale useful) and writes graphify-out/reflections/LESSONS.md partitioned into preferred / tentative / contested sources, known dead ends, and corrections, plus a .graphify_learning.json sidecar. It is genuinely agent-facing — the artifact is meant to be loaded at session start.

WHY SKIPPED NOW: it is the read half of a memory loop whose write half does not exist. go has no save-result and no memory/ directory, so there is nothing to aggregate; porting the aggregator alone produces a command with no input.

UNBLOCK WHEN: the save-result bead has landed AND memory docs have actually accumulated in a real session.

FILES (when built): new internal/reflect parsing the YAML frontmatter internal/extract/markdown.go already handles, plus `case "reflect"` in cmd/graphify/main.go.

UPSTREAM: graphify/reflect.py (module docstring L1-18, parse_memory_doc, load_memory_docs); graphify/cli.py L1492-1548.

**Rationale:** Classified core-cli — LESSONS.md is written for the next agent session, not a person — but strictly downstream of save-result and by far the larger build. Sequence it after, and only on evidence that docs accumulate.

**Minimal change:** When built, start with plain counts, not upstream's half-life decay; add decay only once contested nodes actually appear.

### [synth] p4 `skip` `core-cli` — Skip persisting corpus-shaping options across incremental rebuilds (--exclude, --no-gitignore)

**type** chore · **labels** delta, core-cli, build-incremental-watch, skip · **goals** new

WHAT: upstream writes .graphify_build.json into the output dir recording the build's --exclude patterns and gitignore setting, and update/hook rebuilds read it back so an incremental rebuild scans the same corpus the original build did. The --no-gitignore opt-out itself landed in this window (f17e2c5). go has neither side: parseBuildOpts (cmd/graphify/main.go:367) accepts only --cargo/--no-manifests/--force/--no-cluster/--semantic/--backend, and detect always applies its .gitignore matcher plus a hardcoded skip-dir list (internal/detect/detect.go:328, :388) with no override.

WHY SKIPPED: the persistence half only has value once the flags exist, and the flags are speculative — no gap report or use case here asks to graph gitignored code or exclude a subtree, and detect's built-in skip list already covers the vendor/build directories that motivate most excludes. A config file plus two flags plus their cache interaction is real surface for a demand that has not appeared.

UPSTREAM: graphify/watch.py:84-143 (_write_build_config, _read_build_excludes, _read_build_gitignore); commit f17e2c5.

**Rationale:** Build it when someone asks — and build both halves at once so update can never disagree with build.

**Minimal change:** If demanded: --exclude (repeatable glob) and --no-gitignore in parseBuildOpts, threaded into detect.CollectFilesReport, with writeOutputs persisting them to <outDir>/.graphify_build.json that cmdUpdate reads when the flags are absent. ~60 lines across cmd/graphify/main.go and internal/detect/detect.go.

### [synth] p4 `skip` `core-cli` — Do not port `graphify diagnose` (MultiDiGraph-readiness report)

**type** chore · **labels** delta, core-cli, build-incremental-watch, skip · **goals** already-tracked

WHAT: diagnostics.py provides diagnose_file/diagnose_extraction behind `graphify diagnose <graph.json> [--json] [--directed|--undirected] [--extract-path P]`, reporting how many edge pairs would collapse under a simple DiGraph, which producer call sites suppress duplicate edges, and (drift, 6e761f2) counting external reference edges separately from internal ones. It is scaffolding for upstream's own migration to parallel-edge support.

WHY SKIPPED: the report answers a question that exists only while upstream migrates its graph model to MultiDiGraph. go's model (internal/model, internal/graph) was written clean-room and does not face that migration. `graphify validate` already covers the structural checks an agent or CI actually needs (dangling edges, duplicate/empty ids). Porting this is parity theatre. GOALS.md already lists diagnostics as marginal and deferred.

UPSTREAM: graphify/diagnostics.py:156-421; graphify/cli.py:1924-1945; commit 6e761f2.

**Rationale:** Structured JSON output so not human-only by shape, but its subject matter is upstream's internal graph-model migration, which go does not share.

**Minimal change:** None. If a specific diagnostic is ever wanted, add the single counter to internal/export/validate rather than porting a 422-line module.

### [synth] p4 `skip` `human-only` — Do not port `graphify benchmark`

**type** chore · **labels** delta, human-only, analysis-quality, skip · **goals** new

WHAT: upstream `graphify benchmark` estimates corpus tokens (nodes x 50 words, or detect()'s word count) against the token cost of a BFS query subgraph for five canned questions, and prints a box-drawn 'Nx fewer tokens per query' report. go has no equivalent.

WHY SKIPPED: run_benchmark's only consumer is print_benchmark (cli.py:3149-3150), a unicode-box human report with no JSON path. The number is a marketing ratio derived from a 50-words-per-node guess (benchmark.py:104-106), unfalsifiable by construction. An agent never needs to be told how many tokens it saved — it either gets the subgraph it asked for or it does not. Two of the four post-v0.9.17 touches are crash fixes for inputs go would never produce (#2674 None label, #2212 no-cluster edges key).

UPSTREAM: graphify/benchmark.py; graphify/cli.py:3135-3150.

**Rationale:** Human-only by consumer and by shape; nothing to remove since it was never built here.

**Minimal change:** No change. If a token-cost number is ever wanted, `graphify ask --budget N` already reports what it emitted — measure that.

### [synth] p4 `skip` `core-cli` — Do not port the Streamable HTTP MCP transport

**type** chore · **labels** delta, core-cli, agent-interfaces-serving, skip, security · **goals** already-tracked

WHAT: upstream serves the identical tool/resource surface over MCP Streamable HTTP as well as stdio: serve_http runs a Starlette ASGI app under uvicorn with a pure-ASGI API-key gate (X-API-Key or Bearer, constant-time compare), DNS-rebinding Host protection, configurable mount path, --json-response, --stateless, and --session-timeout session reaping. go's serve is stdio-only: run() reads newline-delimited JSON-RPC from an io.Reader (cmd/graphify/serve.go:101-120) and cmdServe accepts no transport flag.

WHY SKIPPED: every capability HTTP exposes is already reachable over stdio, which is how agent clients launch a local binary — the tool surface an agent sees is identical either way. The cost is disproportionate: session management, SSE streaming, auth, and DNS-rebinding defences. An auth-bearing network listener is the one component here where a shortcut is a security bug. GOALS.md already records serve as 'Stdio-only (HTTP/api-key/hot-reload skipped)'.

UPSTREAM: graphify/serve.py _build_http_app L2479-2560, serve_http L2561-2617, _ApiKeyMiddleware L2437-2477, _MCPASGIApp L2412-2435.

REVISIT WHEN: someone actually needs a shared multi-user server.

**Rationale:** Deployment topology, not a navigation capability. Not human-facing — which is why it is classified core-cli and skipped on cost, not on framing.

**Minimal change:** If ever needed, prefer net/http with a single POST handler returning JSON (upstream's --json-response mode) over reimplementing SSE sessions, and require the API key unconditionally rather than making it opt-in as upstream does.

### [synth] p4 `skip` `core-cli` — Do not port querylog

**type** chore · **labels** delta, core-cli, agent-interfaces-serving, skip, privacy · **goals** new

WHAT: graphify/querylog.py appends an opt-in JSONL record per query/path/explain (timestamp, kind, question, corpus path, nodes_returned, duration_ms, optionally the full response) to ~/.cache/graphify-queries.log, gated behind GRAPHIFY_QUERY_LOG / GRAPHIFY_QUERY_LOG_ENABLE. Confirmed absent in go (grep for querylog|QUERY_LOG over cmd/ and internal/ returns nothing).

WHY SKIPPED: machine-written data, but nothing in either codebase consumes it — `graphify reflect` reads memory docs, not this log. Upstream's own comment calls it a plaintext record of proprietary queries living outside any repo's .gitignore, and it is off by default precisely because it conflicts with the no-telemetry posture. A write-only artifact with no reader is not a capability.

UPSTREAM: graphify/querylog.py (_log_path L15-32 documents the opt-in rationale); call sites in serve.py _tool_query_graph and cli.py query.

**Rationale:** No reader, plus a privacy footgun. If query analytics are ever wanted they should feed reflect, not a loose cache file.

**Minimal change:** No change.

### [synth] p4 `skip` `out-of-scope` — Do not port semantic_cleanup

**type** chore · **labels** delta, out-of-scope, analysis-quality, skip · **goals** out-of-scope-per-goals

WHAT: upstream semantic_cleanup.py post-processes LLM-extracted semantic nodes and chunk merges, including validating untrusted subagent chunk JSON before merge-chunks folds it in. Its only post-v0.9.17 commit (ee74d80) hardens that chunk-JSON validation.

WHY SKIPPED: the module exists solely to clean up after LLM-based semantic extraction and the subagent chunk pipeline, both of which GOALS.md lists as intentionally not ported. go has no merge-chunks command and no LLM extraction path, so there is no input for this stage. Note internal/semantic exists but is a Bedrock enrichment path, not the chunked subagent extraction semantic_cleanup services.

UPSTREAM: graphify/semantic_cleanup.py; commit ee74d80.

**Rationale:** Out of scope per GOALS.md and structurally inputless in go.

**Minimal change:** No change.

### [synth] p4 `skip` `out-of-scope` — Do not port scip_ingest

**type** chore · **labels** delta, out-of-scope, agent-interfaces-serving, skip · **goals** new

WHAT: graphify/scip_ingest.py converts a simplified SCIP-style JSON document set (documents/symbols/relationships/occurrences) into graphify nodes and edges, resolving relationship targets through a symbol index or emitting external stubs.

WHY SKIPPED: it is dead code upstream — 360 lines with no CLI entry point and no caller; its own module docstring says 'Not wired to the CLI in this phase', and grep of cli.py/__main__.py finds no import. It is also NOT the official SCIP protobuf format, so it buys no interoperability with real SCIP indexers, and the shape it consumes is explicitly the one 'LLM-generated SCIP-style JSON commonly produces' — an LLM-mediated path GOALS.md excludes. Porting an unreachable module for parity would be pure cost.

UPSTREAM: graphify/scip_ingest.py L1-40.

REVISIT WHEN: real SCIP interop is wanted — and then target the actual protobuf schema from a language indexer, not this skeleton.

**Rationale:** Unreachable upstream and not the real format; GOALS.md does not name it, so tracking it here stops a future audit rediscovering it.

**Minimal change:** No change.

### [synth] p4 `skip` `out-of-scope` — Do not port the remaining long-tail language extractors

**type** chore · **labels** delta, out-of-scope, languages-extraction, skip · **goals** out-of-scope-per-goals

WHAT: upstream ships 16 extractors go lacks — apex, blade, commonlisp, dart, dm, elixir, fortran, objc, ocaml, pascal (+pascal_forms), powershell, razor, robot, sln, sql, swift. Several landed in this window (832256f Robot Framework, 038ada6 Common Lisp, 0302bfa OCaml). go covers the ~24 mainstream languages listed in GOALS.md.

CHECKED AND NOT A GAP: go already captures scala traits/objects/enums (internal/extract/scala.go:33) and php interface/trait/enum (internal/extract/php.go:32), the two node-kind fixes upstream landed in this window (5a8a84d, 88f5396).

WHY SKIPPED: GOALS.md:100-107 already declares the exotic extractors out of scope. Each is 8-36KB of upstream code for a language a given repo either uses entirely or not at all — the definition of long tail. Swift and Elixir are the only two with both a mainstream user base and a maintained Go tree-sitter binding, so they are the ones to revisit if a target repo demands it.

UPSTREAM: graphify/extractors/{apex,blade,commonlisp,dart,dm,elixir,fortran,objc,ocaml,pascal,powershell,razor,robot,sln,sql}.py.

**Rationale:** Adding an extractor nobody's corpus contains is pure speculative surface.

**Minimal change:** Adding one later is self-contained: a new internal/extract/<lang>.go, a case in the FileFromBytes dispatch (internal/extract/extract.go:101), and a go.mod tree-sitter grammar dependency. Do it when a real repo needs it.

### [synth] p4 `skip` `human-only` — Confirm Neo4j/Obsidian/Canvas/SVG/callflow-HTML/tree-HTML/wiki stay unported

**type** chore · **labels** delta, human-only, outputs-human-viewers, skip · **goals** out-of-scope-per-goals

WHAT: upstream export.py provides to_cypher (Neo4j), to_obsidian, to_canvas, to_svg alongside to_json/to_graphml (export.py:271-1354+), plus standalone callflow_html.py (2051 lines), tree_html.py (603 lines) and wiki.py (405 lines) generating browsable HTML/wiki artifacts. None have a Go counterpart; go's export command (cmd/graphify/main.go:662-688) offers graphml, dot, csv and okf — all machine-parseable.

WHY SKIPPED: these exist purely to be browsed or loaded into a third-party visualization tool by a person; none produce output an agent parses. GOALS.md:105 already lists 'Neo4j / Obsidian / SVG exports' as intentionally excluded, consistent with the already-removed HTML graph viewer.

OUTCOME: confirms the existing exclusion still holds against upstream v0.9.65 — no drift to act on, nothing built in go to remove.

UPSTREAM: graphify/export.py, graphify/callflow_html.py, graphify/tree_html.py, graphify/wiki.py.

**Rationale:** Re-audited against current upstream; the agent-first framing and GOALS.md both still exclude it. Tracked so the next audit does not re-derive it.

**Minimal change:** No action needed; GOALS.md:105 already documents this exclusion.

### [synth] p4 `skip` `out-of-scope` — Confirm LLM backends, transcribe, Google Workspace, Postgres/Cargo introspect and URL ingest stay unported

**type** chore · **labels** delta, out-of-scope, llm-integrations, agent-interfaces-serving, skip · **goals** out-of-scope-per-goals

MERGED from two area reports (LLM/integrations omnibus; `graphify add <url>` ingestion).

WHAT: upstream llm.py (3544 lines of multi-backend plumbing for Claude CLI/Bedrock/Ollama/OpenAI-compatible, semantic extraction, JSON-schema coercion, retry/bisect), transcribe.py, google_workspace.py, pg_introspect.py, cargo_introspect.py, and ingest.py's ingest() — which fetches a URL (detecting tweet, arXiv abstract, generic webpage, or binary), converts HTML to markdown and writes a frontmatter'd memory doc, wired to `graphify add` (cli.py L1947-1981). 726 commits of upstream churn since v0.9.17 land almost entirely here, and are all LLM-backend robustness fixes (retry semantics, JSON parsing, pricing tables, path handling) — no new user-facing capability class, no new CLI verb, no new structured export.

WHY SKIPPED: GOALS.md explicitly lists 'LLM-based semantic extraction and the LLM backends, video/audio transcription, image vision, Office/Postgres/Google ingest' as intentionally not ported. go's tree-sitter-only pipeline is deterministic and byte-identical by design; LLM backends break that guarantee and require external credentials. URL ingestion builds a human's curated reading-notes corpus, not code navigation, and pulls in HTML-to-markdown plus per-site scrapers.

OVERLAP NOTE: once the save-result bead lands, `graphify add` reduces to 'fetch a URL, then save-result' — composable by the agent with no new command and no network code in graphify.

UPSTREAM: graphify/llm.py, transcribe.py, google_workspace.py, pg_introspect.py, cargo_introspect.py, ingest.py L65-274.

**Rationale:** Nothing in the 726-commit delta changes the out-of-scope call — the volume is bugfix churn inside a stateful LLM-orchestration module go does not have and is not adopting.

**Minimal change:** No go files to touch. Do not add a `graphify ingest`/`add` command; save-result plus the agent's own fetch covers the only agent-relevant path.

## Critic verdict

Not complete. The backlog is accurate where it points — I spot-checked resolve.go:34 (still `map[string]string`), query.go:306 (first-match resolve), and main.go:79 (`cmdServe(defaultGraphPath)`, no args) and all three claims hold — but it has a blind spot on the query/explain/path *rendering* layer and on Terraform, and it over-weights two GRAPH_REPORT.md beads it simultaneously labels human-facing. Six real gaps, three of them reproduced end to end against a built binary: (1) ask/query_graph drops every edge between two already-visited nodes, so two seeds that call each other are rendered as unconnected — reproduced on a 3-function corpus, and it is the same silent-wrong-answer class the backlog itself reserves p0 for; (2) explain cites the caller's definition line instead of the edge's call-site line — reproduced, caller defined at L3, real call at L8, output says a.go:3; (3) explain dumps every neighbour unbounded and alphabetically — measured 100 lines/9964 bytes for one hub on a 360-file repo, in a backlog whose stated p0 rationale is context-window protection; (4) the `path` CLI prints bare labels while query.PathEdges, already written and already used by the MCP tool, carries relation/confidence/direction; (5) Terraform block attributes are absent entirely and go's TF nodes carry no metadata, with a hard requirement that redaction ship in the same commit because this repo commits graph.json; (6) no `god-nodes` CLI verb. I also chased and DISMISSED four plausible-looking candidates rather than padding the list: upstream's terraform local-module topology (go already emits tfmodule directory nodes at resolve.go:133, so 161e5f2 is not drift), the detect.py sensitive-file batch (.npmrc/service-account.json are either uncollected or have no extractor in go — no leak reproduced), the `_` tokenizer asymmetry (go still matched via the substring tier), and colliding file-node labels (the path::Symbol bead covers the agent path). On classification: no bead is wrong in the direction the brief worried about — nothing human-only is smuggled in as core-cli except the two GRAPH_REPORT.md beads, which are labelled `outputs-human-viewers` and classified `core-cli` in the same object.

## Critic: missed capabilities

### [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges

**type** bug · **labels** delta, core-cli, agent-interfaces-serving, query, correctness · **goals** new

WHAT: internal/query/ask.go:299 bfsTraverse pre-marks every seed visited (`visited := toSet(seeds)`) and records an edge only inside `if !visited[nb]` (:317-319). dfsTraverse (:332) has the same guard at :356. So an edge between two seeds can NEVER be emitted, and any cross-edge between two already-visited nodes is dropped. The result is a spanning tree over the node set the header claims, not the induced subgraph.

REPRODUCED on a 3-function corpus (/tmp/advseed): graph.json holds `zorkfoo() -> zorkbar() calls`; `graphify ask "zorkfoo zorkbar"` seeds on both, lists both as NODE lines, and emits zero EDGE line between them. Output was `EDGE zorkbar()--contains-->a.go / EDGE zorkfoo()--contains-->a.go / EDGE zorkfoo()--calls-->main()`.

WHY (agent value): this is the exact failure class the backlog itself calls p0 — the tool answers confidently and wrongly to an agent that cannot see the source. An agent that asks how two named symbols relate is shown both nodes and told there is no edge. It hits BOTH `graphify ask` and the MCP `query_graph` tool, since both route through query.Ask.

FILES: internal/query/ask.go bfsTraverse and dfsTraverse only; test in internal/query/ask_test.go.

UPSTREAM: commit 875c7df 'fix(query): render every edge between visited nodes, not just traversal edges' — adds _complete_induced_edges at the end of both traversals, scanning only edges incident to the visited set.

**Rationale:** Reproduced silent wrong answer on the two most-used agent entry points (ask + MCP query_graph), and the fix is a single post-pass shared by both traversals. Strictly higher severity than several p1 items the backlog did file.

**Minimal change:** One loop after the traversal: for each visited node, scan g.Links incident to it and append any pair whose both endpoints are visited, deduped. Do not restructure the traversals. Cost tracks the subgraph, not the graph.

### [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line

**type** bug · **labels** delta, core-cli, agent-interfaces-serving, query, correctness · **goals** new

WHAT: internal/query/query.go:138-143 sets each Neighbor's Location from the NEIGHBOUR NODE (`label, location = o.Label, loc(o)`), discarding the traversed edge's SourceFile/SourceLocation, which model.Edge already carries.

REPRODUCED (/tmp/advloc): a.go declares zorkcaller() at L3 which calls zorktarget() at L8. graph.json holds `EDGE zorkcaller() -> zorktarget() loc L8`. `graphify explain "zorktarget()"` prints `<- calls  zorkcaller()  a.go:3` — a precise-looking citation pointing at the function header, not the call site.

WHY (agent value): 'who calls this and where' is the primary explain question. An agent told a.go:3 opens the wrong line, finds no call, and concludes the graph is wrong (or worse, reads the wrong code). It is a wrong citation the agent cannot detect without the grep the tool exists to avoid.

FILES: internal/query/query.go Explain (:127-151) — prefer l.SourceFile/l.SourceLocation when non-empty, else fall back to loc(o). cmd/graphify/main.go cmdExplain already prints Neighbor.Location, so no change there. Note cmd/graphify/serve.go toolGetNeighbors (:240) prints no location at all today, so it is unaffected either way.

UPSTREAM: commit 1fbc623 'fix(query): report real call-site lines + stop silent query truncation' — 'every caller/relation listing now reads the traversed edge's source_file:source_location, falling back to the node's own line only when the edge lacks one'.

**Rationale:** Reproduced wrong-citation bug on a documented agent navigation command; the correct data is already on the Edge struct and is simply being ignored. Two lines in one function.

**Minimal change:** In the Explain neighbour loop, set location from l.SourceLocation (prefixed by l.SourceFile when it differs from the node's) when l.SourceLocation != "", else keep loc(o). Do not add a field to Neighbor.

### [critic] p2 `build` `core-cli` — Cap and group explain's connection list instead of dumping every neighbour

**type** bug · **labels** delta, core-cli, agent-interfaces-serving, query · **goals** new

WHAT: query.Explain returns every incident edge with no limit and sorts alphabetically by (relation, label) — internal/query/query.go:145-150 — and cmd/graphify/main.go:574-576 prints all of them. There is no top-N, no degree ordering, and no summary of what was cut.

MEASURED on this repo's own graph (360 files): `graphify explain strings` emits 100 lines / 9964 bytes for one hub. On a real monorepo a logging or error helper with thousands of callers dumps the whole list into the agent's context, and alphabetical ordering means the important callers are not the ones that survive a manual cut.

WHY (agent value): the backlog already accepted this argument for MCP get_neighbors/get_community ('output budgeting is the most agent-specific concern'). The CLI explain path is the same resource with the same exposure and was left out — an agent shelling out to `graphify explain` gets no budget at all.

FILES: internal/query/query.go Explain (ordering), cmd/graphify/main.go cmdExplain (cap + grouped tail).

UPSTREAM: graphify/cli.py ~L1820-1845; commits 0ef452c 'group cut connections by file on explain for high-degree nodes' (#2009) and 4c759b6 (deterministic tie-break). Upstream sorts by neighbour degree, shows the top 20, then a 'Grouped by file:' section of (direction, source_file) counts capped at 20 files.

**Rationale:** Same context-window argument the backlog used to justify its p0 token_budget bead, applied to the command an agent actually shells out to. Cheap: a sort key change plus a fold in the printer.

**Minimal change:** Sort neighbours by len(g.adj[id]) descending, print the first 20, then fold the remainder into (direction, source_file) counts. Skip upstream's --callers/--all flag surface entirely — the grouped tail answers the question.

### [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction

**type** bug · **labels** delta, core-cli, agent-interfaces-serving, query · **goals** new

WHAT: cmd/graphify/main.go cmdPath (:580-599) calls query.Path, which returns bare []Node, and prints `strings.Join(labels, " -> ")`. query.PathEdges — which already returns per-hop Relation, Confidence and Forward — exists and is used ONLY by cmd/graphify/serve.go:312 (the MCP shortest_path tool). Verified: `graphify path "Ask()" "subgraphToText()"` prints exactly `Ask() -> subgraphToText()`.

WHY (agent value): the CLI form of a documented navigation command returns strictly less than the MCP form of the same command. `A -> B -> C` with no relations cannot be distinguished from a contains/imports chain, so an agent tracing a call chain cannot tell whether it found one. The `->` glyph also asserts a direction the undirected BFS did not establish, which is the same fabricated-direction failure upstream fixed in #2074.

NOTE: this is adjacent to but distinct from the backlog's 'Respect edge direction by default in path/shortest_path' bead, which threads a directed flag through bfsPath and says nothing about what cmdPath prints. Land them together.

FILES: cmd/graphify/main.go cmdPath only — swap query.Path for query.PathEdges and render each hop.

UPSTREAM: graphify/cli.py L1675-1706 (segments built from the stored relation + confidence, `<--rel--` when the arc is reversed); commits b194301 (#2074) 'deterministic route + honest edge relation, not fabricated calls' and 50be9dc (#2309) 'recover edge direction from _src/_tgt in path and explain'.

**Rationale:** The data is already computed by an existing exported function with tests; only the CLI renderer ignores it. Smallest possible diff for a real information gap between two surfaces of the same command.

**Minimal change:** Call PathEdges (already exported, already tested), render `A --calls [INFERRED]--> B` / `A <--calls [INFERRED]-- B` off PathEdge.Forward. Do not duplicate the BFS; delete the now-unused query.Path only if nothing else calls it.

### [critic] p2 `build` `core-cli` — Persist Terraform block attributes as searchable node metadata, with secret redaction

**type** feature · **labels** delta, core-cli, languages-extraction, terraform, security · **goals** new

WHAT: upstream now keeps each HCL block's attribute key/value pairs on the node (`attributes` map, capped by _METADATA_MAX_ATTRIBUTES / _MAX_LIST_ITEMS / _MAX_VALUE_LEN) and scores them as their own search tier in serve.py (_node_attributes_text, _ATTRIBUTE_MATCH_BONUS, plus an attrs suffix in _subgraph_to_text). go emits nothing of the kind — verified on a one-resource corpus, the node is `{id, label, file_type, source_file, source_location}` with no attribute data; internal/extract/terraform.go only ever reads `source`/`version` and the null-label inputs (:180-215).

WHY (agent value): it turns 'which resources run t3.large', 'what ami does this module pin', 'which bucket sets versioning' into graph queries instead of file reads. The whole backlog is silent on Terraform, yet it is the extractor with an active multi-repo adoption effort behind it.

SECURITY RIDER — non-negotiable: this repo commits graph.json in CI. Attribute values must be redacted on the way in, or a hardcoded `db_password` in a .tf lands in a committed artifact and in the agent's context. Upstream shipped the redaction as a follow-up fix precisely because the first version leaked; do not repeat that ordering.

FILES: internal/extract/terraform.go (collect attributes in the block walk alongside the existing moduleArgs scalar/list classifiers, which already do most of the value parsing), internal/model/model.go (one `Attributes map[string]string `json:"attributes,omitempty"`` — omitempty keeps graph.json byte-identical for non-TF corpora), internal/query/ask.go scoreNodes (one extra field in the search text).

UPSTREAM: graphify/extractors/terraform.py _parse_attr_value / _redact_value / _SENSITIVE_KEY_RE (:19-45, :340-360); graphify/serve.py _node_attributes_text; commits 1e09e8b (#3644) and 650f47f (redaction follow-up).

**Rationale:** The only genuinely new agent-facing extraction capability in the drift window that go lacks and that maps onto a language this repo actively targets. Reuses the scalar/list classifiers terraform.go already has for null-label args.

**Minimal change:** Flat map[string]string only — stringify lists with the existing listSep, drop nested objects rather than porting the recursive walk. Ship _SENSITIVE_KEY_RE redaction in the SAME commit. Skip the separate _ATTRIBUTE_MATCH_BONUS tier at first: folding attributes into the existing search text is one line and gets most of the recall.

### [critic] p3 `build` `core-cli` — Wire a `god-nodes` CLI subcommand

**type** feature · **labels** delta, core-cli, analysis-quality, query · **goals** new

WHAT: analyze.GodNodes is reachable from `graphify serve` (MCP god_nodes) and from GRAPH_REPORT.md, but there is no CLI verb — the switch in cmd/graphify/main.go (:55-84) has no `god-nodes` case, so `graphify god-nodes` exits 1 with 'unknown command'. Upstream hit the identical gap and fixed it (#2004: 'an analyzer, an MCP tool, and a README-advertised capability, but never a CLI subcommand').

WHY (agent value): god nodes are the orientation answer for an unfamiliar repo, and an agent that is not running the MCP server (the common one-shot Bash case) can only get them by parsing a markdown report. Upstream's version takes --top, --graph and --json, so the agent gets a structured answer.

FILES: cmd/graphify/main.go — one `case "god-nodes"` plus a ~15-line handler reusing loadGraphAt + analyze.GodNodes + the existing parseGraphFlag helper, and one usage line.

UPSTREAM: graphify/cli.py L1391-1460; commit 1f4e3b2 'fix(cli): wire god-nodes subcommand (#2004)'.

NOTE: the backlog's p4 'exclude_hubs_percentile' bead adds that knob to the MCP tool only; if both land, share one analyze.GodNodes signature.

**Rationale:** Small, and it removes an MCP-only ceiling on the first question an agent asks about an unfamiliar repo. --json makes it the one genuinely parseable read command in the CLI.

**Minimal change:** Reuse parseGraphFlag and analyze.GodNodes as they stand. Support --top and --json; skip --exclude-hubs here and let the p4 bead add it to both call sites at once.

## Critic: misclassified beads

### Surface unclassified-file corpus coverage in GRAPH_REPORT.md

**Problem:** Classified core-cli and marked build, but its own labels say `outputs-human-viewers` and the entire deliverable is one prose line inside a markdown report. Under the stated rule ('human-only = exists to be looked at by a person ... pretty reports') a GRAPH_REPORT.md line is human-only by definition — no command emits it, nothing parses it, and an agent deciding whether to trust the graph would have to read and regex a markdown file to get the number. The bead's own justification ('coverage-confidence signal for an agent') argues for a machine-readable home it does not propose.

**Suggested:** Keep build but retarget the output: emit the unclassified count + top extensions from `graphify validate` and/or the MCP graph_stats tool (both already agent-parsed), and add the report line only as a byproduct. Relabel core-cli only once it has a non-report surface; as written it is human-only.

### Reconcile GRAPH_REPORT.md headline community counts with the render loop

**Problem:** Same labelling contradiction (`outputs-human-viewers` + classification core-cli), and here the payload is purely an internal-consistency nit in a human artifact: a headline integer disagreeing with a rendered list. No agent-facing command reads GRAPH_REPORT.md's Summary line — an agent wanting community counts calls graph_stats or reads graph.json. The bead justifies it as 'correctness of a CLI-consumed output', but nothing in cmd/ or internal/query consumes that number.

**Suggested:** Downgrade to skip, or drop to p4 and fold into whichever report change lands anyway. It is cosmetic consistency in the one surface the project declared it is not optimising for.

### Ship `graphify install` to register the Claude Code skill and search nudge

**Problem:** Under-scoped to the point of not working as described. The acceptance criterion is 'install merges one tagged PreToolUse entry', but a PreToolUse entry is a command, and upstream's is `graphify hook-guard search` (install.py:327, :353-356) — a subcommand graphify-go does not have and the bead never mentions. As written the bead installs a hook pointing at a nonexistent verb, which fires on every Bash and Grep call. The classification (core-cli) is also arguable: a one-time host-configuration writer is run by a person, not parsed by an agent.

**Suggested:** Either scope the bead to include a `graphify hook-guard <search|read>` subcommand (the thing that actually produces the nudge text), or cut the PreToolUse half entirely and ship only the skill copy — which is the lazy version and already delivers the adoption story. Do not merge a settings.json hook entry that has no command behind it.

### Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs)

**Problem:** Marked p1 build — the highest tier in the serving area — for a capability an agent can already compose from parts that exist. `get_pr_impact` is `gh pr diff <n> --name-only` piped into `graphify affected`, and the agent has both. The bead's own simplicity note concedes the implementation is exec.Command("gh"), a suffix path match, and a fold — i.e. graphify would be reimplementing the agent's own shell composition inside the server. It also adds a new internal/prs package, a gh dependency and three tool schemas for that.

**Suggested:** Downgrade to p3 skip with a note that the composition already works, or scope it down to nothing more than making `affected` accept the file list on stdin so `gh pr diff --name-only | graphify affected -` works. Reserve p1 for the reproduced wrong-answer beads.
