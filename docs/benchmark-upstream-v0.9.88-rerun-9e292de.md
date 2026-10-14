# Benchmark re-run: graphify-go 9e292de against upstream graphify 0.9.88

Bead: graphify-go-ccu. Measured 2026-10-13. Measure only: no product code changed. This re-runs the benchmark in `docs/benchmark-upstream-v0.9.88.md` (graphify-go f442d3f) after the first part of epic graphify-go-vi5. At 9e292de, vi5.1 and vi5.2 are closed and vi5.10 is in progress; 15 of 17 vi5 children are still open, so this is an intermediate state and the benchmark has to run again when the epic closes.

## Result

graphify-go 9e292de writes more of the graph than f442d3f on all seven corpora: between 59% (opencv) and 95% (curl) of upstream's nodes, up from 35% to 90%. Its node and edge counts are identical in every run. That extra output costs time, CPU, memory and disk, and on microsoft/vscode it roughly doubles the cold build.

| Corpus | Nodes, f442d3f to 9e292de | Edges, f442d3f to 9e292de | Share of upstream nodes |
|---|---|---|---|
| Graphify-Labs/graphify | 16,774 to 16,835 | 33,316 to 33,379 | 91% |
| redis/redis | 11,678 to 14,130 | 37,409 to 47,095 | 82% |
| curl/curl | 13,955 to 17,394 | 24,345 to 37,309 | 95% |
| opencv/opencv | 34,691 to 58,593 | 61,293 to 121,769 | 59% |
| microsoft/TypeScript | 120,367 to 174,482 | 254,203 to 343,723 | 89% |
| kubernetes/kubernetes | 152,610 to 159,418 | 833,321 to 841,668 | 87% |
| microsoft/vscode | 200,704 to 219,294 | 448,108 to 787,785 | 85% |

Largest regressions against f442d3f measured in the same session. The second column is the refutation agent's own interleaved measurement with `/usr/bin/time -l`, outside the harness, on a quieter machine.

| Cell | Harness | Refutation agent |
|---|---|---|
| vscode cold build, wall | +122% (1.59 to 3.54 s) | +105% (1.68 to 3.44 s) |
| vscode cold build, user CPU | +126% (12.1 to 27.4 s) | +123% |
| vscode cold build, peak RSS | +44% (548 to 789 MB) | not measured |
| vscode no-change update, wall | +83% | +79% |
| opencv no-change update, wall / user CPU / peak RSS | +58% / +69% / +62% | +59% / +67% / +61% |
| TypeScript cold build, wall | +42% (1.35 to 1.92 s) | +29% (1.33 to 1.71 s) |
| kubernetes cold build, wall | +5% | +6.5% |

The TypeScript harness figure is inflated: two of its four cold runs ran while other processes used 186% and 282% of one core. Use +29% to +43%. redis no-change wall reads +71% in the harness, but that median rests on a run that waited 3,120 s at the idle gate between its cold build and its update; treat it as noise until measured again.

Output size grew on every corpus. The output directory of 9e292de is now larger than upstream's on two corpora: microsoft/TypeScript (372 MB against 318 MB) and microsoft/vscode (707 MB against 508 MB). graph.json alone is still smaller than upstream's everywhere.

Against upstream 0.9.88 (first-run numbers, not re-run), 9e292de is still faster and smaller in every timing and memory cell:

| Ratio, upstream / graphify-go 9e292de | Cold build | No-change update |
|---|---|---|
| Wall time | 50x to 239x | 46x to 276x |
| CPU time, user + system | 16x to 39x | 16x to 70x |
| Peak RSS (see the RSS caveat under Limits) | 6.2x to 10.8x | 6.7x to 19x |

The no-change ratios on opencv and vscode compare different work: upstream does not reach its "no topology change" skip there and re-clusters on every update.

## What the first run left out

Clustering is a small part of upstream's time. `--no-cluster` takes 30% to 65% off upstream's cold wall time, but in upstream that flag skips much more than clustering. Upstream with the `leiden` extra is only 7% to 11% faster than with its default Louvain fallback, measured in the same session.

`--no-cluster` does not skip the same work in the two tools:

| Tool | What `--no-cluster` skips |
|---|---|
| graphify-go 9e292de | `cluster.Cluster` only (`cmd/graphify/main.go:491`). The report is still written, with empty community scores. |
| upstream 0.9.88 | the networkx graph build, clustering, scoring, god nodes, suggested questions and the report (`graphify/watch.py:2219` returns before `build_from_json`). It writes the raw merged extraction, which differs slightly from the built graph (redis: 17,244 nodes and 61,180 edges against 17,212 and 61,076). |

A cProfile of upstream on redis with Leiden (20.4 s under the profiler) splits the run as: extraction 7.0 s, `suggest_questions` 4.6 s (almost all of it networkx `betweenness_centrality`), `to_json` 2.8 s, JSON write 2.3 s, `build_from_json` 1.9 s, clustering 1.9 s. The betweenness pass costs more than clustering.

graphify-go's own clustering is 3% to 32% of its cold wall time (median of 3 runs each, these are sub-second numbers on small corpora and noisy); on the large corpora it is 12% to 22%.

Leiden. `pip install 'graphifyy[leiden]'` on Python 3.14 adds `graspologic-native` 1.3.1 and `scipy`. An instrumented run on redis showed every clustering call going through `_native_leiden` and none through Louvain. Clustering itself took 1.19 s with Leiden against 1.72 s with Louvain, in a run of about 9 s. Nodes and edges are identical; communities change (redis 493 to 511, opencv 2,033 to 1,874, kubernetes 7,861 to 8,241). The refutation agent's same-session, interleaved measurement: redis Louvain 9.51 to 9.68 s against Leiden 8.74 to 9.00 s (-7%); opencv Louvain 62.6 and 63.8 s against Leiden 55.5 and 57.2 s (-9% to -11%). kubernetes, 3 interleaved cold builds each in one session (`results-dma.jsonl`, bead graphify-go-dma): Louvain 407.0 s (387.0 to 408.9), Leiden 375.6 s (367.0 to 392.6), -8%; `--no-cluster` 269.5 s (263.4 to 288.9), 34% less than Louvain. Leiden used 6% more peak RSS (4,715 MB against 4,443 MB). The Louvain runs agree with the first run (407.1 s), so the Python rebuild did not move kubernetes.

## Machine, versions and method

Same machine, harness and corpora as the first run (Apple M4 Pro, 14 cores, 64 GB, macOS 27.0.1). See the first report for the method; differences only:

- graphify-go 9e292de: `CGO_ENABLED=0 go build ./cmd/graphify`, go 1.27.1, commit 9e292de (origin/main on 2026-10-13). `go version -m` shows `vcs.modified=true`; the only modified files in that worktree were the generated `graphify-out/` files, which are not compiled. The f442d3f binary also shows `vcs.modified=true`, and what was modified there cannot be recovered.
- f442d3f was measured again in this session, interleaved with 9e292de and with 9e292de `--no-cluster` (rotating order, 3 rounds). The "against this session" column is the regression measure; "against first run" is shown for reference.
- Upstream full runs were not repeated for the main table; the upstream columns are the first-run numbers. Upstream ran again only with `--no-cluster` (all corpora, 3 runs, cold and no-change) and with the `leiden` extra (redis, opencv, kubernetes, 3 cold runs). The leiden environment was a second virtual environment, made just before its section and deleted after it (pip freezes in `venv.freeze.txt` and `venv-leiden.freeze.txt`).
- `prep.py` now adds the package manifests itself (`go.mod`, `pyproject.toml`, `pom.xml`, `Cargo.toml`, with the skip-directory and `.gitignore` rules of `CollectManifests` in `internal/detect/detect.go`). Run again on the seven source repositories, it reproduces the benchmarked corpora exactly: the same files and the same counts in `files.json`.
- `bench.py` takes `BENCH_RESULTS`, `BENCH_TOOLS` and `BENCH_PHASES` from the environment. New records are in `results-9e292de.jsonl`; `results.jsonl` (first run) is unchanged. `queue-9e292de.sh` ran the sections one after the other; `report-9e292de.py` prints the tables below.
- Every output directory was on the SSD. In the first run, all vscode runs and kubernetes runs 2 and 3 wrote to a RAM disk.

## Limits

These came out of the refutation pass.

1. The machine was busy. Up to nine other sessions ran Go builds and tests. The idle gate (at least 85% idle CPU before each run) held every run, but it also waits between a cold build and its no-change update; two such waits took 3,872 s and 3,120 s. Other processes still used up to 3.5 cores during some runs, and 9e292de ran under more background load than f442d3f on vscode (mean 115% against 73% of one core) and TypeScript (161% against 124%).
2. The sessions do not fully agree. f442d3f measured again agrees with its first-run numbers within about 6% on most cells, but not on Graphify-Labs/graphify cold build (wall +68%, system CPU +27%), TypeScript no-change system CPU (+17%) and kubernetes no-change system CPU (+11%). That is why the regression column uses the same-session baseline.
3. The disk filled during the run (other work on the machine). The harness stopped below 2.5 GB free and resumed later. That left one orphan cold record for 9e292de on TypeScript (4 cold runs instead of 3). The first run has the same problem on kubernetes upstream cold (5 records, run 2 three times).
4. Upstream's Python changed between sessions: Homebrew replaced 3.14.8 with the rebuild 3.14.8_2 at 17:54. Every upstream run of this session used 3.14.8_2; the first run used 3.14.8. The refutation agent's same-session Louvain runs on redis and opencv came out 1% to 4% slower than the first-run numbers, which is within session drift. The `--no-cluster` split subtracts numbers from the two sessions.
5. In the no-change split, upstream's full update did no clustering at all on graphify, redis, curl, TypeScript and kubernetes (it exits early: "No code-graph topology changes detected"). The "share" figures there compare two code paths, not clustering.
6. Peak RSS for upstream is the sum over its worker processes sampled every 0.1 s, which counts shared pages more than once; for graphify-go it is one process. The RSS ratios favour graphify-go for that reason (redis upstream cold: largest single process 294 MB, tree sum 1,193 MB).
7. "Highest CPU use by other processes" is the largest per-run mean of `ps` samples, not a peak, and for runs under one second it mostly shows the load just before the run.

## Not measured

- graphify-go with the rest of vi5 closed.
- A quiet machine, Linux, x86-64.

## Files

`docs/benchmark-upstream-v0.9.88/`: `prep.py` and `bench.py` (updated), `queue-9e292de.sh`, `report-9e292de.py`, `results-9e292de.jsonl` (178 records), `venv.freeze.txt`, `venv-leiden.freeze.txt`, `results-dma.jsonl` (kubernetes, same session, upstream Louvain, Leiden and `--no-cluster`, run with `queue-dma.sh`).

`run1.py` and `bench.py` now also guard the disk: a run stops when free space falls below a floor or swap grows past a limit, and a session stops after it has lost 32 GB of free space (`BENCH_DISK_BUDGET_GB`) or gained 8 GB of swap, whatever the cause. Each record stores the lowest free space and the highest swap seen during its run. `guardtest.sh` checks both cases. The kubernetes runs used under 1 GB of disk and grew swap by nothing. The scripts expect to sit in the scratch directory with `runs/`, `bin/`, `venv/` and the corpora, as before. The tables below are the output of `report-9e292de.py`.

## Results per corpus

Each timing cell is the median of 3 runs with the minimum and maximum in brackets (9e292de on TypeScript cold: 4 runs, see Limits). Upstream columns are first-run numbers.

### Graphify-Labs/graphify

| Phase | Metric | f442d3f, first run | f442d3f, this session | 9e292de | 9e292de against first run | against this session | upstream 0.9.88, first run | upstream / 9e292de |
|---|---|---|---|---|---|---|---|---|
| Cold build | wall s | 0.12 (0.12 to 0.12) | 0.20 (0.20 to 0.22) | 0.14 (0.14 to 0.23) | +14% | -32% | 12.0 (12.0 to 12.2) | 86.9x |
| Cold build | user CPU s | 0.67 (0.66 to 0.67) | 0.68 (0.67 to 0.68) | 0.68 (0.67 to 0.69) | +1% | +1% | 18.4 (18.3 to 18.5) | 27.0x |
| Cold build | system CPU s | 0.14 (0.14 to 0.14) | 0.17 (0.17 to 0.18) | 0.14 (0.14 to 0.18) | +1% | -20% | 3.87 (3.86 to 3.92) | 27.6x |
| Cold build | peak RSS MB | 133 (128 to 137) | 128 (127 to 131) | 133 (128 to 136) | +0% | +4% | 1,166 (1,165 to 1,168) | 8.8x |
| No-change update | wall s | 0.09 (0.09 to 0.09) | 0.09 (0.09 to 0.10) | 0.11 (0.10 to 0.14) | +22% | +19% | 8.38 (8.34 to 8.43) | 78.3x |
| No-change update | user CPU s | 0.27 (0.27 to 0.27) | 0.28 (0.27 to 0.28) | 0.28 (0.28 to 0.28) | +2% | -1% | 6.26 (6.24 to 6.29) | 22.7x |
| No-change update | system CPU s | 0.06 (0.06 to 0.06) | 0.06 (0.06 to 0.06) | 0.07 (0.06 to 0.08) | +16% | +7% | 2.21 (2.19 to 2.24) | 34.0x |
| No-change update | peak RSS MB | 71 (69 to 71) | 70 (70 to 73) | 74 (72 to 75) | +5% | +5% | 797 (794 to 800) | 10.8x |

| Output | f442d3f, first run | f442d3f, this session | 9e292de | change | upstream 0.9.88 | 9e292de / upstream |
|---|---|---|---|---|---|---|
| files that own a node | 882 | 882 | 882 | +0% | 885 | 100% |
| nodes | 16,774 | 16,774 | 16,835 | +0% | 18,601 | 91% |
| edges | 33,316 | 33,316 | 33,379 | +0% | 38,496 | 87% |
| communities | 309 | 309 | 312 | +1% | 1,022 | 31% |
| graph.json MB | 15.8 | 15.8 | 16.1 | +2% | 22.0 | 73% |
| output directory MB | 30.9 | 30.9 | 31.5 | +2% | 49.8 | 63% |

Second-run records: 24. Highest CPU use by other processes during a run: 200% of one core. Longest wait for idle CPUs: 3872 s. Runs that started without idle CPUs: 0.

### redis/redis

| Phase | Metric | f442d3f, first run | f442d3f, this session | 9e292de | 9e292de against first run | against this session | upstream 0.9.88, first run | upstream / 9e292de |
|---|---|---|---|---|---|---|---|---|
| Cold build | wall s | 0.16 (0.13 to 0.17) | 0.14 (0.14 to 0.17) | 0.19 (0.14 to 0.20) | +20% | +33% | 9.47 (9.39 to 9.56) | 49.6x |
| Cold build | user CPU s | 0.72 (0.72 to 0.75) | 0.74 (0.69 to 0.75) | 0.77 (0.75 to 0.78) | +6% | +4% | 16.7 (16.4 to 16.8) | 21.9x |
| Cold build | system CPU s | 0.18 (0.17 to 0.19) | 0.18 (0.17 to 0.18) | 0.18 (0.17 to 0.20) | +1% | +3% | 3.09 (3.05 to 3.11) | 17.0x |
| Cold build | peak RSS MB | 112 (111 to 115) | 118 (115 to 119) | 116 (111 to 120) | +3% | -2% | 1,220 (1,193 to 1,230) | 10.5x |
| No-change update | wall s | 0.08 (0.08 to 0.08) | 0.08 (0.08 to 0.12) | 0.13 (0.09 to 0.18) | +69% | +71% | 5.94 (5.87 to 6.06) | 45.7x |
| No-change update | user CPU s | 0.22 (0.22 to 0.22) | 0.22 (0.22 to 0.23) | 0.27 (0.27 to 0.27) | +21% | +23% | 5.37 (5.32 to 5.50) | 20.1x |
| No-change update | system CPU s | 0.06 (0.06 to 0.07) | 0.06 (0.06 to 0.07) | 0.08 (0.06 to 0.09) | +40% | +37% | 0.69 (0.69 to 0.70) | 8.6x |
| No-change update | peak RSS MB | 62 (61 to 62) | 62 (61 to 63) | 71 (69 to 75) | +15% | +14% | 584 (582 to 588) | 8.2x |

| Output | f442d3f, first run | f442d3f, this session | 9e292de | change | upstream 0.9.88 | 9e292de / upstream |
|---|---|---|---|---|---|---|
| files that own a node | 992 | 992 | 992 | +0% | 992 | 100% |
| nodes | 11,678 | 11,678 | 14,130 | +21% | 17,212 | 82% |
| edges | 37,409 | 37,409 | 47,095 | +26% | 61,076 | 77% |
| communities | 314 | 314 | 219 | -30% | 493 | 44% |
| graph.json MB | 12.7 | 12.7 | 16.7 | +31% | 20.1 | 83% |
| output directory MB | 23.0 | 23.0 | 29.5 | +28% | 52.7 | 56% |

Second-run records: 27. Highest CPU use by other processes during a run: 162% of one core. Longest wait for idle CPUs: 3120 s. Runs that started without idle CPUs: 0.

### curl/curl

| Phase | Metric | f442d3f, first run | f442d3f, this session | 9e292de | 9e292de against first run | against this session | upstream 0.9.88, first run | upstream / 9e292de |
|---|---|---|---|---|---|---|---|---|
| Cold build | wall s | 0.12 (0.12 to 0.14) | 0.12 (0.12 to 0.18) | 0.13 (0.13 to 0.14) | +15% | +13% | 7.75 (7.71 to 8.10) | 57.9x |
| Cold build | user CPU s | 0.54 (0.52 to 0.56) | 0.56 (0.53 to 0.57) | 0.64 (0.63 to 0.64) | +18% | +13% | 14.1 (14.0 to 14.5) | 22.1x |
| Cold build | system CPU s | 0.20 (0.20 to 0.20) | 0.20 (0.19 to 0.21) | 0.20 (0.20 to 0.21) | +0% | +0% | 3.87 (3.79 to 3.90) | 19.0x |
| Cold build | peak RSS MB | 92 (92 to 96) | 90 (89 to 91) | 97 (93 to 99) | +5% | +8% | 1,047 (1,046 to 1,050) | 10.8x |
| No-change update | wall s | 0.07 (0.07 to 0.08) | 0.07 (0.07 to 0.08) | 0.09 (0.09 to 0.09) | +20% | +20% | 5.13 (5.12 to 5.26) | 57.0x |
| No-change update | user CPU s | 0.20 (0.20 to 0.20) | 0.20 (0.20 to 0.20) | 0.26 (0.26 to 0.27) | +35% | +30% | 4.59 (4.59 to 4.70) | 17.4x |
| No-change update | system CPU s | 0.06 (0.06 to 0.06) | 0.07 (0.06 to 0.07) | 0.07 (0.07 to 0.07) | +11% | +5% | 0.73 (0.72 to 0.76) | 10.6x |
| No-change update | peak RSS MB | 54 (54 to 58) | 54 (54 to 55) | 67 (66 to 70) | +24% | +26% | 450 (449 to 459) | 6.7x |

| Output | f442d3f, first run | f442d3f, this session | 9e292de | change | upstream 0.9.88 | 9e292de / upstream |
|---|---|---|---|---|---|---|
| files that own a node | 2,065 | 2,065 | 2,065 | +0% | 2,068 | 100% |
| nodes | 13,955 | 13,955 | 17,394 | +25% | 18,247 | 95% |
| edges | 24,345 | 24,345 | 37,309 | +53% | 42,203 | 88% |
| communities | 1,006 | 1,006 | 927 | -8% | 1,243 | 75% |
| graph.json MB | 10.4 | 10.4 | 14.8 | +42% | 15.7 | 94% |
| output directory MB | 19.1 | 19.1 | 26.7 | +40% | 42.7 | 63% |

Second-run records: 24. Highest CPU use by other processes during a run: 203% of one core. Longest wait for idle CPUs: 80 s. Runs that started without idle CPUs: 0.

### opencv/opencv

| Phase | Metric | f442d3f, first run | f442d3f, this session | 9e292de | 9e292de against first run | against this session | upstream 0.9.88, first run | upstream / 9e292de |
|---|---|---|---|---|---|---|---|---|
| Cold build | wall s | 0.47 (0.47 to 0.48) | 0.49 (0.48 to 0.65) | 0.57 (0.57 to 0.60) | +23% | +17% | 61.3 (61.2 to 61.6) | 106.6x |
| Cold build | user CPU s | 4.04 (3.94 to 4.05) | 4.01 (3.87 to 4.03) | 4.54 (4.52 to 4.61) | +12% | +13% | 95.9 (95.4 to 96.4) | 21.1x |
| Cold build | system CPU s | 0.48 (0.48 to 0.50) | 0.48 (0.48 to 0.56) | 0.50 (0.50 to 0.51) | +4% | +3% | 7.91 (7.80 to 7.98) | 15.8x |
| Cold build | peak RSS MB | 225 (224 to 226) | 221 (209 to 223) | 243 (239 to 246) | +8% | +10% | 1,889 (1,881 to 1,889) | 7.8x |
| No-change update | wall s | 0.18 (0.18 to 0.20) | 0.18 (0.18 to 0.19) | 0.29 (0.29 to 0.29) | +61% | +58% | 80.0 (79.9 to 80.3) | 275.7x |
| No-change update | user CPU s | 0.73 (0.70 to 0.75) | 0.72 (0.72 to 0.73) | 1.21 (1.19 to 1.22) | +66% | +69% | 78.3 (78.2 to 78.7) | 64.8x |
| No-change update | system CPU s | 0.16 (0.15 to 0.20) | 0.17 (0.17 to 0.17) | 0.19 (0.19 to 0.19) | +19% | +10% | 3.46 (3.45 to 3.50) | 18.2x |
| No-change update | peak RSS MB | 120 (120 to 122) | 120 (119 to 121) | 194 (193 to 195) | +61% | +62% | 2,645 (2,641 to 2,646) | 13.6x |

| Output | f442d3f, first run | f442d3f, this session | 9e292de | change | upstream 0.9.88 | 9e292de / upstream |
|---|---|---|---|---|---|---|
| files that own a node | 4,447 | 4,447 | 4,447 | +0% | 4,447 | 100% |
| nodes | 34,691 | 34,691 | 58,593 | +69% | 99,262 | 59% |
| edges | 61,293 | 61,293 | 121,769 | +99% | 222,456 | 55% |
| communities | 3,641 | 3,641 | 419 | -88% | 2,033 | 21% |
| graph.json MB | 28.6 | 28.6 | 58.5 | +105% | 100.0 | 58% |
| output directory MB | 69.9 | 69.9 | 132.0 | +89% | 298.2 | 44% |

Second-run records: 27. Highest CPU use by other processes during a run: 224% of one core. Longest wait for idle CPUs: 352 s. Runs that started without idle CPUs: 0.

### microsoft/TypeScript

| Phase | Metric | f442d3f, first run | f442d3f, this session | 9e292de | 9e292de against first run | against this session | upstream 0.9.88, first run | upstream / 9e292de |
|---|---|---|---|---|---|---|---|---|
| Cold build | wall s | 1.42 (1.37 to 1.55) | 1.35 (1.34 to 1.44) | 1.92 (1.69 to 2.12) | +35% | +42% | 182.4 (181.2 to 182.4) | 95.1x |
| Cold build | user CPU s | 7.47 (7.29 to 7.53) | 7.34 (7.33 to 7.36) | 9.81 (9.68 to 10.3) | +31% | +34% | 237.0 (234.4 to 237.1) | 24.2x |
| Cold build | system CPU s | 2.48 (2.47 to 2.51) | 2.51 (2.41 to 2.51) | 2.67 (2.49 to 2.75) | +8% | +6% | 32.8 (32.1 to 32.8) | 12.3x |
| Cold build | peak RSS MB | 324 (316 to 329) | 332 (328 to 334) | 413 (406 to 425) | +28% | +25% | 2,548 (2,543 to 2,552) | 6.2x |
| No-change update | wall s | 0.72 (0.71 to 0.73) | 0.74 (0.71 to 0.80) | 0.98 (0.97 to 1.00) | +37% | +33% | 168.3 (168.0 to 168.5) | 171.4x |
| No-change update | user CPU s | 2.41 (2.41 to 2.46) | 2.45 (2.40 to 2.46) | 3.26 (3.23 to 3.34) | +35% | +33% | 213.2 (213.1 to 213.2) | 65.4x |
| No-change update | system CPU s | 0.53 (0.53 to 0.54) | 0.61 (0.56 to 0.81) | 0.58 (0.58 to 0.61) | +11% | -5% | 25.5 (25.5 to 25.8) | 43.7x |
| No-change update | peak RSS MB | 335 (329 to 336) | 325 (324 to 327) | 430 (430 to 438) | +28% | +33% | 5,369 (5,355 to 5,372) | 12.5x |

| Output | f442d3f, first run | f442d3f, this session | 9e292de | change | upstream 0.9.88 | 9e292de / upstream |
|---|---|---|---|---|---|---|
| files that own a node | 31,747 | 31,747 | 31,747 | +0% | 31,988 | 99% |
| nodes | 120,367 | 120,367 | 174,482 | +45% | 196,431 | 89% |
| edges | 254,203 | 254,203 | 343,723 | +35% | 425,606 | 81% |
| communities | 21,045 | 21,045 | 27,724 | +32% | 28,647 | 97% |
| graph.json MB | 126.0 | 126.0 | 188.2 | +49% | 210.5 | 89% |
| output directory MB | 251.8 | 251.8 | 372.1 | +48% | 317.9 | 117% |

Second-run records: 25. Highest CPU use by other processes during a run: 282% of one core. Longest wait for idle CPUs: 448 s. Runs that started without idle CPUs: 0.

### kubernetes/kubernetes

| Phase | Metric | f442d3f, first run | f442d3f, this session | 9e292de | 9e292de against first run | against this session | upstream 0.9.88, first run | upstream / 9e292de |
|---|---|---|---|---|---|---|---|---|
| Cold build | wall s | 1.60 (1.54 to 1.79) | 1.62 (1.58 to 1.70) | 1.70 (1.65 to 1.90) | +6% | +5% | 407.1 (403.9 to 413.0) | 239.0x |
| Cold build | user CPU s | 10.3 (10.2 to 10.3) | 10.3 (9.74 to 10.4) | 10.5 (10.3 to 10.6) | +2% | +2% | 430.5 (428.3 to 437.6) | 41.0x |
| Cold build | system CPU s | 1.65 (1.62 to 1.68) | 1.58 (1.55 to 1.80) | 1.60 (1.59 to 1.90) | -3% | +1% | 37.6 (37.0 to 38.4) | 23.5x |
| Cold build | peak RSS MB | 509 (500 to 523) | 512 (508 to 515) | 529 (521 to 530) | +4% | +3% | 4,442 (4,426 to 4,450) | 8.4x |
| No-change update | wall s | 1.03 (0.98 to 1.03) | 1.05 (1.02 to 1.11) | 1.15 (1.13 to 1.16) | +11% | +9% | 309.5 (305.5 to 310.7) | 270.3x |
| No-change update | user CPU s | 4.38 (4.35 to 4.42) | 4.41 (4.38 to 4.42) | 4.60 (4.56 to 4.64) | +5% | +4% | 288.6 (285.1 to 289.5) | 62.7x |
| No-change update | system CPU s | 0.59 (0.58 to 0.65) | 0.66 (0.65 to 0.84) | 0.63 (0.60 to 0.63) | +7% | -4% | 22.1 (21.9 to 22.6) | 34.9x |
| No-change update | peak RSS MB | 529 (524 to 534) | 505 (504 to 518) | 532 (524 to 534) | +1% | +5% | 8,348 (8,344 to 8,352) | 15.7x |

| Output | f442d3f, first run | f442d3f, this session | 9e292de | change | upstream 0.9.88 | 9e292de / upstream |
|---|---|---|---|---|---|---|
| files that own a node | 14,164 | 14,164 | 14,164 | +0% | 14,165 | 100% |
| nodes | 152,610 | 152,610 | 159,418 | +4% | 183,215 | 87% |
| edges | 833,321 | 833,321 | 841,668 | +1% | 1,239,065 | 68% |
| communities | 602 | 602 | 771 | +28% | 7,861 | 10% |
| graph.json MB | 344.5 | 344.5 | 369.4 | +7% | 482.8 | 77% |
| output directory MB | 540.6 | 540.6 | 584.7 | +8% | 943.4 | 62% |

Second-run records: 27. Highest CPU use by other processes during a run: 232% of one core. Longest wait for idle CPUs: 336 s. Runs that started without idle CPUs: 0.

### microsoft/vscode

| Phase | Metric | f442d3f, first run | f442d3f, this session | 9e292de | 9e292de against first run | against this session | upstream 0.9.88, first run | upstream / 9e292de |
|---|---|---|---|---|---|---|---|---|
| Cold build | wall s | 1.65 (1.65 to 2.09) | 1.59 (1.58 to 1.80) | 3.54 (3.40 to 3.62) | +114% | +122% | 284.1 (283.2 to 295.9) | 80.3x |
| Cold build | user CPU s | 11.9 (11.6 to 12.0) | 12.1 (11.6 to 12.4) | 27.4 (27.3 to 27.5) | +130% | +126% | 442.4 (437.0 to 444.3) | 16.1x |
| Cold build | system CPU s | 1.75 (1.70 to 1.92) | 1.66 (1.63 to 1.95) | 1.83 (1.76 to 1.86) | +5% | +10% | 30.8 (30.2 to 31.4) | 16.8x |
| Cold build | peak RSS MB | 554 (505 to 578) | 548 (544 to 556) | 789 (780 to 795) | +43% | +44% | 6,902 (6,891 to 6,905) | 8.7x |
| No-change update | wall s | 0.95 (0.94 to 0.96) | 0.95 (0.94 to 0.96) | 1.74 (1.73 to 1.75) | +82% | +83% | 345.8 (328.2 to 347.0) | 199.2x |
| No-change update | user CPU s | 4.40 (4.21 to 4.41) | 4.32 (4.31 to 4.41) | 6.85 (6.78 to 6.86) | +56% | +58% | 493.9 (482.9 to 496.2) | 72.1x |
| No-change update | system CPU s | 0.61 (0.60 to 0.63) | 0.60 (0.59 to 0.62) | 0.70 (0.69 to 0.71) | +14% | +16% | 30.6 (30.1 to 31.8) | 43.9x |
| No-change update | peak RSS MB | 520 (517 to 535) | 533 (526 to 536) | 750 (747 to 761) | +44% | +41% | 14,299 (14,256 to 14,303) | 19.1x |

| Output | f442d3f, first run | f442d3f, this session | 9e292de | change | upstream 0.9.88 | 9e292de / upstream |
|---|---|---|---|---|---|---|
| files that own a node | 15,382 | 15,382 | 15,382 | +0% | 15,516 | 99% |
| nodes | 200,704 | 200,704 | 219,294 | +9% | 256,511 | 85% |
| edges | 448,108 | 448,108 | 787,785 | +76% | 993,942 | 79% |
| communities | 4,232 | 4,232 | 5,404 | +28% | 5,411 | 100% |
| graph.json MB | 227.6 | 227.6 | 396.5 | +74% | 467.4 | 85% |
| output directory MB | 446.7 | 446.7 | 706.6 | +58% | 508.2 | 139% |

Second-run records: 24. Highest CPU use by other processes during a run: 349% of one core. Longest wait for idle CPUs: 128 s. Runs that started without idle CPUs: 0.

## Cells that got worse

Timing cells where the 9e292de median is above the same-session f442d3f median, and size cells that grew. Several are inside run-to-run noise (a few percent), for example graphify cold user CPU +1% and kubernetes cold system CPU +1%.

| Corpus | Cell | f442d3f, first run | f442d3f, this session | 9e292de | against first run | against this session | worse than upstream |
|---|---|---|---|---|---|---|---|
| Graphify-Labs/graphify | Cold build, user CPU s | 0.67 | 0.68 | 0.68 | +1% | +1% | no |
| Graphify-Labs/graphify | Cold build, peak RSS MB | 133 | 128 | 133 | +0% | +4% | no |
| Graphify-Labs/graphify | No-change update, wall s | 0.09 | 0.09 | 0.11 | +22% | +19% | no |
| Graphify-Labs/graphify | No-change update, system CPU s | 0.06 | 0.06 | 0.07 | +16% | +7% | no |
| Graphify-Labs/graphify | No-change update, peak RSS MB | 71 | 70 | 74 | +5% | +5% | no |
| Graphify-Labs/graphify | graph.json MB | 15.8 | 15.8 | 16.1 | +2% | +2% | no |
| Graphify-Labs/graphify | output directory MB | 30.9 | 30.9 | 31.5 | +2% | +2% | no |
| redis/redis | Cold build, wall s | 0.16 | 0.14 | 0.19 | +20% | +33% | no |
| redis/redis | Cold build, user CPU s | 0.72 | 0.74 | 0.77 | +6% | +4% | no |
| redis/redis | Cold build, system CPU s | 0.18 | 0.18 | 0.18 | +1% | +3% | no |
| redis/redis | No-change update, wall s | 0.08 | 0.08 | 0.13 | +69% | +71% | no |
| redis/redis | No-change update, user CPU s | 0.22 | 0.22 | 0.27 | +21% | +23% | no |
| redis/redis | No-change update, system CPU s | 0.06 | 0.06 | 0.08 | +40% | +37% | no |
| redis/redis | No-change update, peak RSS MB | 62 | 62 | 71 | +15% | +14% | no |
| redis/redis | graph.json MB | 12.7 | 12.7 | 16.7 | +31% | +31% | no |
| redis/redis | output directory MB | 23.0 | 23.0 | 29.5 | +28% | +28% | no |
| curl/curl | Cold build, wall s | 0.12 | 0.12 | 0.13 | +15% | +13% | no |
| curl/curl | Cold build, user CPU s | 0.54 | 0.56 | 0.64 | +18% | +13% | no |
| curl/curl | Cold build, peak RSS MB | 92 | 90 | 97 | +5% | +8% | no |
| curl/curl | No-change update, wall s | 0.07 | 0.07 | 0.09 | +20% | +20% | no |
| curl/curl | No-change update, user CPU s | 0.20 | 0.20 | 0.26 | +35% | +30% | no |
| curl/curl | No-change update, system CPU s | 0.06 | 0.07 | 0.07 | +11% | +5% | no |
| curl/curl | No-change update, peak RSS MB | 54 | 54 | 67 | +24% | +26% | no |
| curl/curl | graph.json MB | 10.4 | 10.4 | 14.8 | +42% | +42% | no |
| curl/curl | output directory MB | 19.1 | 19.1 | 26.7 | +40% | +40% | no |
| opencv/opencv | Cold build, wall s | 0.47 | 0.49 | 0.57 | +23% | +17% | no |
| opencv/opencv | Cold build, user CPU s | 4.04 | 4.01 | 4.54 | +12% | +13% | no |
| opencv/opencv | Cold build, system CPU s | 0.48 | 0.48 | 0.50 | +4% | +3% | no |
| opencv/opencv | Cold build, peak RSS MB | 225 | 221 | 243 | +8% | +10% | no |
| opencv/opencv | No-change update, wall s | 0.18 | 0.18 | 0.29 | +61% | +58% | no |
| opencv/opencv | No-change update, user CPU s | 0.73 | 0.72 | 1.21 | +66% | +69% | no |
| opencv/opencv | No-change update, system CPU s | 0.16 | 0.17 | 0.19 | +19% | +10% | no |
| opencv/opencv | No-change update, peak RSS MB | 120 | 120 | 194 | +61% | +62% | no |
| opencv/opencv | graph.json MB | 28.6 | 28.6 | 58.5 | +105% | +105% | no |
| opencv/opencv | output directory MB | 69.9 | 69.9 | 132.0 | +89% | +89% | no |
| microsoft/TypeScript | Cold build, wall s | 1.42 | 1.35 | 1.92 | +35% | +42% | no |
| microsoft/TypeScript | Cold build, user CPU s | 7.47 | 7.34 | 9.81 | +31% | +34% | no |
| microsoft/TypeScript | Cold build, system CPU s | 2.48 | 2.51 | 2.67 | +8% | +6% | no |
| microsoft/TypeScript | Cold build, peak RSS MB | 324 | 332 | 413 | +28% | +25% | no |
| microsoft/TypeScript | No-change update, wall s | 0.72 | 0.74 | 0.98 | +37% | +33% | no |
| microsoft/TypeScript | No-change update, user CPU s | 2.41 | 2.45 | 3.26 | +35% | +33% | no |
| microsoft/TypeScript | No-change update, peak RSS MB | 335 | 325 | 430 | +28% | +33% | no |
| microsoft/TypeScript | graph.json MB | 126.0 | 126.0 | 188.2 | +49% | +49% | no |
| microsoft/TypeScript | output directory MB | 251.8 | 251.8 | 372.1 | +48% | +48% | yes |
| kubernetes/kubernetes | Cold build, wall s | 1.60 | 1.62 | 1.70 | +6% | +5% | no |
| kubernetes/kubernetes | Cold build, user CPU s | 10.3 | 10.3 | 10.5 | +2% | +2% | no |
| kubernetes/kubernetes | Cold build, system CPU s | 1.65 | 1.58 | 1.60 | -3% | +1% | no |
| kubernetes/kubernetes | Cold build, peak RSS MB | 509 | 512 | 529 | +4% | +3% | no |
| kubernetes/kubernetes | No-change update, wall s | 1.03 | 1.05 | 1.15 | +11% | +9% | no |
| kubernetes/kubernetes | No-change update, user CPU s | 4.38 | 4.41 | 4.60 | +5% | +4% | no |
| kubernetes/kubernetes | No-change update, peak RSS MB | 529 | 505 | 532 | +1% | +5% | no |
| kubernetes/kubernetes | graph.json MB | 344.5 | 344.5 | 369.4 | +7% | +7% | no |
| kubernetes/kubernetes | output directory MB | 540.6 | 540.6 | 584.7 | +8% | +8% | no |
| microsoft/vscode | Cold build, wall s | 1.65 | 1.59 | 3.54 | +114% | +122% | no |
| microsoft/vscode | Cold build, user CPU s | 11.9 | 12.1 | 27.4 | +130% | +126% | no |
| microsoft/vscode | Cold build, system CPU s | 1.75 | 1.66 | 1.83 | +5% | +10% | no |
| microsoft/vscode | Cold build, peak RSS MB | 554 | 548 | 789 | +43% | +44% | no |
| microsoft/vscode | No-change update, wall s | 0.95 | 0.95 | 1.74 | +82% | +83% | no |
| microsoft/vscode | No-change update, user CPU s | 4.40 | 4.32 | 6.85 | +56% | +58% | no |
| microsoft/vscode | No-change update, system CPU s | 0.61 | 0.60 | 0.70 | +14% | +16% | no |
| microsoft/vscode | No-change update, peak RSS MB | 520 | 533 | 750 | +44% | +41% | no |
| microsoft/vscode | graph.json MB | 227.6 | 227.6 | 396.5 | +74% | +74% | no |
| microsoft/vscode | output directory MB | 446.7 | 446.7 | 706.6 | +58% | +58% | yes |

## Split with --no-cluster

"cluster share" is 1 minus (no-cluster / full). For upstream that covers the graph build, clustering, analysis and report, and the full numbers are first-run numbers (see Limits 4 and 5).

### Cold build

| Corpus | Metric | 9e292de | 9e292de --no-cluster | cluster share | upstream, first run | upstream --no-cluster | cluster share | upstream / 9e292de, both --no-cluster |
|---|---|---|---|---|---|---|---|---|
| Graphify-Labs/graphify | wall s | 0.14 | 0.13 | 3% | 12.0 | 7.31 | 39% | 54.6x |
| Graphify-Labs/graphify | CPU s, user + system | 0.82 | 0.82 | 0% | 22.2 | 18.5 | 17% | 22.5x |
| Graphify-Labs/graphify | peak RSS MB | 133 | 139 | n/a | 1,166 | 1,168 | n/a | 8.4x |
| redis/redis | wall s | 0.19 | 0.13 | 32% | 9.47 | 3.40 | 64% | 26.4x |
| redis/redis | CPU s, user + system | 0.96 | 0.91 | 4% | 19.8 | 13.5 | 32% | 14.8x |
| redis/redis | peak RSS MB | 116 | 121 | n/a | 1,220 | 1,212 | n/a | 10.0x |
| curl/curl | wall s | 0.13 | 0.12 | 13% | 7.75 | 3.63 | 53% | 31.3x |
| curl/curl | CPU s, user + system | 0.84 | 0.80 | 5% | 18.0 | 13.9 | 23% | 17.5x |
| curl/curl | peak RSS MB | 97 | 98 | n/a | 1,047 | 1,040 | n/a | 10.6x |
| opencv/opencv | wall s | 0.57 | 0.52 | 9% | 61.3 | 21.3 | 65% | 40.8x |
| opencv/opencv | CPU s, user + system | 5.04 | 4.96 | 1% | 103.7 | 65.0 | 37% | 13.1x |
| opencv/opencv | peak RSS MB | 243 | 254 | n/a | 1,889 | 1,867 | n/a | 7.3x |
| microsoft/TypeScript | wall s | 1.92 | 1.55 | 19% | 182.4 | 127.5 | 30% | 82.3x |
| microsoft/TypeScript | CPU s, user + system | 12.5 | 12.0 | 4% | 269.8 | 212.6 | 21% | 17.7x |
| microsoft/TypeScript | peak RSS MB | 413 | 414 | n/a | 2,548 | 3,607 | n/a | 8.7x |
| kubernetes/kubernetes | wall s | 1.70 | 1.33 | 22% | 407.1 | 270.4 | 34% | 203.9x |
| kubernetes/kubernetes | CPU s, user + system | 12.2 | 11.5 | 5% | 468.6 | 336.8 | 28% | 29.3x |
| kubernetes/kubernetes | peak RSS MB | 529 | 534 | n/a | 4,442 | 3,656 | n/a | 6.9x |
| microsoft/vscode | wall s | 3.54 | 3.13 | 12% | 284.1 | 135.2 | 52% | 43.2x |
| microsoft/vscode | CPU s, user + system | 29.3 | 28.7 | 2% | 473.8 | 325.6 | 31% | 11.3x |
| microsoft/vscode | peak RSS MB | 789 | 785 | n/a | 6,902 | 9,453 | n/a | 12.0x |

### No-change update

| Corpus | Metric | 9e292de | 9e292de --no-cluster | cluster share | upstream, first run | upstream --no-cluster | cluster share | upstream / 9e292de, both --no-cluster |
|---|---|---|---|---|---|---|---|---|
| Graphify-Labs/graphify | wall s | 0.11 | 0.11 | 1% | 8.38 | 7.97 | 5% | 75.2x |
| Graphify-Labs/graphify | CPU s, user + system | 0.34 | 0.31 | 9% | 8.47 | 8.05 | 5% | 25.9x |
| Graphify-Labs/graphify | peak RSS MB | 74 | 69 | n/a | 797 | 976 | n/a | 14.2x |
| redis/redis | wall s | 0.13 | 0.09 | 34% | 5.94 | 5.41 | 9% | 62.9x |
| redis/redis | CPU s, user + system | 0.35 | 0.31 | 11% | 6.07 | 5.55 | 9% | 17.8x |
| redis/redis | peak RSS MB | 71 | 68 | n/a | 584 | 758 | n/a | 11.2x |
| curl/curl | wall s | 0.09 | 0.09 | 2% | 5.13 | 4.62 | 10% | 52.5x |
| curl/curl | CPU s, user + system | 0.33 | 0.30 | 10% | 5.32 | 4.81 | 10% | 16.0x |
| curl/curl | peak RSS MB | 67 | 70 | n/a | 450 | 468 | n/a | 6.7x |
| opencv/opencv | wall s | 0.29 | 0.28 | 2% | 80.0 | 32.3 | 60% | 114.2x |
| opencv/opencv | CPU s, user + system | 1.40 | 1.29 | 8% | 81.8 | 35.9 | 56% | 27.8x |
| opencv/opencv | peak RSS MB | 194 | 192 | n/a | 2,645 | 2,152 | n/a | 11.2x |
| microsoft/TypeScript | wall s | 0.98 | 0.95 | 3% | 168.3 | 140.1 | 17% | 147.4x |
| microsoft/TypeScript | CPU s, user + system | 3.84 | 3.47 | 10% | 238.7 | 210.9 | 12% | 60.8x |
| microsoft/TypeScript | peak RSS MB | 430 | 427 | n/a | 5,369 | 8,390 | n/a | 19.7x |
| kubernetes/kubernetes | wall s | 1.15 | 1.00 | 13% | 309.5 | 299.5 | 3% | 299.8x |
| kubernetes/kubernetes | CPU s, user + system | 5.24 | 4.68 | 11% | 310.7 | 301.0 | 3% | 64.3x |
| kubernetes/kubernetes | peak RSS MB | 532 | 538 | n/a | 8,348 | 8,752 | n/a | 16.3x |
| microsoft/vscode | wall s | 1.74 | 1.70 | 2% | 345.8 | 154.3 | 55% | 90.8x |
| microsoft/vscode | CPU s, user + system | 7.54 | 6.84 | 9% | 524.5 | 344.3 | 34% | 50.3x |
| microsoft/vscode | peak RSS MB | 750 | 743 | n/a | 14,299 | 14,683 | n/a | 19.8x |

## Upstream with the leiden extra, cold build

Harness numbers; Leiden is compared with the first-run Louvain numbers. Same-session numbers for redis and opencv are in "What the first run left out".

| Corpus | Metric | upstream, Louvain, first run | upstream, Leiden | Leiden against Louvain | upstream --no-cluster | 9e292de | upstream Leiden / 9e292de |
|---|---|---|---|---|---|---|---|
| redis/redis | wall s | 9.47 (9.39 to 9.56) | 8.80 (8.73 to 9.54) | -7% | 3.40 (3.37 to 3.45) | 0.19 (0.14 to 0.20) | 46.1x |
| redis/redis | user CPU s | 16.7 (16.4 to 16.8) | 15.3 (15.2 to 15.3) | -9% | 10.6 (10.6 to 10.7) | 0.77 (0.75 to 0.78) | 19.9x |
| redis/redis | system CPU s | 3.09 (3.05 to 3.11) | 2.71 (2.68 to 2.74) | -12% | 2.92 (2.88 to 2.92) | 0.18 (0.17 to 0.20) | 14.9x |
| redis/redis | peak RSS MB | 1,220 (1,193 to 1,230) | 1,178 (1,157 to 1,201) | -3% | 1,212 (1,196 to 1,220) | 116 (111 to 120) | 10.2x |
| redis/redis | nodes | 17,212 | 17,212 | +0% | n/a | 14,130 | n/a |
| redis/redis | edges | 61,076 | 61,076 | +0% | n/a | 47,095 | n/a |
| redis/redis | communities | 493 | 511 | +4% | n/a | 219 | n/a |
| opencv/opencv | wall s | 61.3 (61.2 to 61.6) | 56.8 (56.1 to 58.7) | -7% | 21.3 (20.9 to 21.8) | 0.57 (0.57 to 0.60) | 98.7x |
| opencv/opencv | user CPU s | 95.9 (95.4 to 96.4) | 91.2 (90.5 to 91.3) | -5% | 58.1 (56.3 to 58.9) | 4.54 (4.52 to 4.61) | 20.1x |
| opencv/opencv | system CPU s | 7.91 (7.80 to 7.98) | 7.69 (7.63 to 7.80) | -3% | 6.86 (6.70 to 6.89) | 0.50 (0.50 to 0.51) | 15.4x |
| opencv/opencv | peak RSS MB | 1,889 (1,881 to 1,889) | 1,894 (1,856 to 1,900) | +0% | 1,867 (1,841 to 1,879) | 243 (239 to 246) | 7.8x |
| opencv/opencv | nodes | 99,262 | 99,262 | +0% | n/a | 58,593 | n/a |
| opencv/opencv | edges | 222,456 | 222,456 | +0% | n/a | 121,769 | n/a |
| opencv/opencv | communities | 2,033 | 1,874 | -8% | n/a | 419 | n/a |
| kubernetes/kubernetes | wall s | 407.1 (403.9 to 413.0) | 393.9 (388.4 to 398.3) | -3% | 270.4 (268.3 to 275.8) | 1.70 (1.65 to 1.90) | 231.3x |
| kubernetes/kubernetes | user CPU s | 430.5 (428.3 to 437.6) | 413.0 (412.4 to 416.0) | -4% | 298.7 (298.7 to 300.2) | 10.5 (10.3 to 10.6) | 39.3x |
| kubernetes/kubernetes | system CPU s | 37.6 (37.0 to 38.4) | 38.7 (38.7 to 39.0) | +3% | 38.1 (37.8 to 39.9) | 1.60 (1.59 to 1.90) | 24.2x |
| kubernetes/kubernetes | peak RSS MB | 4,442 (4,426 to 4,450) | 4,697 (4,694 to 4,708) | +6% | 3,656 (3,654 to 3,658) | 529 (521 to 530) | 8.9x |
| kubernetes/kubernetes | nodes | 183,215 | 183,215 | +0% | n/a | 159,418 | n/a |
| kubernetes/kubernetes | edges | 1,239,065 | 1,239,065 | +0% | n/a | 841,668 | n/a |
| kubernetes/kubernetes | communities | 7,861 | 8,241 | +5% | n/a | 771 | n/a |
