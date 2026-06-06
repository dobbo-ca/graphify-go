---
name: graphify
description: Use when exploring or understanding a codebase's structure — finding where a function/type is defined, what calls or imports something, tracing a dependency chain, or locating a subsystem's core abstractions. Query the prebuilt knowledge graph (graphify-out/graph.json) with the graphify CLI instead of grepping the source tree. Triggers on questions like "where is X defined", "what calls X", "what does X depend on", "how does this project fit together".
---

# graphify — query the code graph, don't grep

This project ships a prebuilt knowledge graph at `graphify-out/graph.json`,
regenerated on every merge to `main`. It maps files, functions, types, and
methods (nodes) and their `contains` / `calls` / `imports` relationships
(edges). Querying it is faster and far cheaper than grepping.

## Use it when you need to

- **Find a definition** → `graphify query <pattern>` (regex, case-insensitive)
- **Understand one symbol** → `graphify explain <node>` — shows its source
  location plus every neighbour: what it `calls`, what `calls` it, what file
  `contains` it, what it `imports`.
- **Trace a dependency chain** → `graphify path <from> <to>` — shortest path
  between two nodes.

## Workflow

1. Start at the graph, not the filesystem. To answer "how does auth work?",
   run `graphify query auth`, then `graphify explain` the most relevant hit.
2. Follow edges with `explain` until you've found the right
   `source_file:line`.
3. Only then open the file — the graph has already told you which lines matter.
4. Read `graphify-out/GRAPH_REPORT.md` for the project's "god nodes" (core
   abstractions), surprising cross-file connections, and import cycles.

## If the graph is missing or stale

`graph.json` records the commit it was built from (`built_at_commit`). If it is
absent or behind `git rev-parse HEAD`, rebuild it:

```bash
graphify build .
```

Supported languages: Go, JavaScript, TypeScript. For files in other languages,
fall back to reading the source directly.
