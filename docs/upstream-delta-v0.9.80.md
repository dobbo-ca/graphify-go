# Upstream delta backlog: graphify-go v0.9.0 vs graphify v0.9.80

Generated 2026-10-08 by the `graphify-delta-analysis` workflow. Baseline upstream v0.9.65 (7ca736c), target v0.9.80 (6478eb7), 308 commits. Epic `graphify-go-byx`.

## Summary

37 raw items collapse to 29 beads: 23 build, 6 skip. Merges: the two Python/local-import items became one p0 Python bead (with the `from __future__` phantom node folded in, same lines) plus one p1 bead for Go/C/C++/Rust/Ruby; the two UTF-16 items became one p2; the two skill auto-refresh items became one skip; the conflicting schema_version items (one area said build p3, one said skip) are resolved as skip because no go command reads the keys and they churn every determinism golden; the four "checked, not applicable" records became one chore. Agent-first stance: p0 is reserved for confident wrong answers an agent cannot check (fabricated calls edges from discarded receivers, `update .` from a subdir leaving a partial graph that answers `no matches`, `affected` printing `impacted (0)` for an imported Python file). Missing-edge recall gaps sit at p1-p2, node-count growth with no new edges (enum members, macros) is skipped, and nothing human-facing is proposed. No bead is already tracked in GOALS.md Follow-ups; two are out-of-scope-per-goals. GOALS.md:25 claims "relative imports resolve to files", which holds for JS/TS only (internal/extract/resolve.go:568-579), so that line should be corrected when the import beads land. What I verified myself: GOALS.md in full, a grep of docs/upstream-delta-v0.9.65.md for each candidate (no prior skip is reopened; hooksPath, pom placeholders, JSX, node_id alias, UTF-16 have no prior entry), and spot reads of resolve.go:568-579, python.go:42/98/101, maintenance.go:60/156/215, serve.go:282/303/549, cargo.go:29-110 and upstream commit 7a508a7. Everything else, including every fixture repro, is carried from the per-area raw items and was not re-run by me. One correction from my own read: the Cargo workspace-inheritance item was overstated. cargo.go:86-96 binds deps by key name, so `foo = { workspace = true }` already gets an edge when the key equals the crate name; the real gap is a `package =` rename declared in [workspace.dependencies], so it drops to p3. That is inference from source, not a run. Unchecked and worth one test outside this backlog: whether a refused anti-shrink update makes `graphify watch` rebuild every tick. The recall cost of the p0 member-call bead (correct instance-method edges it drops) is unmeasured; measure it on this repo's own graph before merging.

## Critic verdict

REFUTED as "complete and correctly classified". The three p0 beads are real (I re-ran two: `update .` from a subdir creates sub/graphify-out and `query OtherFn` prints `no matches`; with GRAPHIFY_OUT set, `update sub` prints `1 files (1 reparsed, 0 reused, 2 removed)`, exit 0; the committed graph has brokenpipe_test.go L26 -> model_model_new). No bead is human-only and no prior skip is reopened. But the backlog misses one p0 and two p1 items, and five beads carry wrong claims or a design that regresses.

Missed, all reproduced with a binary built from main (51ccfca) at /tmp/gfy:
1. p0: nested git worktrees are indexed and merged into real nodes. Building this repo to a temp GRAPHIFY_OUT gave 146 nodes under .claude/worktrees/claude-installer-8f2c, and `explain 'GodNodes()'` listed edges sourced from the stale copy. Upstream has skipped this since before v0.9.65 (detect.py:1153-1154, #1810), so it is not drift, but neither audit recorded it.
2. p1: `.h` headers holding C++ go to the C grammar. `class Widget { public: void show(); ... };` in w.h yields one node `Widget()`. Upstream reroutes via _is_cpp_header (extract.py:7254-7296, #1547, pre-baseline). This makes the export-macro bead a no-op on Qt/LLVM/Unreal, which declare classes in .h.
3. p1: Terraform secrets still reach graph.json through the raw-text fallback (terraform.go:279-282): `jsonencode([{name="API_TOKEN", value=...}])`, `env = ["DB_PASSWORD=..."]`, and a credential URL in an unlabelled output all leaked. The three upstream fixes the backlog marks not-applicable (#3762, #3817, #3870) are indeed covered in go; upstream's own else-branch has the same hole by source read (upstream not run, no tree_sitter installed).
4. p3: Python `module.func()` calls to an imported external module (#4043, 6868fff) are absent; `os.getcwd()` gave no edge.

Wrong or incomplete in existing beads: see misclassified. Short version: the member-call p0 drops inherited `self`/`super` calls that resolve today and that upstream binds via the ancestor chain; the Go/C/C++/Rust/Ruby import bead's claim that no imports_from exists outside JS/TS is false for C/C++; the Python import bead misses `from . import b`, which makes `graphify validate` fail on a fresh build; #3963 is filed under a node-count skip but is the class-to-method link for header-declared C++; the graph_stats staleness line would always read stale in this repo because CI commits the graph one commit after built_at_commit (9be99b9 vs HEAD 51ccfca).

Not checked by me: hooksPath, pom, JSX, PHP, JSON Schema, UTF-16, node_id alias, markdown angle links, bash runners, Cargo beads (carried from the backlog's own repros); anything requiring upstream to execute.

## Beads

### [synth] p0 `build` `core-cli`: Stop binding member and qualified calls by bare name

WHAT: every extractor records `x.f()`, `pkg.F()`, `T::f()` and `this.f()` as the bare callee `f` (internal/extract/javascript.go:159, golang.go:127, rust.go:160-164, python.go:190, csharp.go:136, php.go:122) and Resolve binds it to the unique same-named definition in the file, else corpus-wide (internal/extract/resolve.go:76-103). Only Python sets IsMember and only the import-guided pass reads it. Fixture builds by the area audit produced these fabricated INFERRED calls edges: Go `errors.New("x")` -> local `New()`; Rust `HashMap::new()` -> `Config::new`; TS `arr.push(1)` -> local `push()`; JS `res.format({})` and `axios.get()` -> lib.format / lib.get in another file; Python `self.session.get()` -> unrelated `pkg.b.get`; JS `this.m()` in class A (no m) -> `B.m`; Python `self.save()` in class A -> `B.save`. This repo's committed graph has one: cmd/graphify/brokenpipe_test.go L26 `errors.New("boom")` -> model_model_new.

WHY (agent value): calls edges feed explain, path and affected. An agent cannot tell a fabricated edge from a real one without re-reading source, which defeats the tool.

HOW: add `Recv string` to extract.Call (internal/extract/extract.go:32); each extractor's member branch passes the receiver text. In Resolve (resolve.go:76): (a) Recv is self/this/cls/$this/@self or the enclosing Go method's receiver name: bind only to a method whose contains-parent is the caller's parent, else drop; (b) any other Recv: accept a candidate only if the receiver's last segment is in defQualifiers(results)[id] (resolve.go:318 already computes path-segment, file-stem and owner-label tokens), else drop. Keeps `util.Join()` and `Config::new()`, drops `errors.New()`, `HashMap::new()`, `arr.push()`. Bump the extractor cache version.

FILES: internal/extract/extract.go, resolve.go, every language extractor's call branch, internal/extract/resolve_test.go.

UPSTREAM: graphify/extract.py:8771-8774 (shared pass drops non-self member calls, since e44e6e9), _resolve_python_member_calls, _resolve_typescript_member_calls; graphify/extractors/engine.py; graphify/ruby_resolution.py; fixes #4011 (Python self/cls/super), #4012 (JS/TS this), #4031 (Ruby self). The non-self guard predates v0.9.65 and the prior audit did not record it.

ACCEPTANCE: the seven fixtures above yield no edge; `util.Join()` to a corpus package `util` still resolves. Before merge, count calls edges on this repo's graph before and after and report the drop (unmeasured so far).

Rationale: Largest source of wrong data found. The prior p0 (same-file two-type collision) fixed one symptom; the root cause is the discarded receiver. Wrong edges are worse than missing ones.

Simplicity: One new Call field and two rules in Resolve. Skip upstream's per-language typed resolvers; the typed-receiver bead recovers lost recall later.

Critic amendment: Rule (a) binds self/this calls only to a method whose contains-parent is the caller's own class, else drops, and `super` is not in the self list so it falls to rule (b) and is dropped. That removes correct edges go emits today. Reproduced on main: Java `class A extends Base { void go() { super.log(); } }` currently yields `A.go -calls-> Base.log`. Upstream's fix for the same bug binds through the ancestor chain: #4011 (Python self/cls/super, nearest ancestor in the in-file MRO), #3932 (Java inherited and super.method() via the inherits chain), fail-closed on ties and unknown ancestors. The acceptance fixture `Python self.save() in class A -> B.save yields no edge` is also wrong when A inherits B. Lua `self:step()` (lua colon call) is missing from the extractor list; go drops it today when two tables define `step` (reproduced), and resolves it by bare name otherwise.

Required change: Keep p0. Change rule (a) to: own class first, then walk resolved `inherits` edges to the nearest ancestor defining the name, drop on a tie or an unresolved ancestor; treat `super` as self starting at the parent. Add Lua to the extractor list. Add acceptance: `super.log()` and an inherited `self.save()` still resolve.

### [synth] p0 `build` `core-cli`: Redirect `update <subdir>` and `watch` to the recorded scan root

WHAT: an explicit path to `update` is always taken as a fresh scan root. Two failures reproduced by the area audit on scratch repos with a binary from main. (A) After `graphify build .` at the repo root, `graphify update .` from `sub/` writes a second `sub/graphify-out/`; outDir()'s upward walk then finds that one first, so `graphify query OtherFn` from `sub/` prints `no matches` while the root finds `other/b.go:2`. (C) With GRAPHIFY_OUT set, `build .` then `update sub` prints `1 files (1 reparsed, 0 reused, 2 removed)` and replaces the whole-repo graph with the subfolder graph, exit 0; the anti-shrink guard does not fire because siblings count as deleted, not skipped. `graphify watch` has the same problem: cmdWatch calls cmdUpdate([]string{root}) with the default ".", so rootSet is always true (cmd/graphify/maintenance.go:253).

WHY (agent value): `graphify update .` is the command this repo's CLAUDE.md and the global graphify.md tell agents to run, and agents change directory constantly. The result is `no matches` for a symbol that exists, with nothing showing the graph is partial.

HOW: in cmdUpdate (cmd/graphify/main.go:550-557), when opts.rootSet and `<root>/graphify-out/graph.json` does not exist, compute scanRoot(); if abs(root) lies inside it (same containment test as rootUsable, main.go:120-133), set root = scanRoot(). Make cmdWatch pass no argument when the user gave none.

FILES: cmd/graphify/main.go, cmd/graphify/maintenance.go, cmd/graphify/outdir_test.go.

UPSTREAM: graphify/detect.py _manifest_storage_anchor (commit 79b7a84, #3785/#4118, 0.9.77); graphify/watch.py.

ACCEPTANCE: build at root, `update .` from a subdir: no subdir graphify-out is created and the sibling symbol still resolves. Same with GRAPHIFY_OUT set and `update sub`.

Rationale: The prior audit's acceptance line (docs/upstream-delta-v0.9.65.md:186) holds only for the no-argument form. The documented explicit-path form still breaks, silently. A whole-repo incremental update is cheap, so redirecting costs nothing.

Simplicity: One branch in cmdUpdate plus a one-line cmdWatch change. No scoped or partial update mode.

### [synth] p0 `build` `core-cli`: Resolve Python imports to corpus files

WHAT: resolveRelImport (internal/extract/resolve.go:568-579) returns "" for any specifier not starting with `.`, `/` or `@/` and only probes JS/TS extensions, so no Python import becomes an imports_from edge; python.go:98 says so ("Python relative imports stay external for now"). Reproduced by two area audits: a.py `import b` / b.py `import a` gives only `imports` edges to concept nodes; `graphify affected pkg/b.py` prints `impacted (0)` and GRAPH_REPORT.md prints `Import Cycles: None detected` for a real cycle. `from .b import helper`, `import pkg.b`, `from pkg.a import A` all land on concept nodes. Second defect: imports nested in a block (`if TYPE_CHECKING:`, try/except ImportError) are not recorded at all because python.go:42 only handles top-level statements. Third, same lines: `from __future__ import annotations` emits a phantom `annotations` concept node and imports edge shared by every modern Python file (python.go:42 and :101 route future_import_statement through the import branch).

WHY (agent value): `affected` is the blast-radius command an agent trusts. `impacted (0)` for an imported file is a confident wrong negative. Calls edges mask it only when the importer calls a symbol; modules imported for constants, side effects or re-exports get nothing.

HOW: (1) resolve.go: add a Python branch for .py importers next to resolveRelImport. Leading dots walk up from the importer's dir; then probe `<path>.py` and `<path>/__init__.py` in the existing corpus map. Absolute `a.b.c` probes from the importer's dir, each ancestor dir, and the corpus root; bind only on a single hit, drop on two. (2) python.go:42: descend into if/try/with bodies for import statements. (3) python.go: call b.impTyped (extract.go:299) with typeOnly=true when an ancestor if_statement condition is `TYPE_CHECKING` or `<x>.TYPE_CHECKING`, consequence branch only; analyze.ImportCycles already skips TypeOnly (analyze.go:171). (4) python.go:42 and :101: drop `future_import_statement` from both case lists. Bump the extractor cache version. Correct GOALS.md:25, which claims relative imports resolve to files for all languages.

FILES: internal/extract/resolve.go, internal/extract/python.go, tests beside them, GOALS.md.

UPSTREAM: graphify/extract.py _import_python, _resolve_python_module_path, _python_under_type_checking L696-721; fixes #3729 (absolute package imports, fail closed on ambiguity), #3867, #3784, #3898, #4179 (commit 1292639); tests/test_type_only_import_cycles.py. Upstream handling of `__future__` was not checked.

ACCEPTANCE: the a.py/b.py cycle is reported and `affected pkg/b.py` lists a.py; the TYPE_CHECKING variant yields an edge and zero cycles; no `annotations` node exists.

Rationale: Confident wrong negatives from affected and cycle analysis on the most common agent-facing language. The hole predates v0.9.65; the last audit closed only the TS half (graphify-go-2af.13).

Simplicity: All in resolve.go and python.go, reusing the corpus map and resolveModulePath's lookup style. Skip upstream's sys.path-root ambiguity machinery; drop on ambiguity instead.

Critic amendment: Misses a defect on the same lines that breaks the tool's own validator. Reproduced: pkg/a.py with `from . import b` emits a node with id "" and label `.`, and `graphify validate` on the freshly built graph exits non-zero with `node with empty id (label ".")` and `edge pkg_a_py --imports--> : target node missing`. The HOW resolves `<path>.py` for the module part only, so `from . import b` and `from .pkg import submodule` (the imported name is the module) would still not bind to b.py.

Required change: Keep p0. Add: for `from <dots>[pkg] import name`, probe `<dir>/name.py` and `<dir>/name/__init__.py` before treating name as a symbol; never emit an empty id. Add acceptance: `from . import b` yields an edge to pkg/b.py and `graphify validate` passes on the fixture.

### [synth] p1 `build` `core-cli`: Resolve local imports to files for Go, C/C++, Rust and Ruby

WHAT: same root cause as the Python import bead (internal/extract/resolve.go:568-579 only resolves `.`, `/`, `@/` specifiers with JS/TS extensions). Fixture builds by the area audit: Go `import "example.com/m/util"` with go.mod in the corpus, C/C++ `#include "h.h"`, Rust `use crate::helper::assist` and `mod helper;`, Ruby `require_relative 'lib/h'`, Java `import com.x.B` all become `imports` edges to source-less concept nodes. No imports_from edge exists outside JS/TS, so file-level `affected`, `path` and import-cycle analysis are blind there, including on this repo.

WHY (agent value): imports_from is what `affected` and cycle detection read. Its absence gives a silently short blast radius.

HOW: branch resolveRelImport on the importing file's extension. Go: read the module path from any go.mod in the corpus (manifest.go already parses it), strip the prefix, link to every .go file in that dir. C/C++: quoted include relative to the including file's dir. Rust: `mod x;` -> x.rs or x/mod.rs beside the file; `crate::a::b` -> src/a/b.rs, src/a/b/mod.rs, src/a.rs. Ruby require_relative: add `.rb`. Bump the extractor cache version.

FILES: internal/extract/resolve.go, resolve_test.go.

UPSTREAM: graphify/extractors/go.py (#3748, intra-module imports repoint from the go_pkg_ sink to the package's files), graphify/extractors/resolution.py, graphify/extract.py.

ACCEPTANCE: one fixture per language yields an imports_from edge to the real file; an unresolvable import still yields the concept node. Land after the Python bead so the branch structure exists.

Rationale: Missing rather than wrong, so p1. Go symbol-level calls edges already cover much of the Go case, which is why this sits below the Python bead.

Simplicity: Four small branches in one function. Skip Java/Kotlin/C#/PHP namespace imports until a repo needs them.

Critic amendment: The claim `No imports_from edge exists outside JS/TS` and the C/C++ fixture claim are false. Reproduced: `#include "../inc/w.hpp"` yields `src_w_cpp -imports_from-> inc_w_hpp` (resolveRelImport takes any `.`-prefixed spec and resolveModulePath tries the path as-is first), and `#include "inc/h.h"` from the corpus root lands on the real file node by id match, so `graphify affected inc/h.h` prints `impacted (1): main.c`. The real C/C++ gap is narrower: an include relative to the including file's directory without `./`, when that file is not at the corpus root (`src/main.c` with `#include "h.h"` and `src/h.h` gave `impacted (0)`). Go, Rust and Ruby claims hold (Ruby `require_relative './lib/h'` gave no file edge; Go and Rust land on concept nodes).

Required change: Keep build p1. Rewrite the C/C++ part to: probe the including file's directory for a quoted include with no leading dot. Remove the blanket no-imports_from sentence.

### [synth] p1 `build` `core-cli`: Record CommonJS require, re-exports and dynamic import as JS/TS import edges

WHAT: internal/extract/javascript.go:30-48 only handles `import_statement`. Fixture builds by the area audit: `const lib = require('./lib')` yields no import edge, `export * from './lib2'` and `export { thing } from './lib2'` yield none (export_statement is unwrapped only when it has a `declaration`), and `await import('./lib2')` yields none. A barrel file is a dead end for `affected` and `path`; a CommonJS codebase has no file-level import graph.

WHY (agent value): same imports_from substrate as the import-resolution beads. Barrels and CommonJS are mainstream.

HOW: in jsStatement, for export_statement with a `source` field call b.impTyped (reuse jsTypeOnlyImport for `export type`). In a whole-file walk, a call_expression whose function is the identifier `require` or the `import` keyword with one string-literal argument calls b.imp. Resolve already turns relative specs into imports_from. Bump the extractor cache version.

FILES: internal/extract/javascript.go, javascript_test.go.

UPSTREAM: graphify/extract.py, graphify/extractors/resolution.py, graphify/extractors/engine.py (#4155, #4143). require/re-export support itself predates v0.9.65; the prior audit's type-only-import bead named export re-exports as a file to touch (docs/upstream-delta-v0.9.65.md:74) but the code does not handle them.

ACCEPTANCE: each of the four forms yields an imports_from edge to the target file.

Rationale: A few lines per form in an extractor that already has the resolver behind it.

Simplicity: No separate re_exports relation. Excluded: tsconfig `references` (#3753) and source-over-dist workspace resolution (#4000); both extend the tsconfig paths work the prior audit deferred, and nothing upstream changes that.

### [synth] p1 `build` `core-cli`: Keep C++ classes declared behind export macros

WHAT: `class Q_CORE_EXPORT Widget : public Base { public: void show(); };` yields a single node labelled `Widget()` (a function) with no class, no members and no inherits edge; `struct API Thing { int x; };` yields `Thing()`. In a second fixture the class vanished entirely. A plain class in the same file extracts correctly. Reproduced by the area audit. The pattern is on nearly every public class in Qt, LLVM (LLVM_ABI) and Unreal (MODULE_API).

WHY (agent value): a class reported as a function with its methods missing is a wrong answer to query/explain on any exported C++ type.

HOW: in internal/extract/cpp.go, before parseRoot, run a regexp over src matching `\b(class|struct)\s+((?:[A-Z][A-Z0-9_]*\s+)+)([A-Za-z_]\w*(?:\s+final)?\s*)([:{])` and overwrite group 2 with spaces, keeping \r and \n so byte offsets and line numbers hold. Go regexp has no lookahead, so in the replace func skip when group 3 is `final`. Port upstream's two guards: skip when preceded by `(`, and when `{` is followed by a digit or quote (brace init). Bump the extractor cache version.

FILES: internal/extract/cpp.go, cpp_test.go.

UPSTREAM: graphify/extract.py:3029-3080 (_CPP_EXPORT_MACRO_RE, _normalize_cpp_export_macros), #3661/#3648, 0.9.78.

ACCEPTANCE: the Widget fixture yields a class node with a `show` method and an inherits edge to Base.

Rationale: Wrong node kind plus lost members on the classes an agent most often asks about in a C++ library. Small local fix.

Simplicity: One regexp pre-pass. Paren macros like `__declspec(dllexport)` stay unsupported, as upstream.

Critic amendment: The regexp pre-pass lives in cpp.go, but `.h` files go to the C extractor (extract.go:141). Qt, LLVM and Unreal, the three corpora cited as motivation, put these classes in `.h`, so the fix never runs there. A plain class in a `.h` already degrades to `Widget()` without any macro (reproduced).

Required change: Keep build p1 but block it on the new `.h` C++ routing bead, and make the acceptance fixture a `.h` file.

### [synth] p1 `build` `core-cli`: Write an absolute .graphify_root when GRAPHIFY_OUT is absolute

WHAT: writeOutputs stores the caller-supplied root verbatim (cmd/graphify/main.go:374). scanRoot() resolves a relative marker against the out directory's parent (main.go:94-96), not the cwd it was written from. With a shared absolute GRAPHIFY_OUT these differ. Reproduced by the area audit: from `main/wt` with GRAPHIFY_OUT=`main/graphify-out`, `graphify build .` graphs 1 file and writes marker `.`; a following no-arg `graphify update` resolves the marker to `main/`, scans the parent checkout and reports `2 files (2 reparsed, 0 reused, 1 removed)` with no warning. When the out parent has no sources the command fails with `no supported source files found`.

WHY (agent value): GRAPHIFY_OUT exists for the worktree-per-session workflow. There a bare `update` silently graphs another checkout, so every later query/explain/affected answer is about the wrong tree.

HOW: main.go:374 only: if filepath.IsAbs(os.Getenv("GRAPHIFY_OUT")) write filepath.Abs(root), else keep root verbatim so a committed marker stays portable. No reader change; rootUsable already rejects stale or foreign absolute markers.

FILES: cmd/graphify/main.go, cmd/graphify/outdir_test.go.

UPSTREAM: graphify/watch.py:328 (_graphify_root_marker_value, #3375/#3735, 0.9.66).

ACCEPTANCE: absolute GRAPHIFY_OUT, build from a nested dir with ".": the marker is absolute and a no-arg update rescans the same file set.

Rationale: Silent wrong-tree scan, reproduced, one-line fix copying upstream's rule.

Simplicity: One conditional at the write site. Not fixed here: `graphify build src` from the repo root writes marker `src`, which resolves to `src/src` and is discarded with a warning; the fallback lands on the right tree, so that is noise only.

### [synth] p1 `build` `core-cli`: Resolve the real git hooks dir in hook install, uninstall and status

WHAT: hookInstall, hookUninstall and hookStatus hard-code `<root>/.git/hooks` (cmd/graphify/maintenance.go:60, :156, :215; confirmed by source read). Reproduced by the area audit: (1) with core.hooksPath set to `.husky`, `graphify hook install .` writes three scripts into `.git/hooks`, which git never runs, and `hook status` prints `installed` for all three. (2) In a linked worktree `.git` is a file, so install fails with `has no .git directory (git worktrees and submodules are not supported by hook install)` and leaves the merge driver `partial`.

WHY (agent value): `hook status` is the machine-checkable answer to whether the graph stays fresh. `installed` for hooks that cannot fire means an agent trusts a graph that goes stale after the next commit. This user's setup uses a global core.hooksPath and a worktree per session, so both failures hit the primary workflow.

HOW: one helper in maintenance.go used by all three functions: run `git -C root rev-parse --git-path hooks`; accept it if it resolves under root or equals `<git rev-parse --git-common-dir>/hooks`; otherwise use the common-dir hooks. When the dir used differs from the effective `--git-path hooks`, status prints `installed (inactive: core.hooksPath=<dir>)`. Drop the `.git` is-a-directory precheck.

FILES: cmd/graphify/maintenance.go, tests beside cmd/graphify/hook_mergedriver_status_test.go.

UPSTREAM: graphify/hooks.py:613-690 (_builtin_hooks_dir, _hooks_path_allowed, _hooks_dir; commit 762cfd3, #3869, 0.9.76).

ACCEPTANCE: in-repo hooksPath is honoured; out-of-repo hooksPath is refused and reported inactive; a linked worktree installs. Submodules were not checked.

Rationale: The prior audit never considered core.hooksPath (no match in docs/upstream-delta-v0.9.65.md), so this reopens nothing. Port the containment rule with the lookup; the lookup alone imports the write-outside-the-repo bug upstream just closed.

Simplicity: One helper, two git calls. No config knob.

### [synth] p1 `build` `core-cli`: Report build commit and HEAD staleness in MCP graph_stats

WHAT: go writes built_at_commit into graph.json (internal/export/export.go:77) but query.Graph never loads it (internal/query/query.go:21-38) and toolGraphStats (cmd/graphify/serve.go:407-431) prints only counts, the confidence split and the unclassified line. The area audit called graph_stats over stdio and saw no commit line. Upstream appends `Built at commit: <sha> (matches HEAD)` or `HEAD is <sha7>, graph built at <sha7>: graph may be stale`.

WHY (agent value): an MCP client cannot stat graph.json, so this is its only staleness signal. It tells the agent whether to trust the graph or run `graphify update` first.

HOW: add `BuiltAtCommit string` with json tag `built_at_commit` to query.Graph. In toolGraphStats append the line, comparing against the existing gitHead() helper (cmd/graphify/main.go:1145) run on the graph file's parent dir; when git is unavailable print the commit without the comparison.

FILES: internal/query/query.go, cmd/graphify/serve.go, serve_test.go.

UPSTREAM: graphify/serve.py _load_graph L77-81, _build_commit_line L2292-2312; commit 748de12 (#4144/#3354, 0.9.80).

ACCEPTANCE: graph_stats output contains the commit line, with the stale wording when HEAD differs.

Rationale: A stale graph gives confident answers the agent cannot detect as stale through MCP. The data is already on disk.

Simplicity: One struct field, one line of output, one git call. This is also why the schema_version stamp is skipped: built_at_commit is the provenance that has a reader.

Critic amendment: A plain HEAD comparison is always stale in this repo. CI builds the graph at commit X and commits it as X+1: the committed graph.json has built_at_commit 9be99b9 while HEAD is 51ccfca (`chore: regenerate knowledge graph [skip ci]`). Every clean checkout would print `graph may be stale`, which teaches the agent to ignore the line or to run a needless update.

Required change: Keep build p1. Report fresh when built_at_commit equals HEAD, or is an ancestor of HEAD and `git diff --quiet <built> HEAD -- . ':!graphify-out'` shows no change outside the out dir. Add that case to the acceptance.

### [synth] p1 `build` `core-cli`: Resolve a source file path as a node in explain, path and the MCP node tools

WHAT: resolve (internal/query/query.go:418-474) tries exact ID, path::Symbol, exact label, then label/ID substring, and never matches source_file. Reproduced by the area audit: `graphify path pkg/a.py pkg/b.py` and `graphify explain pkg/b.py` both return `no node matching "pkg/b.py"`, while the bare label `b.py` resolves. Node ids are `pkg_b_py`, so the substring tier cannot catch a slash path.

WHY (agent value): every lookup command and four MCP tools (get_node, get_neighbors, shortest_path, explain path) route through resolve, and file paths are what an agent has in hand from git diff, `affected` output or a stack trace. `no node matching` for an indexed file reads as "not indexed". `affected` already accepts paths, so this is inconsistent.

HOW: one tier in resolve between path::Symbol and exact label: collect nodes whose SourceFile equals the query (or ends with "/"+query) and return the file node, the one whose Label equals path.Base(SourceFile). Reuse the ./ and absolute-path normalisation from internal/query/affected.go.

FILES: internal/query/query.go, cmd/graphify/explain_path_test.go.

UPSTREAM: graphify/serve.py _find_node_tiers source_exact tier, _resolve_path_endpoint L1701-1740; graphify/cli.py path command L1679-1690; commit ea1f158 (#3913/#3935, 0.9.76). The source_exact tier predates v0.9.65 (serve.py L1492-1583 at that tag) and the prior audit missed it; the drift is that path endpoints now rely on it. The other half of ea1f158 (ambiguous endpoints refused) is not a go gap: resolve already returns AmbiguousError.

ACCEPTANCE: explain and path accept a repo-relative path, with and without a leading `./`.

Rationale: Wrong negative on the most natural agent input, fixed by one tier in a shared function.

Simplicity: One lookup tier, reusing affected's normalisation. No new flag.

### [synth] p2 `build` `core-cli`: Skip PHP language constructs as call targets

WHAT: internal/extract/php.go:116-119 records every function_call_expression by name with no filter. Reproduced by the area audit: a class with `function isset()` and `if (isset($x))` in another method emits `A.go -calls-> A.isset` INFERRED.

WHY (agent value): fabricated calls edge; methods named `list` or `empty` are common in PHP collection code.

HOW: at php.go:116 skip the bare call when strings.ToLower(name) is in {array, die, empty, eval, exit, isset, list, unset}. Bare calls only; `$this->list()` must keep working (upstream restored it in #4119). Bump the extractor cache version.

FILES: internal/extract/php.go, php_test.go.

UPSTREAM: graphify/extractors/engine.py:4361 (_PHP_LANGUAGE_CONSTRUCTS), :7486; #3975/#3830, 0.9.76.

ACCEPTANCE: the fixture yields no edge to `isset`; `$this->list()` still resolves.

Rationale: Same bug as upstream, verified, eight-word fix.

Simplicity: One set literal and one guard.

### [synth] p2 `build` `core-cli`: Stop walking JSON Schema files as config manifests

WHAT: configJSONKeys includes `$schema` (internal/extract/json.go:46) and isConfigJSON (json.go:198) has no schema exclusion. Reproduced by the area audit: a draft-07 schema with `$id`, `definitions` and `properties.dependencies` produced 10 keyword nodes (`type`, `properties`, `definitions`...) and a bogus `dependencies.type -imports-> type` edge to a concept node.

WHY (agent value): nodes named `type` and `properties` pollute query results and god-node ranking; the imports edge is fabricated.

HOW: in isConfigJSON, after the filename checks, collect the root keys once and return false when `$schema` is present together with any of `$defs`, `definitions`, `$id`. Filename-matched configs (package.json, tsconfig.json) are decided earlier and stay unaffected. Bump the extractor cache version.

FILES: internal/extract/json.go, json_test.go.

UPSTREAM: graphify/extractors/json_config.py:36-75; #4048/#2255, 0.9.76.

ACCEPTANCE: the schema fixture yields only its file node; a config with `$schema` alone is still walked, as upstream.

Rationale: Verified same bug, narrow fix with a boundary upstream already tested.

Simplicity: One early return.

### [synth] p2 `build` `core-cli`: Decode UTF-16 and non-UTF-8 sources before parsing

WHAT: extract.File hands raw bytes to tree-sitter (internal/extract/extract.go:98-104), which assumes UTF-8. Reproduced by the languages audit: a UTF-16 (BOM) `def hello()` file yields only its file node; a latin-1 `def café()` yields label `caf()` and id `misc_latin_caf`. UTF-8 with BOM is already fine (checked for .py and .cs).

WHY (agent value): a file that silently contributes no symbols, or a symbol under a truncated name, is a wrong answer with no warning.

HOW: first line of FileFromBytes: if utf8.Valid(src) leave it untouched (byte-identical output for all current corpora). Else if it starts with FF FE or FE FF decode with unicode/utf16 and re-encode; else map each byte to a rune (latin-1) and encode. Do it inside FileFromBytes so the cache keeps hashing the original bytes.

FILES: internal/extract/extract.go, extract_test.go.

UPSTREAM: graphify/extractors/base.py:90-150 (_read_source_bytes); commit be528e1, #4146/#4145, 0.9.78.

ACCEPTANCE: both fixtures yield the full function name; the determinism goldens are unchanged.

Rationale: Silent data loss with a stdlib-only fix. Rare outside Windows-origin C#/SQL/legacy code, so p2 not p1 (the second area that proposed p1 had no repro).

Simplicity: About 15 lines, stdlib only. Skip cp1252 proper: it needs golang.org/x/text and differs from latin-1 only in 0x80-0x9F. Skip UTF-32.

### [synth] p2 `build` `core-cli`: Emit calls edges for JSX component usage

WHAT: jsCalls (internal/extract/javascript.go:142) only matches call_expression. Verified absent by the area audit: `export function Page() { return <div><MyButton/></div> }` with MyButton imported from ./btn gives the imports_from edge and no calls edge.

WHY (agent value): in a React codebase rendering is the call graph. Without it `affected` on a component reaches importers only at file granularity and `path` between components finds nothing.

HOW: in jsCalls also match jsx_opening_element and jsx_self_closing_element; take the `name` field, require kind identifier (not member_expression, so `<Nav.Item>` is skipped) and a first rune that is uppercase or `_`, then b.call. The bare capitalised tag rides the existing bare-name resolution and import disambiguation. Bump the extractor cache version.

FILES: internal/extract/javascript.go, javascript_test.go.

UPSTREAM: graphify/extract.py:1311, :1365 (call_types incl. jsx_*), graphify/extractors/engine.py; #3855/#3854, 0.9.70.

ACCEPTANCE: the Page fixture yields `Page -calls-> MyButton`; `<div>` and `<Nav.Item>` yield nothing.

Rationale: High recall gain on TSX repos for about ten lines.

Simplicity: Two node kinds added to an existing switch.

### [synth] p2 `build` `core-cli`: Record call sites and methods the C#, Ruby, JS/TS and Scala extractors drop

WHAT: four misses, each reproduced by the area audit. (1) C# null-conditional `st?.Save()` records nothing while `((S)st).Load()` resolves; csharp.go:120-138 only handles member_access_expression/qualified_name (#3976). (2) Ruby paren-less self-send `do_thing` records nothing while `self.other` resolves; ruby.go:168 only sees `call` nodes and the bare form parses as an identifier (#3960). This is the dominant intra-class call form in Ruby. (3) JS `res.format = function () { helper2() }` produces no node and the inner call is lost; javascript.go only walks declarations (#4121). (4) TS `abstract area(): number` (#3961) and Scala deferred `def area: Double` (#3962) produce no method node, so `this.area()` has no target.

WHY (agent value): missing calls edges shorten `affected` and `explain`. Nothing fabricated.

HOW: csharp.go csCalls: handle fn.Kind()==conditional_access_expression by taking the member_binding_expression `name`. ruby.go: in a method body, treat a statement-position identifier as a call when the name is not a parameter or assigned local of that method and is a method defined in the file. javascript.go: top-level expression_statement whose assignment right side is a function/arrow and left side is a member_expression becomes a def named by the property; jsClass also accepts abstract_method_signature. scala.go: accept function_declaration without body. Bump the extractor cache version.

FILES: internal/extract/csharp.go, ruby.go, javascript.go, scala.go and their tests.

UPSTREAM: graphify/extractors/engine.py:7167-7214 (Ruby bare self-send suppression rule), graphify/extract.py _resolve_csharp_member_calls, graphify/extractors/csharp.py.

ACCEPTANCE: one fixture per case yields the edge or node. Do the Ruby case after the member-call p0 bead so the self-send binds to the caller's class, not by bare name. Java anonymous-class methods (#4140) need nothing: the inner call is already attributed to the enclosing method.

Rationale: Each is a small case in one extractor. Ruby and C# carry the volume.

Simplicity: Four independent small cases; split into separate PRs if one stalls. No shared abstraction.

### [synth] p2 `build` `core-cli`: Resolve typed-receiver method calls from annotations and constructor bindings

WHAT: go has no receiver typing. Verified absent by the area audit: `def use(c: Client): return c.fetch()` with two classes defining fetch in the file yields no edge. Upstream 0.9.80 resolves Python `obj.method()` when the receiver's class is known from an annotation or a constructor/`with` binding (#4198) and `self.<attr>.<method>()` through the attribute's type (#4176), fail-closed to a single owning class; 0.9.78 does the C# equivalent for casts and interface-typed properties (#4172).

WHY (agent value): this is how instance-method calls get a correct target instead of a name guess, and it is the recall recovery for the member-call p0 bead, which drops value-receiver calls lacking evidence.

HOW: per function body, build map[localName]typeName from the cheapest syntactic sources and pass typeName as Call.Recv so the p0 bead's qualifier check accepts the owner-label match. Python: typed parameters and `x = Foo(...)` (python.go). Go: typed params and `x := Foo{}` / `&Foo{}` (golang.go). Start with those two.

FILES: internal/extract/python.go, golang.go, resolve_test.go.

UPSTREAM: graphify/extract.py:4139-4360 (_resolve_python_member_calls, _python_self_attr_type), :4674 (_resolve_csharp_member_calls); graphify/extractors/engine.py:1693-1720.

ACCEPTANCE: the Client fixture yields `use -calls-> Client.fetch` and no edge to the other class. BLOCKED BY the member-call p0 bead (needs Call.Recv); without it this adds nothing because the name pass already guesses.

Rationale: Puts back the correct instance-method edges with a type name as evidence. Size it after measuring how much the p0 bead actually drops.

Simplicity: A per-function map, two languages. Skip unions, generics, `with`, attribute chains, cross-function flow, and Java/C#/Kotlin/TS until a repo needs them.

### [synth] p2 `build` `core-cli`: Report a stale hook script as out of date in hook status

WHAT: hookStatus reports `installed` whenever the file contains the `# graphify-managed hook` marker (cmd/graphify/maintenance.go:218). The script embeds the absolute path of the binary that ran install (maintenance.go:67-75) and ends in `>/dev/null 2>&1 || true`. Reproduced by the area audit: after rewriting the hook to exec `/nonexistent/graphify`, status still prints `post-commit: installed`. The same happens when a release changes the script or the binary moves.

WHY (agent value): status is the only place a silently failing hook can show, and it is the output an agent parses to decide whether the graph self-updates.

HOW: extract the script text built in hookInstall (maintenance.go:75-84) into a func(hook, self, absRoot) string; in hookStatus compare the file bytes to it and print `installed (out of date: run graphify hook install)` on mismatch.

FILES: cmd/graphify/maintenance.go, cmd/graphify/hook_mergedriver_status_test.go.

UPSTREAM: graphify/hooks.py:985-1038 (_comparable_block, status; commit 81bb528, #3771/#3951, 0.9.73).

ACCEPTANCE: a hand-edited hook reports out of date; a fresh install reports installed. Land with or after the hooks-dir bead, same functions.

Rationale: About ten lines, and it removes the second way `installed` can be false. Needs a moved binary or an upgrade to trigger, so below the hooks-dir bead.

Simplicity: Byte compare against the generator. No version stamp file. Untested inference: on Linux os.Executable() returns the symlink-resolved path, so a versioned install prefix would trip this on every upgrade; check before choosing the wording.

### [synth] p2 `build` `core-cli`: Resolve inherited groupId and ${property} placeholders in pom.xml ingestion

WHAT: parsePom (internal/extract/manifest.go:307-345) has upstream's pre-fix behaviour. Reproduced by the area audit: two modules sharing a <parent> groupId, app depending on lib via ${project.groupId} and ${lib.group}. Go emitted package nodes `app` and `lib` with no group, plus phantom nodes labelled `${lib.group}:lib` and `${project.groupId}:lib`, with both depends_on edges pointing at the phantoms. The real app -> lib edge does not exist.

WHY (agent value): module dependency edges are what `affected`, `path` and get_neighbors answer from in a multi-module Maven repo. Inherited groupId is the normal shape, so "what depends on lib" returns nothing with no signal it is wrong.

HOW: in parsePom read the parent groupId via root child `parent`, build a map from <properties> children plus project.groupId / project.artifactId / project.version / project.parent.*, and run a `\$\{([^}]+)\}` regexp replace over gid and each dependency groupId/artifactId, leaving unknown placeholders intact. Offline, one level, as upstream.

FILES: internal/extract/manifest.go, manifest_test.go.

UPSTREAM: graphify/manifest_ingest.py _parse_pom L288-328; commit cac084c (#3806/#3823, 0.9.68).

ACCEPTANCE: the two-module fixture yields one app -> lib depends_on edge and no node label containing `${`.

Rationale: The common Maven layout currently yields disconnected modules and literal placeholder labels.

Simplicity: About 25 lines in one function. Skip the version half; the go node does not store version. The Cargo virtual-workspace change in the same range (c5c4ee6) needs nothing: manifest.go:197 already emits no node for it.

### [synth] p2 `build` `core-cli`: Accept node_id and id as aliases for label on MCP get_node and get_neighbors

WHAT: go reads only `label` (cmd/graphify/serve.go:282 and :303, confirmed by source read) and declares it required (:549, :556). Reproduced over stdio by the area audit: get_node with {"node_id":"pkg_b_helper"} and get_neighbors with {"id":"pkg_b_helper"} both return `No node matching '' found.`, although {"label":"pkg_b_helper"} resolves.

WHY (agent value): get_node prints `ID:` in its own output, so passing that back as node_id is the obvious next call. The current reply reads as "node does not exist" rather than "wrong argument name".

HOW: a nodeArg(args) helper in serve.go returning the first non-empty of label/node_id/id; use it at :282 and :303; return `Provide a node label or id (accepted keys: label, node_id, id).` when empty; add node_id to both schemas and drop the required "label" at :549 and :556.

FILES: cmd/graphify/serve.go, serve_test.go.

UPSTREAM: graphify/serve.py _node_arg L188-198, tool schemas L2011-2033, _tool_get_node L2175, _tool_get_neighbors L2208; commit a1d2318 (#3725, 0.9.66).

ACCEPTANCE: all three keys resolve the same node; an empty call returns the guidance string.

Rationale: Upstream saw real clients do this; the failure mode misleads the agent.

Simplicity: A 6-line helper and two schema edits.

### [synth] p3 `build` `core-cli`: Resolve markdown links with spaces or percent-encoding

WHAT: `[x](<My Note.md>)` and `[x](My%20Note.md)` produce no references edge, while wikilinks to the same file resolve (reproduced by the area audit). mdLink (internal/extract/markdown.go:14) captures `[^)\s]+`, so the angle form is cut at the space, and resolveMDTarget (internal/extract/resolve.go:376) does not unescape. go drops the edge rather than minting a ghost target, so this is a miss, not a wrong edge.

WHY (agent value): doc-to-doc references edges are agent-navigable and the miss is silent.

HOW: markdown.go:14: allow an alternative `<([^>]+)>` capture in mdLink. resolveMDTarget: apply url.PathUnescape before the corpus lookup, keep the raw value on error. Wikilinks stay verbatim. Bump the extractor cache version.

FILES: internal/extract/markdown.go, resolve.go, markdown_test.go.

UPSTREAM: graphify/extractors/markdown.py, graphify/extract.py; #4178, 0.9.80.

ACCEPTANCE: both link forms yield a references edge to `My Note.md`.

Rationale: Two-line fix, low volume.

Simplicity: One regexp alternative and one stdlib call.

Critic amendment: Incomplete for the same file: the escaped-pipe wikilink alias used inside markdown tables, `[[target\|alias]]`, yields no references edge in go (reproduced; `[[plain]]`, `[[note.en]]` and `[[v1.2 release]]` all resolve). Upstream fixed it in 0.9.72 (#3772).

Required change: Keep build p3 and add the `\|` alias split to the wikilink parser in the same change.

### [synth] p3 `build` `core-cli`: Record bash invocations of non-shell scripts

WHAT: go only links shell runners to `.sh` targets (bashScriptRunners, internal/extract/bash.go:15; scriptInvocationTarget bash.go:125). Verified by the area audit: a run.sh containing `python3 build.py` with build.py in the corpus emits no edge.

WHY (agent value): cross-language edge from build/CI scripts to the programs they run, useful to `affected`.

HOW: add python/python3/node/ruby/perl/php/deno/bun to the runner set and let scriptInvocationTarget accept any literal first non-flag argument that names a file; the existing prune-if-not-in-corpus step drops misses. Keep the rule that a dynamic target yields nothing, and do not treat a `$VAR` or path command word as an interpreter. Bump the extractor cache version.

FILES: internal/extract/bash.go, bash_test.go.

UPSTREAM: graphify/extractors/bash.py; #4007, 0.9.75.

ACCEPTANCE: `python3 build.py` and `node build.js` yield an edge to the corpus file; `"$PY" build.py` and `python3 "$X"` yield none.

Rationale: Extends an existing mechanism by a table entry and an extension check.

Simplicity: Reuse the relation go already emits for script invocation; do not add upstream's `invokes`.

### [synth] p3 `build` `core-cli`: Let directed path step from a symbol out to its containing file

WHAT: bfsPath (internal/query/query.go:335-371) skips any hop without a stored forward edge (:351). Reproduced by the area audit: `graphify path 'run()' b.py` returns `no directed path ...; retry with --undirected`; with --undirected it returns `run() --calls--> helper() <--contains-- b.py`. MCP shortest_path behaves the same. Upstream adds an implied reverse hop for `contains` edges to the directed search graph only.

WHY (agent value): file-to-file questions ("does a.py depend on b.py") need this. The --undirected retry drops direction on every edge, so on a real graph the shortest route can run through unrelated reverse edges.

HOW: in bfsPath allow the hop when the forward edge is missing but g.edge[{nb,cur}] exists with Relation == "contains". PathEdges already renders such a hop with Forward:false (query.go:314-316) and renderPathChain prints `<--contains--`.

FILES: internal/query/query.go, cmd/graphify/explain_path_test.go.

UPSTREAM: graphify/serve.py _path_search_graph L1774-1787; graphify/cli.py path command L1736-1750; commits e61c5fa, e47d633 (#3878/#4004, 0.9.75).

ACCEPTANCE: the directed `run()` to `b.py` query returns the two-hop chain with the contains edge shown in its true direction. Most useful after the source-file-path resolution bead.

Rationale: Not a silent wrong answer, since go names the retry, hence p3. One condition gives the dependency route the agent asked for.

Simplicity: One extra condition in the BFS; no output change.

### [synth] p3 `build` `core-cli`: Honour [workspace.dependencies] identity in the Cargo crate graph

WHAT: internal/extract/cargo.go:74-104 binds a member's dependency by its key, or by a `package =` rename in the member's own table, to a workspace crate of that name. It never reads root [workspace.dependencies]. From my source read (not run): `foo = { workspace = true }` already gets an edge when the key equals the crate name, so the common case works. Two residual gaps: (a) the rename lives in the root table (`db = { path = "crates/real", package = "real" }` under [workspace.dependencies], member says `db = { workspace = true }`): no edge; (b) an inherited registry or git dep that shares a workspace crate's name gets a false crate_depends_on edge.

WHY (agent value): crate_depends_on edges answer "what depends on this crate" under `--cargo`.

HOW: in the dep loop, when spec has `workspace = true`, replace spec with rootData.workspace.dependencies[depName]; skip unless it has a string `path`; apply the existing package-rename lookup; require that root/path/Cargo.toml is the target crate's manifest.

FILES: internal/extract/cargo.go, test beside cargo_rename_test.go.

UPSTREAM: graphify/cargo_introspect.py; commit 7a508a7 (17 added lines, read in full).

ACCEPTANCE: fixture (a) yields the edge; fixture (b) yields none.

Rationale: The raw item claimed inherited deps are dropped wholesale; cargo.go shows only the rename and same-name cases are wrong, so this is narrower and lower than first reported. Opt-in flag, small fix.

Simplicity: About ten lines in the existing loop. Write fixture (a) first to confirm the gap before changing code.

### [synth] p3 `build` `core-cli`: Discover Cargo.toml below the scan root for --cargo

WHAT: IntrospectCargo reads only root/Cargo.toml (internal/extract/cargo.go:29-34, confirmed by source read) and returns the load error when it is absent, so a monorepo with rust/Cargo.toml gets no crate graph. Upstream now finds a Cargo.toml in a subdirectory when the root has none. Not run in go; how the `--cargo` caller surfaces the error was not checked.

WHY (agent value): silent miss of an existing structured-extraction path in polyglot monorepos.

HOW: when root/Cargo.toml does not exist, pick the shallowest Cargo.toml among the already-collected corpus files (ties broken by sorted path for determinism) and introspect from its directory.

FILES: internal/extract/cargo.go and its caller in cmd/graphify/main.go, cargo test.

UPSTREAM: graphify/cargo_introspect.py; commit 165a7da.

ACCEPTANCE: a corpus with only rust/Cargo.toml yields the crate nodes under `--cargo`.

Rationale: Cheap, but opt-in and narrow, so p3.

Simplicity: Reuse the detect file list instead of a second filesystem walk, so ignore rules apply for free.

### [synth] p4 `skip` `core-cli`: Skip enum-member, macro and annotation node coverage

WHAT: upstream added structural node kinds for languages go extracts: enum members with `case_of` edges for Rust (#3938), Zig (#3940, #4050, #4025), C++ (#3939, #4052), Scala 3 (#3937), PHP (#4026), Julia `@enum` and macros (#3841); Rust `macro_rules!` definitions (#4028); PHP closures as nodes (#3461); Kotlin annotations, primary-constructor properties and class-literal references (#3848, #3965); Scala val/var, type-alias and context-bound references (#4036-#4038, #4085); Go interface method requirement nodes (#3672); C++ in-class method declarations (#3963). go emits none (checked for Rust enum variants, macro_rules and C++ enumerators).

WHY SKIPPED: they grow the node count without adding a calls or imports edge, so explain/path/affected do not improve, and go has no type-reference edges for the variants to be referenced by. An agent finds an enum variant by grep in one step.

UPSTREAM: graphify/extractors/rust.py, zig.py, julia.py, engine.py; graphify/extract.py.

REVISIT: only if go gains `references` edges for type usage.

Rationale: Queryable symbols with no new edges are not worth the determinism-golden churn.

Simplicity: Nothing to do. If one is ever wanted, Rust enum variants are cheapest: a loop over enum_variant_list in internal/extract/rust.go.

Critic amendment: Bundles #3963 (C++ in-class method declarations) under `node count with no new edges`. It is the class-to-method membership link. Reproduced: w.hpp declaring `void show();` plus w.cpp defining `void Widget::show() {}` gives class `Widget` with zero members (`explain Widget` shows only its file), and the out-of-line node `Widget.show()` has no edge to the class and no contains edge from its file either. Upstream 9e291b2 emits the declaration as a method node with a `method` edge so declaration and definition share one node (engine.py:5981-5990; extract.py:4556-4560 indexes it for call binding). That is the normal C++ layout, and the member-call p0 bead's `contains-parent` rule has nothing to bind to without it.

Required change: Split #3963 out as build p2 core-cli: in cpp.go emit a method node for a function_declarator inside a class body, and give an out-of-line `Class::method` definition the same id as the declared member when the class is unique in the corpus. Leave the rest of the bead as skip.

### [synth] p4 `skip` `core-cli`: Skip auto-refresh of the installed skill after an upgrade

WHAT: upstream 0.9.72 (#1805, commit 8ae9878) stamps a `.graphify_version` beside each installed skill and, on any non-install CLI command, rewrites stale skills under a lock with a backup of local edits (GRAPHIFY_NO_AUTO_REFRESH=1 opts out). The same window brought install fixes for CRLF preservation, marker-bounded CLAUDE.md section replace and symlinked instruction files (#3741, #3805). go's install copies one embedded SKILL.md to ~/.claude/skills/graphify (cmd/graphify/install.go, 46 lines) with no stamp, so the skill stays at the old text until `graphify install` is rerun.

WHY SKIPPED: upstream needs this for 15+ platform skill directories; go has one target and one file. Writing into ~/.claude as a side effect of `graphify query` is a surprise and shows up as drift in Ansible-managed dotfiles. Rerunning the idempotent `graphify install` from provisioning covers it. The CRLF, section-replace and symlink fixes do not apply: go never edits a shared instructions file.

UPSTREAM: graphify/__main__.py _refresh_stale_skills L260-331; graphify/install.py (8ae9878, f07423b, c228df2, b30a40a).

REVISIT: only if the skill text starts changing per release.

Rationale: Host configuration, not something an agent calls or parses; the side effect costs more than the staleness.

Simplicity: No go code. If drift ever bites: a Homebrew formula post_install that runs `graphify install` when the skill file already exists.

### [synth] p4 `skip` `core-cli`: Skip graph.json schema_version and graphify_version metadata

WHAT: upstream 0.9.78 (#4167, export.py commit 251722c) writes `schema_version` and `graphify_version` under graph.json's `graph` key, and 0.9.79 had to exclude them from watch's unchanged-topology compare (watch.py _canonical_topology_for_compare, commit c0024cf). go's graph.json carries only `built_at_commit` (internal/export/export.go:77).

WHY SKIPPED: two areas disagreed (one build p3, one skip); this records one decision. No go command reads either key and an agent cannot act on them. go already version-stamps what matters for staleness, the extraction cache (cache.Stamp, internal/cache/cache.go:103), and the graph_stats bead surfaces built_at_commit, the provenance that has a reader. graphify_version would change graph.json on every release for an unchanged repo, which breaks the byte-identical guarantee and churns every golden.

UPSTREAM: graphify/export.py (#4167); graphify/watch.py.

REVISIT: when a real consumer needs to detect format drift.

Rationale: Additive metadata with no reader and a determinism cost.

Simplicity: Nothing to do. If asked: one omitempty SchemaVersion field beside BuiltAtCommit in internal/export/export.go and the matching field in internal/query/merge.go; leave the build version out.

### [synth] p4 `skip` `core-cli`: Record v0.9.66-v0.9.80 fixes checked and found not applicable to go

Recorded so the next audit does not re-derive it. All findings are from the per-area audits.

RECONCILE/CACHE (build.py, watch.py, cache.py): go has no merge-into-existing-graph path; assemble() caches raw per-file results and writeOutputs re-runs Resolve and graph.Build over the whole corpus (cmd/graphify/main.go:214-298, :305-315). Not applicable: #4175, #4181, #4161, #3981, 288a938 (--no-cluster endpoints), #3989, #3850, #3774, stale .graphify_root fallback, Windows rebuild lock (prior skip, docs/upstream-delta-v0.9.65.md:538). Same-relation confidence collision (5da2da3) is possible in principle since AddEdge keeps the first edge, but zero differing-confidence duplicates were found on this repo (4058 edges) and the upstream checkout (36844 edges). Empirical, not a proof.

CLUSTER/ANALYZE/DEDUP: 13 commits in cluster.py, analyze.py, dedup.py only. graph_diff direction (#4170): go keys edges without sorting endpoints (internal/query/diff.go:88-91). Surprise root-file bonus (#3934), --exclude-hubs singletons (#3933), dedup fixes (#3825, #3786, #4065), suggest_questions ordering (#3972): go has none of those paths.

SERVING/CLI: MCP perf cache 39f004e (go builds indexes once in query.Load); hook-guard 8e5649f/7a1b12b; merge-graphs --previous 9ec7df0; --allow-dedup-shrink; PYTHONHASHSEED re-exec; edge-direction restore 5eeb037 (networkx artefact); URL ingest host classification; --help banner (human-only). mcp_ingest.py, scip_ingest.py, querylog.py, prs.py have zero commits since v0.9.65, so prior skips and deferred bead graphify-go-u5z stand.

LANGUAGES: Terraform secret redaction (#3817, #3762, #3870); Rust `const _` self-loop (#4108), forward-referenced types (#3903), prelude god nodes (#3973); C# #3815, #3877, #4165, #4017; cross-language base-class binding (#4068, guarded at resolve.go:120); Kotlin #3915; Lua #3994; Java #3932; Verilog .vh (#3983, extract.go:145); path-leaking unresolved-import ids (#4155, #4157); csharp_dispatch.py refactor (no behaviour change).

NOT CHECKED: upstream was never executed; paths.py changes; cross_repo_calls.py, cross_repo_types.py, symbol_resolution.py, file_slice.py small diffs; detect.py manifest re-anchoring beyond the update-subdir bead; per-platform skill markdown; whether a refused anti-shrink update makes `graphify watch` rebuild every tick (worth one test, not upstream drift). Stale comment: internal/analyze/analyze.go:27-30 says GodNodes uses the threshold cluster() applies, but go's cluster applies none.

Rationale: Each fix patches a mechanism go does not have, or a feature a prior audit already skipped.

Simplicity: No code change. If the confidence collision ever shows up, add a confidence-rank tie-break to the sort comparator in internal/graph/build.go:31-40.

### [synth] p4 `skip` `out-of-scope`: Skip COBOL, Erlang, R, Solidity, VB.NET extractors and Swift dispatch

WHAT: upstream 0.9.66 added five extractors (cobol.py, erlang.py, r.py, solidity.py, vbnet.py) with fixes through 0.9.78, and 0.9.75 lifted C# interface dispatch into interface_dispatch.py to reuse it for Swift protocols (swift_dispatch.py). _LANG_FAMILY gained the matching entries. go extracts none of these languages.

WHY SKIPPED: long-tail languages, declared out of scope in GOALS.md and in the prior audit's non-port list. Upstream adding more does not change that. COBOL is the only dependency-free one and no target repo contains it; the others each need a new grammar dependency. Swift has no Go tree-sitter binding (GOALS.md Language coverage).

UPSTREAM: graphify/extractors/cobol.py, erlang.py, r.py, solidity.py, vbnet.py; graphify/interface_dispatch.py; graphify/swift_dispatch.py.

Rationale: None is both cheap and wanted by a real corpus.

Simplicity: Nothing to do. Adding one later is a new internal/extract/<lang>.go plus a case in FileFromBytes (extract.go:113).

### [synth] p4 `skip` `out-of-scope`: Skip LLM backend, transcribe, Google Workspace, Postgres introspect and URL ingest drift

WHAT: upstream llm.py fixes since v0.9.65 (ollama num_ctx merge, file char cap warning, label retry and partial labels) plus ingest.py URL host classification and tweet URL normalisation all sit in modules go deliberately does not port. No new capability class appeared.

WHY SKIPPED: GOALS.md "Out of scope" excludes LLM-based semantic extraction and backends, transcription, Office/Postgres/Google ingest. The drift is bug fixes inside those modules, so the prior decision stands unchanged.

UPSTREAM: graphify/llm.py, ingest.py, transcribe.py, google_workspace.py, pg_introspect.py.

Rationale: Nothing upstream changed materially since the v0.9.65 audit's skip.

Simplicity: No go files to touch.

### [critic] p0 `build` `core-cli`: Skip nested git worktrees and dot-dir worktrees/ in detect

WHAT: internal/detect/detect.go skipDirs (L71-76) lists `.worktrees` only. A linked worktree under `.claude/worktrees/<name>/` (Claude Code's default, and this user's mandated worktree-per-session layout) is walked as source. Reproduced on this repo with a binary from main: `GRAPHIFY_OUT=/tmp/gout graphify build .` produced 146 nodes with source_file under `.claude/worktrees/claude-installer-8f2c/` (1742 nodes total). Node ids are dir+stem based, so the copy merges into the real nodes: `graphify explain 'GodNodes()'` printed `<- contains analyze.go .claude/worktrees/claude-installer-8f2c/internal/analyze/analyze.go:26` and `-> calls isConceptNode() .claude/worktrees/.../analyze.go:31` beside the real edges. `.claude/worktrees/` is untracked and not gitignored here (`git status` shows `?? .claude/worktrees/`). The CI-built committed graph is clean, so only local build/update/hook runs are hit.

WHY (agent value): the stale branch copy contributes edges and source locations to the live node, so explain/path/affected mix two versions of the code with no marker. The agent cannot tell without opening both files.

HOW: in the detect walk, skip a directory named `worktrees` whose parent name starts with `.`, and skip any non-root directory that contains a `.git` FILE (linked worktree or submodule gitdir pointer). Bump nothing; this is file selection.

FILES: internal/detect/detect.go, detect_test.go.

UPSTREAM: graphify/detect.py:1153-1154 (`worktrees` inside a dotted dir), :1294-1320 (nested-worktree detection, #1810), :954 (`.worktrees`, #947). All present at v0.9.65 (detect.py:1033-1034 at that tag); docs/upstream-delta-v0.9.65.md has no entry for it.

ACCEPTANCE: a corpus with `.claude/worktrees/x/a.go` and a dir holding a `.git` file yields no node from either; `.claude/workflows/` and `.github/` are still indexed.

Rationale: Confident wrong answer on the primary local workflow, reproduced on this repo. Pre-baseline upstream behaviour both audits missed.

Simplicity: Two conditions in the existing walk. Skip submodule-content policy beyond the `.git` file test.

### [critic] p1 `build` `core-cli`: Route .h headers containing C++ to the C++ extractor

WHAT: internal/extract/extract.go:141 sends every `.h` to the C grammar. Reproduced: w.h with `class Widget { public: void show(); virtual int area() const = 0; int count; };` yields a single node labelled `Widget()` (function shape), no class, no members; `graphify affected w.h` lists `Widget()` as the changed symbol. The same text as w.hpp yields class node `Widget`.

WHY (agent value): Qt, LLVM, Unreal and most C++ codebases declare classes in `.h`. Every such class is a wrong-kind node. It also makes the export-macro bead inert on exactly the corpora its description cites, because cpp.go never sees those files.

HOW: in the `.h` case, sniff the first 256 KiB for C++-only markers (upstream's list: `class `, `namespace `, `template<`, `::`, `public:` etc.; copy _CPP_HEADER_MARKERS) and dispatch to the C++ extractor on a hit. Bump the extractor cache version.

FILES: internal/extract/extract.go, cpp_test.go.

UPSTREAM: graphify/extract.py:7254-7296 (_is_cpp_header, _CPP_HEADER_MARKERS, #1547). Present at v0.9.65 (extract.py:6487); the prior audit has no entry. ObjC sniffing does not apply, go has no ObjC extractor.

ACCEPTANCE: the Widget fixture in a `.h` yields a class node; a plain C header is unchanged. Land before the export-macro bead.

Rationale: Wrong node kind for the dominant C++ header extension, reproduced. Prerequisite for the export-macro bead to have any effect on its stated targets.

Simplicity: One bytes.Contains loop over a marker list in the existing switch.

### [critic] p1 `build` `core-cli`: Redact secrets in Terraform attribute values that fall through to raw source text

WHAT: attrValue (internal/extract/terraform.go:279-282) renders function calls and other non-literal expressions as raw source text, and tuple scalars verbatim, with redaction keyed only on the attribute key or block label. Reproduced with a binary from main, values found in graph.json: `container_definitions = jsonencode([{ name = "x", environment = [{ name = "API_TOKEN", value = "hunter2json" }] }])` (stored whole), `env = ["DB_PASSWORD=hunter2inline"]`, and `output "conn" { value = "postgres://u:hunter2out@h/db" }`. The cases upstream fixed in this window are already safe in go: `environment = [{name="DB_PASSWORD", value=...}]`, `configs = [{password=...}]`, `settings = {password=...}`, `variable "db_password" { default }` produced no leak (nested objects are dropped, block labels are checked), so the backlog's not-applicable verdict on #3762/#3817/#3870 holds for those exact shapes.

WHY (agent value): this repo and its consumers commit or share graph.json, and MCP get_node prints attributes into agent context. jsonencode'd container_definitions is the standard ECS form, the same idiom #3870 targeted.

HOW: in blockAttributes' default branch, after computing v: if the value node is not a plain string/number/bool literal or var reference and sensitiveKeyRe matches v, store redactedValue; for string values, redact when v matches `://[^/@\s]+:[^/@\s]+@`. Apply the same test per tuple element.

FILES: internal/extract/terraform.go, terraform_test.go.

UPSTREAM: graphify/extractors/terraform.py:38-70 (_redact_value), :315-365 (_parse_attr_value else-branch returns raw text). By source read upstream has the same hole for function calls and scalars; upstream was not executed (no tree_sitter installed). So this is a same-class residual, not a port.

ACCEPTANCE: the three fixtures above leave no `hunter2` string in graph.json; `instance_type = "t3.large"` and `ami = data.aws_ami.x.id` are unchanged.

Rationale: Secret values in a committed artifact and in agent context, reproduced. The prior audit called redaction non-negotiable (docs/upstream-delta-v0.9.65.md:860).

Simplicity: One regexp test on the fallback path plus a URL-credential pattern. No HCL function evaluation.

### [critic] p3 `build` `core-cli`: Record calls to plainly imported external Python modules

WHAT: upstream 0.9.76 records `module.func()` on a plainly imported dependency as a calls edge to the module node, fail-closed on receiver shadowing and non-unique bindings. Verified absent in go: a.py with `import os`, `import json` and a method calling `os.getcwd()` / `json.dumps({})` yields the two file-level imports edges and no calls edge.

WHY (agent value): answers "which functions shell out via subprocess / hit requests" at function granularity; today only the importing file is known.

HOW: after the member-call p0 bead adds Call.Recv: when Recv equals the local name bound by a top-level `import x` / `import x as y` in the same file, the name is not rebound in the function, and no corpus definition was matched, emit caller -calls-> the existing import concept node.

FILES: internal/extract/python.go, resolve.go, tests.

UPSTREAM: commit 6868fff (#4043, #3793), graphify/extract.py.

ACCEPTANCE: the fixture yields `A.go -calls-> os`; a local variable named `os` yields none.

Rationale: New upstream capability in the audited window that the backlog neither builds nor records as skipped. Low priority: missing edge, never a wrong one.

Simplicity: Reuses Call.Recv and the import node already emitted. Blocked by the member-call bead; drop it if that bead slips.
