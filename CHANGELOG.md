## Unreleased

- graphify builds without cgo. The parser is gositter, a Go port of the
  tree-sitter runtime, so `go install` and `go build` need no C toolchain.
- Release binaries for darwin and linux, arm64 and amd64, are cross-built on
  one runner with `CGO_ENABLED=0`. CI builds and tests the same way.
- Cold build of the Kubernetes source tree on an M4 Pro with 14 cores: 13.1 s
  and 3.3 GB peak in v0.10.1, 1.5 s and 0.5 GB peak now.
- A parse that makes no progress for 65,536 parser steps in a row is stopped:
  the file keeps only its file node, stderr names it, and `GRAPH_REPORT.md`
  counts it. `GRAPHIFY_PARSE_BUDGET` sets the limit, `0` turns it off.
- The first `graphify update` after the upgrade drops the parse cache and
  parses every file again, once, with a note on stderr. `graph.json` and
  `GRAPH_REPORT.md` do not change.
- Pooled parsers share their subtree heap chunks: a little CPU on each parse
  for a lower peak on trees with a few huge files.
- The GC target is 50 from resolve to the end of the output writes: about 5 to
  7 percent CPU for about 20 percent lower peak memory on a Kubernetes-sized
  tree.
- `GFY_CPUPROFILE` and `GFY_MEMPROFILE` are documented, cover `update` as well
  as `build`, and warn on a path that cannot be created instead of panicking.

### 📚 Documentation

- Update changelog for v0.6.0 [skip ci]
- Seed v0.7.0 integration branch

### 🔧 Miscellaneous Tasks

- Regenerate knowledge graph [skip ci]

### 🚀 Features

- Add config/manifest .json extractor (#33)

