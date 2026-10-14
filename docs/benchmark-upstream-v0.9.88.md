# Benchmark: graphify-go against upstream graphify 0.9.88

Bead: graphify-go-9j5.13. Measured 2026-10-12 and 2026-10-13. Measure only: no product code changed.

## Result

On all seven corpora, for both a cold build and an update with no changed file, graphify-go at f442d3f (built with `CGO_ENABLED=0`) is faster and uses less CPU and less memory than upstream graphify 0.9.88. It is also faster and smaller than the v0.10.1 cgo build in every cell.

| Ratio, upstream / graphify-go no-cgo | Cold build | No-change update |
|---|---|---|
| Wall time | 60x to 254x | 68x to 444x |
| CPU time, user + system | 22x to 39x | 21x to 105x |
| Peak RSS, summed over the process tree | 8x to 12.5x | 8x to 28x |
| Peak RSS, largest single process | 2.6x to 12.4x | 8x to 27x |

| Ratio, v0.10.1 cgo / no-cgo | Cold build | No-change update |
|---|---|---|
| Wall time | 3.3x to 8.6x | 3.4x to 11.1x |
| CPU time | 3.3x to 5.6x | 1.4x to 2.8x |
| Peak RSS | 1.9x to 6.1x | 2.3x to 5.9x |

Read these numbers with four limits. Each one came out of the refutation pass described below.

1. graphify-go does less work per file. It writes between 35% (opencv) and 90% (graphify) of upstream's nodes, 61% on microsoft/TypeScript and 78% on vscode. Part of that is upstream output with no meaning, part is a documented skip, and part is structure graphify-go misses (C and C++ code inside preprocessor conditionals, JS/TS nested and namespace declarations, node id collisions). The parse cost of the skipped code is already in the graphify-go times, because it parses every file completely. The cost of resolving, clustering and writing the extra nodes is not. Epic graphify-go-vi5 tracks the gaps, and this benchmark has to be run again when they are closed.
2. The wall ratios depend on 14 cores and on upstream's clustering fallback. On opencv, cold build: 131x as measured, 47x with clustering off on both sides, 29x with both tools held to one core. Upstream was installed the way its README says (`pip install graphifyy`), which leaves out the optional `leiden` extra, so it clusters with networkx Louvain in pure Python. About two thirds of its opencv cold time is that stage.
3. The no-change row does not compare the same work. On opencv and vscode upstream never reached its "no topology change" skip: it rebuilt, clustered again and rewrote the graph on every update. On microsoft/TypeScript and vscode it re-parsed 83% and 88% of the files, because it does not cache JS/TS parses. Where upstream did skip (graphify, redis, curl, kubernetes) the no-change wall ratio is 68x to 299x. graphify-go skips parsing but always resolves, clusters and rewrites.
4. The machine was not quiet. Other processes used about 1 core of 14 during a typical run and up to 2.2. The output directory was on a RAM disk for kubernetes runs 2 and 3 and for all vscode runs, because the SSD filled up.

There is no cell where graphify-go no-cgo is slower or uses more memory than upstream or than the cgo build. One disk number goes the other way for the cgo build only: on vscode the v0.10.1 output directory (539 MB) is larger than upstream's (508 MB). The no-cgo build writes 447 MB there.

## Machine and versions

- Apple M4 Pro, 14 cores (10 performance, 4 efficiency), 64 GB, macOS 27.0.1, APFS SSD.
- graphify-go no-cgo: commit f442d3f, go 1.27.1, `CGO_ENABLED=0 go build ./cmd/graphify`.
- graphify-go cgo: v0.10.1, commit 54d6158 with local modifications, the baseline binary kept from the cgo removal work.
- Upstream: `graphifyy==0.9.88` from PyPI (the newest release on 2026-10-12) in a virtual environment, Python 3.14.8, networkx 3.7, tree-sitter 0.25.2. No extras. The previous parity baseline in this repo is upstream 0.9.80.

## Method

Commands:

| | Cold build | No-change update |
|---|---|---|
| graphify-go, both binaries | `graphify build <dir>` | `graphify update <dir>` |
| upstream | `graphify update <dir>` | `graphify update <dir>` |

`graphify update` is upstream's full pipeline without an LLM: detect, parse, build, cluster, analyse, report, write. `graphify extract --code-only` stops before the report and skips markdown, so it is not the match for `graphify build`.

Environment for every run: `GRAPHIFY_OUT=<absolute dir outside the corpus>`. For upstream also `GRAPHIFY_NO_AUTO_REFRESH=1 GRAPHIFY_NO_TIPS=1 GRAPHIFY_NO_BACKUP=1 GRAPHIFY_VIZ_NODE_LIMIT=0 GRAPHIFY_MAX_GRAPH_BYTES=8GB`, and every `*_API_KEY` variable removed. `GRAPHIFY_VIZ_NODE_LIMIT=0` turns off `graph.html`, which graphify-go does not write; it costs upstream 1.3 s of 61 s on opencv. Both tools used their default worker count, 14.

File set. The two tools read different files by default, so each corpus was reduced to the files both tools hand to an extractor, hardlinked into a fresh tree with no `.git` and no ignore files. `prep.py` does this. The package manifests graphify-go reads in a separate pass (`go.mod`, `pyproject.toml`, `pom.xml`, `Cargo.toml`, outside `vendor/` and similar directories) were then added by hand, because the first attempt left them out and that broke Go module import resolution for both tools. The numbers from that attempt were thrown away.

| Corpus | Commit | graphify-go default | upstream default | Both | Manifests added | Only graphify-go | Only upstream |
|---|---|---|---|---|---|---|---|
| Graphify-Labs/graphify | 6478eb7 | 886 | 948 | 885 | 3 | 1 | 63 |
| redis/redis | 940a4d7 | 1,458 | 1,460 | 1,457 | 0 | 1 | 3 |
| curl/curl | 2874cc6 | 2,068 | 2,086 | 2,068 | 0 | 0 | 18 |
| opencv/opencv | 73a26a4 | 5,003 | 4,783 | 4,485 | 3 | 518 | 298 |
| microsoft/TypeScript | c0ca4744 | 31,924 | 31,973 | 31,924 | 3 | 0 | 49 |
| kubernetes/kubernetes | ab4fb775 | 15,354 | 20,008 | 15,344 | 40 | 10 | 4,664 |
| microsoft/vscode | 636f8117 | 16,376 | 16,514 | 16,376 | 4 | 0 | 138 |

The large upstream-only count on kubernetes is `vendor/` (4,608 files): upstream indexes it and graphify-go skips it. With default settings on the original repository the gap on kubernetes would be wider than reported here. On opencv graphify-go alone reads `.markdown` and `.hh` files; upstream alone reads `.cl`, `.mm`, `.swift` and `.cu`. All corpora are shallow clones.

Runs. Three runs per corpus, interleaved: the tool order rotates each round (go, cgo, upstream; upstream, go, cgo; cgo, upstream, go). For each tool in a round the output directory is deleted, the cold build runs, then after 3 seconds the no-change update runs. One process tree at a time, under `nice -n 19` (measured to make no difference). A watchdog stops a run at 40 GB of tree RSS or 90 minutes; it never fired. Before each run the loop waits until the CPUs are at least 85% idle.

Metrics, from `run1.py`:

- Wall: monotonic clock around the child, exact to about 10 ms.
- User and system CPU: `wait4` rusage, which includes every descendant the tool waits for. A second agent checked that upstream's worker pool is waited for.
- Peak RSS: the larger of `ru_maxrss` (largest single process) and the sum of RSS over the process tree sampled every 0.1 s. The sum counts shared pages once per worker, so for upstream's 16 processes on a small corpus it is an upper bound; the single-process ratio is in the result table for that reason. For graphify-go, a single process, the two agree.

Each cell below is the median of 3 runs with the minimum and maximum in brackets. kubernetes upstream cold has 5 runs, because two extra cold runs finished before the disk filled.

## Scope differences

| | graphify-go | upstream 0.9.88 |
|---|---|---|
| LLM or semantic stage | none by default | none on `update` |
| Parsing | all cores, threads | 14 worker processes |
| Clustering | own Louvain, seed 42 | networkx Louvain, seed 42 (Leiden only with the `leiden` extra) |
| Analysis | scores, god nodes, surprising connections, import cycles | the same plus suggested questions, which runs a sampled betweenness pass |
| Written | graph.json, GRAPH_REPORT.md, cache, stat index | graph.json, GRAPH_REPORT.md, labels, manifest, AST cache (graph.html turned off) |
| Cold-run cache | none read | none read; every cold log shows all files uncached |
| No-change update | skips parsing; resolves, clusters and rewrites every time | re-walks, re-parses all JS/TS, rebuilds the graph, skips clustering and writes only if the topology is unchanged |
| Edge storage | one edge per source, target and relation | one edge per unordered node pair |
| `vendor/` | skipped | indexed |

## Results per corpus

### Graphify-Labs/graphify @ 6478eb7

Files read by default: graphify-go 886, upstream 948. Benchmarked on the 885 files both read (14 MB), plus 3 package manifest files.

| Phase | Metric | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 | upstream / no-cgo |
|---|---|---|---|---|---|
| Cold build | wall s | 0.12 (0.12 to 0.12) | 0.72 (0.71 to 0.76) | 12.0 (12.0 to 12.2) | 99.1x |
| Cold build | user CPU s | 0.67 (0.66 to 0.67) | 4.24 (4.10 to 4.26) | 18.4 (18.3 to 18.5) | 27.3x |
| Cold build | system CPU s | 0.14 (0.14 to 0.14) | 0.26 (0.26 to 0.29) | 3.87 (3.86 to 3.92) | 28.0x |
| Cold build | peak RSS MB | 133 (128 to 137) | 278 (259 to 280) | 1,166 (1,165 to 1,168) | 8.8x |
| No-change update | wall s | 0.09 (0.09 to 0.09) | 0.40 (0.40 to 0.41) | 8.38 (8.34 to 8.43) | 95.2x |
| No-change update | user CPU s | 0.27 (0.27 to 0.27) | 0.49 (0.49 to 0.50) | 6.26 (6.24 to 6.29) | 23.1x |
| No-change update | system CPU s | 0.06 (0.06 to 0.06) | 0.07 (0.06 to 0.07) | 2.21 (2.19 to 2.24) | 39.5x |
| No-change update | peak RSS MB | 71 (69 to 71) | 188 (183 to 204) | 797 (794 to 800) | 11.3x |

| Output | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 |
|---|---|---|---|
| files that own a node | 882 | 882 | 885 |
| nodes | 16,774 | 16,774 | 18,601 |
| edges | 33,316 | 33,316 | 38,496 |
| communities | 309 | 309 | 1,022 |
| graph.json MB | 15.8 | 15.8 | 22.0 |
| output directory MB | 30.9 | 36.5 | 49.8 |

Runs: 18. Highest CPU use by other processes during a run: 146% of one core.

### redis/redis @ 940a4d7

Files read by default: graphify-go 1,458, upstream 1,460. Benchmarked on the 1,457 files both read (14 MB).

| Phase | Metric | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 | upstream / no-cgo |
|---|---|---|---|---|---|
| Cold build | wall s | 0.16 (0.13 to 0.17) | 0.54 (0.52 to 0.56) | 9.47 (9.39 to 9.56) | 59.5x |
| Cold build | user CPU s | 0.72 (0.72 to 0.75) | 3.14 (2.94 to 3.19) | 16.7 (16.4 to 16.8) | 23.2x |
| Cold build | system CPU s | 0.18 (0.17 to 0.19) | 0.26 (0.24 to 0.30) | 3.09 (3.05 to 3.11) | 17.1x |
| Cold build | peak RSS MB | 112 (111 to 115) | 220 (220 to 224) | 1,220 (1,193 to 1,230) | 10.9x |
| No-change update | wall s | 0.08 (0.08 to 0.08) | 0.33 (0.32 to 0.38) | 5.94 (5.87 to 6.06) | 77.1x |
| No-change update | user CPU s | 0.22 (0.22 to 0.22) | 0.40 (0.40 to 0.41) | 5.37 (5.32 to 5.50) | 24.2x |
| No-change update | system CPU s | 0.06 (0.06 to 0.07) | 0.07 (0.06 to 0.11) | 0.69 (0.69 to 0.70) | 11.9x |
| No-change update | peak RSS MB | 62 (61 to 62) | 140 (139 to 142) | 584 (582 to 588) | 9.5x |

| Output | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 |
|---|---|---|---|
| files that own a node | 992 | 992 | 992 |
| nodes | 11,678 | 11,678 | 17,212 |
| edges | 37,409 | 37,409 | 61,076 |
| communities | 314 | 314 | 493 |
| graph.json MB | 12.7 | 12.7 | 20.1 |
| output directory MB | 23.0 | 28.2 | 52.7 |

Runs: 18. Highest CPU use by other processes during a run: 221% of one core.

### curl/curl @ 2874cc6

Files read by default: graphify-go 2,068, upstream 2,086. Benchmarked on the 2,068 files both read (12 MB).

| Phase | Metric | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 | upstream / no-cgo |
|---|---|---|---|---|---|
| Cold build | wall s | 0.12 (0.12 to 0.14) | 0.38 (0.37 to 0.39) | 7.75 (7.71 to 8.10) | 66.3x |
| Cold build | user CPU s | 0.54 (0.52 to 0.56) | 2.14 (2.12 to 2.14) | 14.1 (14.0 to 14.5) | 26.1x |
| Cold build | system CPU s | 0.20 (0.20 to 0.20) | 0.31 (0.29 to 0.39) | 3.87 (3.79 to 3.90) | 19.0x |
| Cold build | peak RSS MB | 92 (92 to 96) | 178 (169 to 183) | 1,047 (1,046 to 1,050) | 11.3x |
| No-change update | wall s | 0.07 (0.07 to 0.08) | 0.25 (0.25 to 0.26) | 5.13 (5.12 to 5.26) | 68.4x |
| No-change update | user CPU s | 0.20 (0.20 to 0.20) | 0.30 (0.29 to 0.30) | 4.59 (4.59 to 4.70) | 23.4x |
| No-change update | system CPU s | 0.06 (0.06 to 0.06) | 0.07 (0.07 to 0.07) | 0.73 (0.72 to 0.76) | 11.8x |
| No-change update | peak RSS MB | 54 (54 to 58) | 124 (120 to 131) | 450 (449 to 459) | 8.3x |

| Output | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 |
|---|---|---|---|
| files that own a node | 2,065 | 2,065 | 2,068 |
| nodes | 13,955 | 13,955 | 18,247 |
| edges | 24,345 | 24,345 | 42,203 |
| communities | 1,006 | 1,006 | 1,243 |
| graph.json MB | 10.4 | 10.4 | 15.7 |
| output directory MB | 19.1 | 21.2 | 42.7 |

Runs: 18. Highest CPU use by other processes during a run: 198% of one core.

### opencv/opencv @ 73a26a4

Files read by default: graphify-go 5,003, upstream 4,783. Benchmarked on the 4,485 files both read (70 MB), plus 3 package manifest files.

| Phase | Metric | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 | upstream / no-cgo |
|---|---|---|---|---|---|
| Cold build | wall s | 0.47 (0.47 to 0.48) | 2.37 (2.32 to 2.42) | 61.3 (61.2 to 61.6) | 130.7x |
| Cold build | user CPU s | 4.04 (3.94 to 4.05) | 18.9 (17.4 to 19.0) | 95.9 (95.4 to 96.4) | 23.8x |
| Cold build | system CPU s | 0.48 (0.48 to 0.50) | 0.72 (0.72 to 0.73) | 7.91 (7.80 to 7.98) | 16.4x |
| Cold build | peak RSS MB | 225 (224 to 226) | 627 (618 to 721) | 1,889 (1,881 to 1,889) | 8.4x |
| No-change update | wall s | 0.18 (0.18 to 0.20) | 1.15 (1.13 to 1.16) | 80.0 (79.9 to 80.3) | 444.3x |
| No-change update | user CPU s | 0.73 (0.70 to 0.75) | 1.42 (1.34 to 1.43) | 78.3 (78.2 to 78.7) | 107.7x |
| No-change update | system CPU s | 0.16 (0.15 to 0.20) | 0.17 (0.17 to 0.18) | 3.46 (3.45 to 3.50) | 21.7x |
| No-change update | peak RSS MB | 120 (120 to 122) | 516 (504 to 628) | 2,645 (2,641 to 2,646) | 21.9x |

| Output | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 |
|---|---|---|---|
| files that own a node | 4,447 | 4,447 | 4,447 |
| nodes | 34,691 | 34,691 | 99,262 |
| edges | 61,293 | 61,293 | 222,456 |
| communities | 3,641 | 3,641 | 2,033 |
| graph.json MB | 28.6 | 28.6 | 100.0 |
| output directory MB | 69.9 | 97.5 | 298.2 |

Runs: 18. Highest CPU use by other processes during a run: 165% of one core.

### microsoft/TypeScript @ c0ca4744

Files read by default: graphify-go 31,924, upstream 31,973. Benchmarked on the 31,924 files both read (90 MB), plus 3 package manifest files.

| Phase | Metric | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 | upstream / no-cgo |
|---|---|---|---|---|---|
| Cold build | wall s | 1.42 (1.37 to 1.55) | 6.10 (6.03 to 6.20) | 182.4 (181.2 to 182.4) | 128.2x |
| Cold build | user CPU s | 7.47 (7.29 to 7.53) | 37.3 (36.6 to 37.6) | 237.0 (234.4 to 237.1) | 31.7x |
| Cold build | system CPU s | 2.48 (2.47 to 2.51) | 3.04 (3.02 to 3.07) | 32.8 (32.1 to 32.8) | 13.2x |
| Cold build | peak RSS MB | 324 (316 to 329) | 1,578 (1,408 to 1,651) | 2,548 (2,543 to 2,552) | 7.9x |
| No-change update | wall s | 0.72 (0.71 to 0.73) | 3.62 (3.61 to 3.64) | 168.3 (168.0 to 168.5) | 234.4x |
| No-change update | user CPU s | 2.41 (2.41 to 2.46) | 4.42 (4.39 to 4.50) | 213.2 (213.1 to 213.2) | 88.5x |
| No-change update | system CPU s | 0.53 (0.53 to 0.54) | 0.85 (0.82 to 0.88) | 25.5 (25.5 to 25.8) | 48.4x |
| No-change update | peak RSS MB | 335 (329 to 336) | 1,485 (1,271 to 1,553) | 5,369 (5,355 to 5,372) | 16.0x |

| Output | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 |
|---|---|---|---|
| files that own a node | 31,747 | 31,747 | 31,988 |
| nodes | 120,367 | 120,367 | 196,431 |
| edges | 254,203 | 254,203 | 425,606 |
| communities | 21,045 | 21,045 | 28,647 |
| graph.json MB | 126.0 | 126.0 | 210.5 |
| output directory MB | 251.8 | 275.9 | 317.9 |

Runs: 18. Highest CPU use by other processes during a run: 162% of one core.

### microsoft/vscode @ 636f8117

Files read by default: graphify-go 16,376, upstream 16,514. Benchmarked on the 16,376 files both read (215 MB), plus 4 package manifest files.

| Phase | Metric | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 | upstream / no-cgo |
|---|---|---|---|---|---|
| Cold build | wall s | 1.65 (1.65 to 2.09) | 11.6 (11.3 to 12.0) | 284.1 (283.2 to 295.9) | 171.8x |
| Cold build | user CPU s | 11.9 (11.6 to 12.0) | 58.1 (56.4 to 60.0) | 442.4 (437.0 to 444.3) | 37.2x |
| Cold build | system CPU s | 1.75 (1.70 to 1.92) | 2.23 (2.20 to 2.62) | 30.8 (30.2 to 31.4) | 17.6x |
| Cold build | peak RSS MB | 554 (505 to 578) | 2,853 (2,761 to 3,040) | 6,902 (6,891 to 6,905) | 12.5x |
| No-change update | wall s | 0.95 (0.94 to 0.96) | 8.66 (8.43 to 8.85) | 345.8 (328.2 to 347.0) | 362.5x |
| No-change update | user CPU s | 4.40 (4.21 to 4.41) | 10.1 (10.0 to 10.3) | 493.9 (482.9 to 496.2) | 112.3x |
| No-change update | system CPU s | 0.61 (0.60 to 0.63) | 0.88 (0.84 to 1.09) | 30.6 (30.1 to 31.8) | 50.2x |
| No-change update | peak RSS MB | 520 (517 to 535) | 2,503 (2,337 to 2,679) | 14,299 (14,256 to 14,303) | 27.5x |

| Output | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 |
|---|---|---|---|
| files that own a node | 15,382 | 15,382 | 15,516 |
| nodes | 200,704 | 200,704 | 256,511 |
| edges | 448,108 | 448,108 | 993,942 |
| communities | 4,232 | 4,232 | 5,411 |
| graph.json MB | 227.6 | 227.6 | 467.4 |
| output directory MB | 446.7 | 538.6 | 508.2 |

Runs: 18. Highest CPU use by other processes during a run: 217% of one core.

### kubernetes/kubernetes @ ab4fb775

Files read by default: graphify-go 15,354, upstream 20,008. Benchmarked on the 15,344 files both read (192 MB), plus 40 package manifest files.

| Phase | Metric | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 | upstream / no-cgo |
|---|---|---|---|---|---|
| Cold build | wall s | 1.60 (1.54 to 1.79) | 13.7 (13.7 to 14.1) | 407.1 (403.9 to 413.0) | 254.1x |
| Cold build | user CPU s | 10.3 (10.2 to 10.3) | 51.6 (51.0 to 51.6) | 430.5 (428.3 to 437.6) | 41.6x |
| Cold build | system CPU s | 1.65 (1.62 to 1.68) | 2.14 (2.11 to 2.19) | 37.6 (37.0 to 38.4) | 22.8x |
| Cold build | peak RSS MB | 509 (500 to 523) | 3,130 (3,107 to 3,176) | 4,442 (4,426 to 4,450) | 8.7x |
| No-change update | wall s | 1.03 (0.98 to 1.03) | 11.4 (11.4 to 11.6) | 309.5 (305.5 to 310.7) | 299.3x |
| No-change update | user CPU s | 4.38 (4.35 to 4.42) | 13.1 (13.0 to 13.1) | 288.6 (285.1 to 289.5) | 65.9x |
| No-change update | system CPU s | 0.59 (0.58 to 0.65) | 0.89 (0.87 to 0.91) | 22.1 (21.9 to 22.6) | 37.4x |
| No-change update | peak RSS MB | 529 (524 to 534) | 3,140 (3,014 to 3,528) | 8,348 (8,344 to 8,352) | 15.8x |

| Output | graphify-go f442d3f (no cgo) | graphify-go v0.10.1 (cgo) | upstream 0.9.88 |
|---|---|---|---|
| files that own a node | 14,164 | 14,164 | 14,165 |
| nodes | 152,610 | 152,610 | 183,215 |
| edges | 833,321 | 833,321 | 1,239,065 |
| communities | 602 | 602 | 7,861 |
| graph.json MB | 344.5 | 344.5 | 482.8 |
| output directory MB | 540.6 | 649.9 | 943.4 |

Runs: 20. Highest CPU use by other processes during a run: 171% of one core.

Device note. kubernetes run 1 wrote to the SSD, runs 2 and 3 to a RAM disk. The no-cgo cold build took 1.79 s on the SSD and 1.60 and 1.54 s on the RAM disk; the other tools did not move (upstream cold 404 to 413 s on the SSD, 407 and 410 s on the RAM disk). All vscode runs wrote to the RAM disk, so the vscode no-cgo cold time is probably about 10% better than an SSD run would give.

Communities differ because the algorithms and the graphs differ. They are not a quality signal here.

## What explains the node and edge differences

For each corpus one agent matched the nodes of the two graphs by source file and label, grouped what did not match into classes, read the extractor code on both sides and gave each class a verdict. A second agent then recomputed the counts with its own script and tried to refute each verdict. The totals and the arithmetic were reproduced on all seven corpora. The verdict columns record both agents. They predate the owner decision of 2026-10-13 to match upstream in what it captures, so "documented skip" and "upstream artefact" now mean "work for epic graphify-go-vi5", except the five upstream resolution errors held in graphify-go-vi5.12.

The graphify and opencv analyses used graphs built before 3 manifest files were added to each corpus. That changes the totals by 33 and 14 nodes on the graphify-go side.

The largest findings:

- C and C++ code inside `#if`, `#ifdef` and `#ifndef`, include guards included, is not extracted (graphify-go-vi5.1). opencv loses 22,867 nodes, curl 2,838, redis 1,928. The v0.10.1 cgo build has the same counts, so the parser swap did not cause it.
- Node ids are built from the parent directory name and the file stem, so files such as `test.c` and `test.cpp`, or `X.ts` and a baseline `X.js` in another tree, share ids (graphify-go-vi5.2). One file's symbols replace the other's. microsoft/TypeScript loses 8,018 nodes and kubernetes 7,279, and call edges land in the wrong file.
- JS/TS: only top-level statements are visited (graphify-go-vi5.10). Nested functions, top-level variables that are not functions, and declarations inside `namespace` and `declare` blocks are missing: about 46,000 nodes on microsoft/TypeScript.
- TS: a relative import written as `./foo.js` does not resolve to `foo.ts` (graphify-go-vi5.14). vscode loses 117,984 import edges, and about 26,000 call edges that depend on them.
- graphify-go has wrong edges of its own. On kubernetes at least 17,075 of its 38,775 call edges with no upstream twin are false, for example `time.Now()` bound to an in-corpus function named `Now` (graphify-go-vi5.11).
- Upstream has wrong output too. On kubernetes 208,497 of its edges are imports fanned out onto every `_test.go` file of the imported package, and it emits 7,875 duplicate receiver-type nodes.

### Graphify-Labs/graphify

| Difference | More on | Nodes | Edges | First verdict | Second agent |
|---|---|---|---|---|---|
| Python nested functions (closures, inner defs) as nodes | upstream | 879 | 1,661 | gap | confirmed |
| Source-less per-file type-reference stubs | upstream | 381 | 1,751 | upstream artefact | confirmed |
| Symbol-level import edges (file -> imported symbol) and re-exports | upstream | 0 | 1,611 | gap | confirmed |
| JSON keys of a Windows-path manifest collapsed by Go | upstream | 497 | 497 | upstream artefact | disputed: real gap |
| Go call edges to external module concept nodes | graphify-go | 0 | 832 | graphify-go extra | confirmed |
| Go bare-name INFERRED calls with no upstream twin (mostly false positives) | graphify-go | 0 | 464 | graphify-go extra | confirmed |
| Calls through a module alias or other receiver that Go drops | upstream | 0 | 408 | gap | disputed: real gap |
| Go node-id collisions across same-stem files in one directory | upstream | 61 | 122 | gap | confirmed |
| indirect_call edges (function passed as a value) | upstream | 0 | 148 | gap | confirmed |
| Markdown mention edges: heading-level INFERRED (upstream) vs file-level EXTRACTED (Go) | upstream | 0 | 128 | unclear | disputed: intended difference |
| Type-annotation and decorator references to in-corpus symbols | upstream | 0 | 127 | documented skip | disputed: real gap |
| Member-level node kinds: fields, enum members, constants, namespaces, interface methods, doctest cases, ObjC | upstream | 50 | 16 | documented skip | confirmed |
| 'uses' INFERRED edges | upstream | 0 | 61 | unclear | disputed: real gap |
| External import concept nodes | graphify-go | 11 | 24 | graphify-go extra | confirmed |

Not explained: 0 nodes, 0 edges.

### redis/redis

| Difference | More on | Nodes | Edges | First verdict | Second agent |
|---|---|---|---|---|---|
| Unresolved type-reference stub nodes (no source_file) and the references edges that land on them | upstream | 2,383 | 11,660 | upstream artefact | confirmed |
| C/C++ definitions and includes not at translation-unit root (inside #if/#ifdef/#ifndef include guards, or displaced by parser recovery) | upstream | 1,928 | 9,390 | gap | confirmed |
| C struct data fields as nodes with 'defines' edges | upstream | 1,420 | 1,483 | documented skip | confirmed |
| Type-reference edges (parameter_type / return_type / field) that reach a real node | upstream | 0 | 1,806 | documented skip | confirmed |
| Struct/union/enum types declared in .c files | graphify-go | 252 | 320 | graphify-go extra | confirmed |
| Call-resolution differences between nodes both tools have | graphify-go | 0 | 329 | graphify-go extra | disputed: unknown |
| Function-pointer struct fields recorded as methods, and name-based calls resolved to them | graphify-go | 38 | 257 | graphify-go extra | confirmed |
| Bash '<file> script' entry nodes and exported-variable nodes | upstream | 108 | 157 | upstream artefact | confirmed |
| Enum members with case_of edges | upstream | 40 | 40 | documented skip | confirmed |
| Small per-language leftovers (Python nested functions, Lua globals, markdown title labels, md-to-code references) | upstream | 11 | 62 | unclear | confirmed |
| External import concept nodes: go keeps the full header path, upstream collapses to the stem | graphify-go | 66 | 0 | graphify-go extra | disputed: real gap |
| Import edges after removing the #if-guarded includes (target naming differs: upstream 'stdio', go 'stdio.h') | upstream | 0 | 45 | upstream artefact | disputed: real gap |
| Go keeps two edges on one node pair | graphify-go | 0 | 42 | documented skip | confirmed |

Not explained: 0 nodes, 0 edges.

### curl/curl

| Difference | More on | Nodes | Edges | First verdict | Second agent |
|---|---|---|---|---|---|
| C functions nested under #if / ERROR nodes are dropped by Go | upstream | 2,838 | 10,477 | gap | confirmed |
| Upstream type-reference stubs with no source_file | upstream | 1,465 | 4,612 | upstream artefact | confirmed |
| #include edges dropped (guarded) or left as external concepts | upstream | 0 | 2,062 | gap | disputed: real gap |
| Python type, property and class-use edges | upstream | 0 | 1,285 | documented skip | disputed: real gap |
| Calls Go resolves that upstream does not | graphify-go | 0 | 778 | graphify-go extra | disputed: go extra |
| C struct/union/enum type nodes in .c files | graphify-go | 214 | 214 | graphify-go extra | confirmed |
| Struct fields, enum members and types in .h (upstream routes C++-looking headers to the C++ extractor) | upstream | 125 | 233 | documented skip | disputed: unknown |
| Bash entrypoint and variable nodes | upstream | 53 | 92 | gap | confirmed |
| Python nested functions | upstream | 21 | 59 | gap | confirmed |
| Markdown backtick references attach at different granularity | upstream | 0 | 42 | documented skip | disputed: real gap |
| Markdown node-id collisions on both sides | graphify-go | 0 | 6 | unclear | confirmed |
| External import concept nodes (label scheme and coverage differ) | upstream | 4 | 0 | unclear | confirmed |
| Python method label mismatch (ExecResult.json) | upstream | 0 | 0 | upstream artefact | disputed: unknown |

Not explained: 0 nodes, 0 edges.

### opencv/opencv

| Difference | More on | Nodes | Edges | First verdict | Second agent |
|---|---|---|---|---|---|
| C/C++ code inside preprocessor conditionals (include guards, #if/#ifdef) is not extracted by graphify-go | upstream | 22,857 | 65,237 | gap | disputed: real gap for the nodes and about 48k edges; the 17,253 references edges in this class are  |
| Sourceless type-reference stub nodes and the references edges onto them | upstream | 15,954 | 52,629 | upstream artefact | confirmed |
| Struct/class fields as nodes (defines edges) | upstream | 14,672 | 17,006 | documented skip | confirmed |
| C++ in-class method declarations without a body | upstream | 5,173 | 11,119 | gap | confirmed |
| Enum members (case_of edges) | upstream | 3,818 | 4,128 | documented skip | confirmed |
| Containment of out-of-line methods: upstream file->method edges, Go leaves the method orphaned | upstream | 0 | 7,342 | gap | disputed: real gap for about 4,440 edges; about 2,800 are a representation difference offset by Go's |
| Go-side import stubs and unresolved include edges (counterpart of the class above) | graphify-go | 550 | 6,751 | graphify-go extra | confirmed |
| Calls between nodes both tools have: Go-only pairs | graphify-go | 0 | 5,879 | graphify-go extra | confirmed |
| Include/import resolution: upstream resolves to a real file or a differently named stub | upstream | 543 | 5,115 | gap | confirmed |
| Calls between nodes both tools have: upstream-only pairs | upstream | 0 | 4,467 | gap | disputed: mixed: about 2,860 EXTRACTED pairs are real gap, about 1,600 INFERRED cross-file pairs are |
| Non-C languages: nested functions, anonymous classes, shell entry nodes | upstream | 1,024 | 3,043 | gap | disputed: unknown: mostly undocumented design differences (anonymous classes, nested functions), plu |
| Type-reference edges between nodes both tools have | upstream | 0 | 2,664 | documented skip | confirmed |
| Go-only class->method contains, inherits and markdown references between matched nodes | graphify-go | 0 | 1,861 | graphify-go extra | confirmed |
| Go-only C/C++ nodes (label mismatches and function-pointer fields) | graphify-go | 588 | 486 | unclear | confirmed |
| Members of nested or local C++ types Go does not emit (outside conditionals) | upstream | 302 | 647 | gap | confirmed |
| Go-only nodes in non-C languages | graphify-go | 18 | 158 | graphify-go extra | confirmed |

Not explained: 1,394 nodes, 2,954 edges.

### microsoft/TypeScript

| Difference | More on | Nodes | Edges | First verdict | Second agent |
|---|---|---|---|---|---|
| JS/TS nested functions and object/type-literal methods | upstream | 10,649 | 38,291 | gap | confirmed |
| JS/TS top-level variables and consts that are not functions | upstream | 22,522 | 21,737 | gap | confirmed |
| Upstream stubs for unresolved imported symbols, re-exports and type references | upstream | 5,613 | 24,042 | upstream artefact | confirmed |
| JS/TS declarations inside namespace/module blocks and ambient `declare` | upstream | 12,740 | 12,686 | gap | confirmed |
| Type-reference and struct-embedding edges between real nodes | upstream | 0 | 25,314 | documented skip | confirmed |
| JS/TS interface method signatures | upstream | 10,170 | 11,071 | gap | disputed: intended difference |
| Enum members with case_of edges | upstream | 6,463 | 6,927 | documented skip | confirmed |
| graphify-go cross-file node ID collision (dir.stem ids) | upstream | 8,018 | 5,276 | gap | confirmed |
| Go-language imports_from fan-out to _test.go files | upstream | 0 | 12,747 | upstream artefact | confirmed |
| JS/TS symbol-level import edges (file to imported symbol) | upstream | 0 | 9,405 | gap | confirmed |
| JS/TS class inherits and implements edges | upstream | 0 | 6,463 | gap | confirmed |
| Go-language interface method specs and per-file receiver stubs | upstream | 803 | 3,709 | gap | disputed: intended difference |
| Calls resolver disagreement between matched nodes | graphify-go | 0 | 3,024 | unclear | confirmed |
| graphify-go function-valued `var` declarations | graphify-go | 979 | 1,150 | graphify-go extra | confirmed |
| Go-language method ownership: type-to-method versus file-to-method | graphify-go | 0 | 1,635 | gap | confirmed |
| Class member forms graphify-go skips inside classes it does emit | upstream | 604 | 684 | gap | confirmed |
| graphify-go JSON and go.mod structure | graphify-go | 455 | 621 | graphify-go extra | confirmed |
| Unresolved module import stubs (equivalent on both sides) | graphify-go | 263 | 404 | documented skip | confirmed |
| Upstream file-node id collision with a `_` symbol (matching artefact) | graphify-go | 12 | 24 | upstream artefact | confirmed |

Not explained: 191 nodes, 1,079 edges.

### kubernetes/kubernetes

| Difference | More on | Nodes | Edges | First verdict | Second agent |
|---|---|---|---|---|---|
| imports_from fan-out onto _test.go files of the imported package | upstream | 0 | 208,497 | upstream artefact | confirmed |
| `references` edges from signatures and fields to in-corpus types | upstream | 0 | 104,525 | documented skip | confirmed |
| Sourceless type stubs and the references/embeds that point at them | upstream | 7,860 | 60,108 | upstream artefact | disputed: mixed: intended difference for 745 nodes / 47,787 edges (external qualified types), upstre |
| type -> method `method` edges (upstream) versus file -> method `contains` (Go) | upstream | 0 | 57,643 | gap | confirmed |
| file -> method `contains` edges | graphify-go | 0 | 55,684 | graphify-go extra | confirmed |
| Go-only call edges, cross-file | graphify-go | 0 | 38,777 | unclear | disputed: mixed: at least 17,075 are Go noise (collision artefacts and phantoms), about 21,700 are r |
| Declarations Go drops through node-id collisions | upstream | 7,279 | 29,206 | gap | confirmed |
| Go interface method requirement nodes | upstream | 8,519 | 20,115 | documented skip | confirmed |
| Import-qualified cross-file calls through an aliased package name | upstream | 0 | 18,690 | gap | confirmed |
| Same-file member calls bound by bare name to the last same-named method | upstream | 0 | 14,535 | upstream artefact | confirmed |
| Go-only call edges, same file | graphify-go | 0 | 10,603 | graphify-go extra | confirmed |
| Duplicate receiver-type nodes at method sites | upstream | 7,875 | 0 | upstream artefact | confirmed |
| Struct and interface embedding (`embeds`) onto in-corpus types | upstream | 0 | 4,665 | gap | confirmed |
| Other upstream-only calls between nodes both tools have | upstream | 0 | 4,604 | unclear | confirmed |
| Methods and types upstream loses to case-folded owner ids | graphify-go | 998 | 3,274 | graphify-go extra | confirmed |
| go.mod `require` package nodes and depends_on edges | graphify-go | 384 | 1,901 | graphify-go extra | confirmed |
| Bash: script-entry nodes, variable nodes, resolved `source` | upstream | 525 | 931 | gap | disputed: mixed: real gap for 235 variable nodes / 235 defines / 174 resolved source edges; represen |
| Markdown references and matching artefacts | graphify-go | 1 | 1,248 | graphify-go extra | confirmed |
| Bash `source` targets left as unresolved external concepts | graphify-go | 77 | 224 | graphify-go extra | confirmed |

Not explained: 6 nodes, 133 edges.

### microsoft/vscode

| Difference | More on | Nodes | Edges | First verdict | Second agent |
|---|---|---|---|---|---|
| Named-import symbol edges (file -> imported symbol, relation 'imports') | upstream | 0 | 185,371 | gap | confirmed |
| Go unresolved relative-import stub nodes (the Go-side shadow of the '.js' gap) | graphify-go | 14,866 | 117,828 | gap | confirmed |
| Relative ESM '.js' import specifiers not resolved to .ts files (imports_from, re_exports, dynamic_import) | upstream | 0 | 117,984 | gap | disputed: real gap |
| Type-reference edges (references: parameter_type, field, generic_arg, return_type, decorator) | upstream | 0 | 102,211 | documented skip | confirmed |
| Call edges missing between nodes both tools have (calls + indirect_call) | upstream | 0 | 71,836 | gap | disputed: unknown |
| Interface member signatures as nodes (method_signature in interfaces and type literals) | upstream | 12,980 | 53,354 | gap | confirmed |
| Declarations nested in function/callback bodies, object literals and class property initialisers | upstream | 13,407 | 40,118 | gap | confirmed |
| Module-level const/let/var nodes (non-function initialisers) | upstream | 11,000 | 37,922 | gap | confirmed |
| Upstream unresolved stubs: per-file type-reference stubs, external import concepts, bare-module nodes | upstream | 9,007 | 29,167 | upstream artefact | confirmed |
| Enum member nodes with case_of edges | upstream | 13,508 | 11,986 | documented skip | confirmed |
| Go-only call edges between nodes both tools have | graphify-go | 0 | 22,248 | unclear | disputed: unknown |
| Go node-id collisions merge same-named symbols from different files | upstream | 3,378 | 17,426 | gap | confirmed |
| namespace / 'declare module' bodies and their container nodes | upstream | 7,300 | 11,875 | gap | confirmed |
| inherits / implements edges for TS classes and interfaces | upstream | 0 | 14,627 | documented skip | disputed: real gap |
| Go manifest and JSON config extras | graphify-go | 661 | 7,834 | graphify-go extra | disputed: go extra |
| Go bare external import stubs | graphify-go | 597 | 5,089 | graphify-go extra | confirmed |
| Go miscellaneous extras | graphify-go | 125 | 3,048 | graphify-go extra | confirmed |
| Upstream extras outside TS/JS: Rust, shell, C++, C#, JSON keys, RFC concept nodes | upstream | 1,264 | 1,760 | documented skip | not checked |
| Markdown backtick references to code resolved differently | upstream | 0 | 2,813 | unclear | confirmed |
| Go cross-file contains edges produced by the id collisions | graphify-go | 0 | 1,853 | gap | confirmed |

Not explained: 212 nodes, 5,770 edges.

## Refutation of the top-line numbers

Three agents tried to refute the result. One measured again with `/usr/bin/time -l` and no part of the harness; two audited the harness and the fairness of the comparison without running the tools on a corpus.

What held:

- All 42 medians were recomputed from `results.jsonl` and match. No run failed. Node and edge counts are the same in all 3 runs of graphify-go and the same between the cgo and no-cgo builds.
- redis and opencv, measured again: every wall time within 15% of the harness median except one of two opencv no-cgo cold runs (0.58 s against 0.47 s; the other was 0.46 s).
- Cold runs were cold. `nice` and the `graph.html` switch do not move the result. No cache outside the output directory was found for either tool.
- CPU accounting covers upstream's worker processes.

What changed the report:

- A first draft of the ranges did not match the tables (360x for 444x wall, 115x for 105x CPU, 5x for 8x RSS). The ranges in this document are the recomputed ones.
- The no-change finding and the clustering and single-core probes in the limits above came from this pass.

Probes on opencv (one or two runs each, `/usr/bin/time -l`, wall seconds):

| Configuration | graphify-go no-cgo | upstream | Ratio |
|---|---|---|---|
| Cold build, as benchmarked | 0.46 | 60.9 | 131x |
| Cold build, clustering off on both (`--no-cluster`) | 0.45 | 21.3 | 47x |
| Cold build, one core (`GOMAXPROCS=1`, `GRAPHIFY_MAX_WORKERS=1`) | 3.35 | 96.8 | 29x |
| Cold build, upstream with `graph.html` on | n/a | 62.1 | n/a |
| No-change update, as benchmarked | 0.17 | 79.5 | 444x |
| No-change update, clustering off on both | 0.17 | 32.5 | 191x |

On opencv, repeated upstream updates alternate between two graphs (99,262 nodes and 2,033 communities, then 99,261 and 2,010) and never settle.

## Not measured

- Upstream with the `leiden` extra (graspologic-native). The `--no-cluster` probe bounds what it could gain. On the corpora where upstream's no-change run did no clustering, dividing that run by graphify-go's cold build still gives 37x to 193x.
- The split of upstream's time between parsing, clustering, the betweenness pass and writing, beyond the opencv probes.
- `graphify extract --code-only` at scale, as an alternative no-change path for upstream.
- A quiet machine, a Linux or x86-64 machine, and default-against-default runs on the original repositories.
- graphify-go with the gaps closed. The node and edge counts will rise, and so will its times.

## Files

`docs/benchmark-upstream-v0.9.88/` holds the harness as it ran (`prep.py`, `bench.py`, `run1.py`, `report.py`, `graphstats.py`), the raw records (`results.jsonl`, 128 runs), the file counts (`files.json`) and the output sizes recorded before the output directories were deleted (`sizes.json`). The scripts expect to sit in a scratch directory next to `bin/gfy-nocgo`, `venv/` and the corpora, and `bench.py` has the cgo binary path of this machine in it. The tables in this document are the output of `report.py`.
