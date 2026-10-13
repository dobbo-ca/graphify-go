# graphify-go

Turn a folder of code into a queryable knowledge graph — files, functions,
types, and how they call and import each other — built for **AI agents to
navigate a codebase from the command line** instead of grepping the tree.

graphify-go is agent-first by design. Its primitives (`query`, `explain`,
`path`, `affected`, `update`, `validate`, `serve`) resolve symbols and
relationships directly, so an agent spends far fewer tokens than scanning files
by hand. It does not ship a human-facing visualizer; the graph is something an
agent reads, not something a person browses.

A Go port of [graphify](https://github.com/safishamsi/graphify) by Safi Shamsi.
See [NOTICE.md](NOTICE.md) for attribution; both projects are MIT licensed.

## Why

**Agent navigation, first-class.** A Claude Code [skill](skills/graphify/SKILL.md)
and a `CLAUDE.md` block tell the agent to query the graph (`graphify query` /
`explain` / `path` / `affected`) instead of grepping — faster, and far cheaper
in tokens. For a resident agent session, `graphify serve` exposes the same
primitives over MCP so the graph loads once and answers many structured queries.

A CI workflow regenerates the graph on every merge to `main` and commits it, so
`graphify-out/graph.json` always reflects the code an agent is working against.

## Install

Homebrew:

```bash
brew install dobbo-ca/taps/graphify-go
```

Or from source (pure Go, no C toolchain needed):

```bash
go install github.com/dobbo-ca/graphify-go/cmd/graphify@latest
```

The parser is [gositter](https://github.com/dobbo-ca/gositter), a Go port of
the tree-sitter runtime. `go install` and `go build` work with `CGO_ENABLED=0`
and cross-compile every target from one host.

Releases are cut automatically from conventional commits (Uplift) and published
as GitHub releases + a Homebrew formula on every merge to `main`.

## Usage

```bash
graphify build [path]        # build graph.json + GRAPH_REPORT.md under <path>/graphify-out
graphify update [path]       # rebuild incrementally, re-parsing only changed files
graphify watch [path]        # rebuild incrementally as files change (Ctrl-C to stop)
graphify hook <install|uninstall|status> [path] # manage git hooks that update the graph after commits
graphify query <pattern>     # find nodes by name (regex, case-insensitive)
graphify explain <node>      # show a node and its neighbours (calls, called-by, imports, contains)
graphify path <from> <to>    # shortest dependency path between two nodes
graphify affected [file...]  # nodes defined in changed files + everything that depends on them
graphify ask <question>      # natural-language graph-context retrieval over the graph
graphify diff <old> <new>    # nodes and edges added/removed between two graph.json snapshots
graphify validate            # check graph.json for structural problems
graphify export <fmt> [path] # convert graph.json to graphml, dot, or csv an agent can consume
graphify serve               # MCP stdio server: load graph.json once, answer structured queries
graphify extract <file>      # print one file's extracted nodes/edges (debug)
```

Example:

```bash
$ graphify build .
built graph: 21 files · 186 nodes · 395 edges · 8 communities → graphify-out

$ graphify explain "Build()"
Build()  [code]
  internal/graph/build.go L27
  -> calls        New()              internal/model/model.go:46
  <- calls        cmdBuild()         cmd/graphify/main.go:58
  <- contains     build.go           internal/graph/build.go:1
```

## Pipeline

```
detect → extract → build → cluster → analyze → report → export
```

- **detect** — walk the tree, skip deps/build/cache dirs and anything that looks
  like secrets.
- **extract** — tree-sitter parse each file into nodes (file, function, type,
  method) and edges (`contains`, `calls`, `imports`/`imports_from`).
- **build** — assemble the undirected graph; drop dangling and phantom
  cross-language inferred calls.
- **cluster** — Louvain community detection, with oversized-community
  splitting.
- **analyze** — god nodes (most connected), surprising cross-file connections,
  import cycles.
- **report / export** — `GRAPH_REPORT.md` (an audit trail of the corpus and its
  core abstractions), `graph.json` (NetworkX node-link format), and the
  agent-consumable `graphml` / `dot` / `csv` exports.

### Performance notes

Cold build of the Kubernetes source tree (M4 Pro, 14 cores):

| Version | Wall time | Peak memory |
|---------|-----------|-------------|
| v0.10.1 (cgo tree-sitter) | 13.1 s | 3.3 GB |
| now (gositter) | 1.5 s | 0.5 GB |

Two places spend CPU to keep peak memory down, both on purpose:

- Parsers are pooled, and after every file each one keeps 8 subtree heap
  chunks and hands the rest back to a pool shared by all parsers. One huge
  file does not leave its memory pinned in an idle parser; the cost is the
  hand-back on each parse.
- From resolve to the end of the output writes the Go GC target is 50 instead
  of 100. On a Kubernetes-sized tree that costs about 5 to 7 percent CPU for
  about 20 percent lower peak memory. A lower `GOGC`, or `GOGC=off`, is left
  as set.

`graphify update` keeps a parse cache in `graphify-out/`. A cache written by
another graphify version is dropped and rebuilt once, with a note on stderr.

### Files the parser gives up on

A few inputs make a tree-sitter grammar retry the same byte without end: 54
bytes of Julia can take 14 seconds and 1.4 GB. graphify stops a parse after
65,536 parser steps in a row without progress. A file over 4 KB gets 16 more
steps for each byte past that, up to 262,144 steps at 16 KB: after a broken
region the parser can spend several such steps per byte and still finish.
Past 16 KB the limit falls again, in proportion to the size, back to 65,536
from 64 KB: one such step can take longer in a larger file. The file keeps
its file node and nothing else, a warning on stderr names it on every build
and update, and `GRAPH_REPORT.md` counts such files in its Summary.

`GRAPHIFY_PARSE_BUDGET` sets the limit for a file of up to 4 KB, or of 64 KB
and more: a whole number of steps, `0` for no limit. The sizes between scale
it the same way, up to 4 times. The largest count in eight real repositories
(about 71,000 parsed files) is 588, and no file there uses more than 1/261 of
its limit. A file over about 22 KB of one repeated bad token can still reach
the limit; raise the limit if a file you need is skipped. The limit does not
bound time: some broken inputs keep making progress and still take seconds
per 10 KB. The parse cache is kept per limit, so the first `update` after
a change parses every file again; a git hook must see the same value as your
shell, or each of the two rebuilds the cache.

### Profiling a build

`graphify build` and `graphify update` write Go profiles when asked:
`GFY_CPUPROFILE=cpu.pprof` for CPU and `GFY_MEMPROFILE=mem.pprof` for
allocations. Unset, they cost nothing. A path that cannot be created prints a
warning and the run goes on without that profile.

```bash
GFY_CPUPROFILE=cpu.pprof GFY_MEMPROFILE=mem.pprof graphify build .
go tool pprof -top cpu.pprof
go tool pprof -sample_index=alloc_space -top mem.pprof
```

## Use it in your own repo

1. Add `graphify build` to CI on merge to `main` and commit `graphify-out/` —
   see [.github/workflows/graph.yml](.github/workflows/graph.yml).
2. Copy `skills/graphify/SKILL.md` into your project's skills, and add the
   "Use the knowledge graph, not grep" block from [CLAUDE.md](CLAUDE.md).

## Scope

Languages today: **Go, JavaScript, TypeScript, Terraform/HCL, Python, Rust, C,
C++, Java, C#, Ruby, PHP, Bash, Scala, Julia, Verilog/SystemVerilog, Kotlin,
Lua, Zig**. Each one has a grammar package in gositter. `detect` also honours
`.gitignore`.

The original's LLM-based semantic extraction, video/image/Office ingest,
Postgres/Neo4j/Obsidian/SVG exports, human-facing visualizers (the
force-directed `graph.html` browser viewer and the Mermaid call-flow page), and
the multi-assistant installers need external services or a far larger surface —
or are human visualization graphify-go deliberately drops — and stay out of
scope.
