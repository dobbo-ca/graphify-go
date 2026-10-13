#!/usr/bin/env python3
"""Print the per-corpus markdown tables from runs/results.jsonl. usage: report.py [corpus...]"""
import json, statistics, subprocess, sys
from pathlib import Path

S = Path(__file__).resolve().parent
NAMES = {"go": "graphify-go f442d3f (no cgo)", "cgo": "graphify-go v0.10.1 (cgo)", "up": "upstream 0.9.88"}
TITLES = {"graphify": "Graphify-Labs/graphify", "redis": "redis/redis", "curl": "curl/curl", "opencv": "opencv/opencv",
          "typescript": "microsoft/TypeScript", "vscode": "microsoft/vscode", "k8s": "kubernetes/kubernetes"}
SIZES = json.load(open(S / "sizes.json"))
R = [json.loads(l) for l in open(S / "runs/results.jsonl")]


def fmt(v, unit):
    if unit == "s":
        return f"{v:.2f}" if v < 10 else f"{v:.1f}"
    return f"{v:,.0f}"


def cell(xs, unit):
    if not xs:
        return "n/a"
    m = statistics.median(xs)
    return f"{fmt(m, unit)} ({fmt(min(xs), unit)} to {fmt(max(xs), unit)})"


def du(p):
    if not p.exists():
        return None
    return int(subprocess.run(["du", "-sk", str(p)], capture_output=True, text=True).stdout.split()[0]) / 1024


slower = []
for c in sys.argv[1:] or list(TITLES):
    rows = [r for r in R if r["corpus"] == c]
    if not rows:
        continue
    f = json.load(open(S / f"prep/{c}/files.json"))
    print(f"### {TITLES[c]} @ {f['commit']}\n")
    print(f"Files read by default: graphify-go {f['go_default']:,}, upstream {f['up_default']:,}. "
          f"Benchmarked on the {f['both']:,} files both read ({f['both_mb']:,.0f} MB)"
          + (f", plus {f['manifests_added']} package manifest files.\n" if f.get("manifests_added") else ".\n"))
    print("| Phase | Metric | " + " | ".join(NAMES.values()) + " | upstream / no-cgo |")
    print("|---|---|---|---|---|---|")
    for ph, label in (("cold", "Cold build"), ("nochange", "No-change update")):
        for key, name, unit in (("wall", "wall s", "s"), ("user", "user CPU s", "s"), ("sys", "system CPU s", "s"), ("rss", "peak RSS MB", "mb")):
            med, cells = {}, []
            for t in NAMES:
                x = [r for r in rows if r["tool"] == t and r["phase"] == ph and not r["killed"] and r["exit"] == 0]
                xs = [max(r["rss_tree_mb"], r["rss_max_mb"]) if key == "rss" else r[key] for r in x]
                cells.append(cell(xs, unit))
                med[t] = statistics.median(xs) if xs else None
                bad = [r for r in rows if r["tool"] == t and r["phase"] == ph and (r["killed"] or r["exit"] != 0)]
                if bad and not xs:
                    cells[-1] = f"stopped: {bad[0]['killed'] or 'exit ' + str(bad[0]['exit'])}"
            ratio = f"{med['up'] / med['go']:.1f}x" if med["up"] and med["go"] else "n/a"
            print(f"| {label} | {name} | " + " | ".join(cells) + f" | {ratio} |")
            if med["up"] and med["go"] and med["go"] > med["up"]:
                slower.append(f"{c} {label} {name}: no-cgo {fmt(med['go'], unit)} against upstream {fmt(med['up'], unit)}")
            if med["cgo"] and med["go"] and med["go"] > med["cgo"] * 1.05:
                slower.append(f"{c} {label} {name}: no-cgo {fmt(med['go'], unit)} against cgo {fmt(med['cgo'], unit)}")
    print("\n| Output | " + " | ".join(NAMES.values()) + " |")
    print("|---|---|---|---|")
    for key, name in (("src_files", "files that own a node"), ("nodes", "nodes"), ("edges", "edges"), ("communities", "communities")):
        cells = []
        for t in NAMES:
            v = sorted({r[key] for r in rows if r["tool"] == t and r["phase"] == "cold" and r.get(key) is not None})
            cells.append(" / ".join(f"{x:,}" for x in v) if v else "n/a")
        print(f"| {name} | " + " | ".join(cells) + " |")
    for k, name in (("graph", "graph.json MB"), ("out", "output directory MB")):
        cells = []
        for t in NAMES:
            x = [r for r in rows if r["tool"] == t and r["phase"] == "nochange" and r.get("out_mb")]
            v = (x[-1]["graph_mb"] if k == "graph" else x[-1]["out_mb"]) if x else SIZES.get(f"{c}.{t}", {}).get(k)
            cells.append(v)
        print(f"| {name} | " + " | ".join("n/a" if v is None else f"{v:,.1f}" for v in cells) + " |")
    worst = max((r["other_cpu_pct"] or 0) for r in rows)
    print(f"\nRuns: {len(rows)}. Highest CPU use by other processes during a run: {worst:.0f}% of one core.\n")
if slower:
    print("### Cases where the no-cgo build is slower or larger\n")
    for s in slower:
        print("- " + s)
