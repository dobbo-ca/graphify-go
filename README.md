# graphify-go

Turn a folder of code into a queryable knowledge graph — files, functions,
types, and how they call and import each other — so you (and your coding agent)
can navigate the codebase by querying a graph instead of grepping the tree.

A Go port of [graphify](https://github.com/safishamsi/graphify) by Safi Shamsi.
See [NOTICE.md](NOTICE.md) for attribution; both projects are MIT licensed.

## Why

Two uses, both first-class:

1. **CI artifact** — a workflow regenerates the graph on every merge to `main`
   and commits it, so `graphify-out/graph.json` always reflects the code.
2. **Agent navigation** — a Claude Code [skill](skills/graphify/SKILL.md) and a
   `CLAUDE.md` block tell the agent to query the graph (`graphify query` /
   `explain` / `path`) instead of grepping, which is faster and uses far fewer
   tokens.

## Install

```bash
go install github.com/dobbo-ca/graphify-go/cmd/graphify@latest
```

Requires a C toolchain (cgo) — extraction uses tree-sitter.

## Usage

```bash
graphify build [path]        # build graph.json + GRAPH_REPORT.md + graph.html under <path>/graphify-out
graphify query <pattern>     # find nodes by name (regex, case-insensitive)
graphify explain <node>      # show a node and its neighbours (calls, called-by, imports, contains)
graphify path <from> <to>    # shortest dependency path between two nodes
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
- **report / export** — `GRAPH_REPORT.md`, `graph.json` (NetworkX node-link
  format), and a small `graph.html` viewer.

## Use it in your own repo

1. Add `graphify build` to CI on merge to `main` and commit `graphify-out/` —
   see [.github/workflows/graph.yml](.github/workflows/graph.yml).
2. Copy `skills/graphify/SKILL.md` into your project's skills, and add the
   "Use the knowledge graph, not grep" block from [CLAUDE.md](CLAUDE.md).

## Scope

Languages today: **Go, JavaScript, TypeScript**. Python and Rust are planned.
The original's LLM-based semantic extraction, Obsidian/Neo4j/SVG exports, MCP
server, and AI-assistant installers are intentionally out of scope.
