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

Or from source (requires a C toolchain — extraction uses cgo/tree-sitter):

```bash
go install github.com/dobbo-ca/graphify-go/cmd/graphify@latest
```

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
- **cluster** — Louvain community detection (gonum), with oversized-community
  splitting.
- **analyze** — god nodes (most connected), surprising cross-file connections,
  import cycles.
- **report / export** — `GRAPH_REPORT.md` (an audit trail of the corpus and its
  core abstractions), `graph.json` (NetworkX node-link format), and the
  agent-consumable `graphml` / `dot` / `csv` exports.

## Use it in your own repo

1. Add `graphify build` to CI on merge to `main` and commit `graphify-out/` —
   see [.github/workflows/graph.yml](.github/workflows/graph.yml).
2. Copy `skills/graphify/SKILL.md` into your project's skills, and add the
   "Use the knowledge graph, not grep" block from [CLAUDE.md](CLAUDE.md).

## Scope

Languages today: **Go, JavaScript, TypeScript, Terraform/HCL, Python, Rust, C,
C++, Java, C#, Ruby, PHP, Bash, Scala, Julia, Verilog/SystemVerilog, Kotlin,
Lua, Zig** — every grammar with a Go tree-sitter binding. `detect` also honours
`.gitignore`.

The original's LLM-based semantic extraction, video/image/Office ingest,
Postgres/Neo4j/Obsidian/SVG exports, human-facing visualizers (the
force-directed `graph.html` browser viewer and the Mermaid call-flow page), and
the multi-assistant installers need external services or a far larger surface —
or are human visualization graphify-go deliberately drops — and stay out of
scope.
