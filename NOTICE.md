# Attribution

`graphify-go` is a Go reimplementation of **graphify** by Safi Shamsi.

- Original project: https://github.com/safishamsi/graphify (Python, MIT licensed)
- This port: https://github.com/dobbo-ca/graphify-go (Go, MIT licensed)

The original graphify turns a folder of code into a queryable knowledge graph
(`detect → extract → build → cluster → analyze → report → export`). This port
re-implements that pipeline in Go for two uses:

1. A CI step that regenerates the graph on merge to `main` and commits it.
2. A Claude Code skill that reads the graph instead of grepping the tree.

It is a clean-room reimplementation of the original's **schema and pipeline**,
not a line-by-line translation. The extraction output schema (nodes/edges with
`id`, `label`, `file_type`, `source_file`, `source_location`, `relation`,
`confidence`) and the `graph.json` format are kept compatible with upstream so
the same downstream tooling works.

Scope today: Go, JavaScript, and TypeScript extractors. Python and Rust are
planned. The original's LLM-based semantic extraction, Obsidian/Neo4j/SVG
exports, MCP server, and AI-assistant installers are intentionally omitted.

The original MIT copyright is preserved in `LICENSE`.
