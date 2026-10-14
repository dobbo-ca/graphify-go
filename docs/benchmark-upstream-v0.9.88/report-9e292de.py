#!/usr/bin/env python3
"""Print the tables of the second run (graphify-go 9e292de) next to the first run.

usage: report2.py [corpus...]
reads: runs/results.jsonl (first run, kept), runs/results-9e292de.jsonl (second run), sizes.json
"""
import json, statistics, sys
from pathlib import Path

S = Path(__file__).resolve().parent
TITLES = {"graphify": "Graphify-Labs/graphify", "redis": "redis/redis", "curl": "curl/curl", "opencv": "opencv/opencv",
          "typescript": "microsoft/TypeScript", "k8s": "kubernetes/kubernetes", "vscode": "microsoft/vscode"}
SIZES = json.load(open(S / "sizes.json"))
load = lambda n: [json.loads(l) for l in open(S / "runs" / n)]
R = [dict(r, tool="rec-" + r["tool"]) for r in load("results.jsonl")] + load("results-9e292de.jsonl")
METRICS = (("wall", "wall s", "s"), ("user", "user CPU s", "s"), ("sys", "system CPU s", "s"), ("rss", "peak RSS MB", "mb"))
PHASES = (("cold", "Cold build"), ("nochange", "No-change update"))


def fmt(v, unit="mb"):
    if v is None:
        return "n/a"
    if unit == "s":
        return f"{v:.2f}" if v < 10 else f"{v:.1f}"
    return f"{v:,.0f}"


def vals(c, tool, ph, key):
    x = [r for r in R if r["corpus"] == c and r["tool"] == tool and r["phase"] == ph and not r["killed"] and r["exit"] == 0]
    return [max(r["rss_tree_mb"], r["rss_max_mb"]) if key == "rss" else r[key] for r in x]


def med(c, tool, ph, key):
    xs = vals(c, tool, ph, key)
    return statistics.median(xs) if xs else None


def cell(c, tool, ph, key, unit):
    xs = vals(c, tool, ph, key)
    return f"{fmt(statistics.median(xs), unit)} ({fmt(min(xs), unit)} to {fmt(max(xs), unit)})" if xs else "n/a"


def pct(new, old):
    return "n/a" if not new or not old else f"{(new / old - 1) * 100:+.0f}%"


def ratio(a, b):
    return "n/a" if not a or not b else f"{a / b:.1f}x"


def out(c, tool, key):
    """nodes/edges/communities from the cold records, sizes from the last record that has them."""
    if key in ("nodes", "edges", "communities", "src_files"):
        v = sorted({r[key] for r in R if r["corpus"] == c and r["tool"] == tool and r["phase"] == "cold" and r.get(key) is not None})
        return v[-1] if v else None, len(v) > 1
    x = [r for r in R if r["corpus"] == c and r["tool"] == tool and r.get("out_mb")]
    if x:
        return x[-1][key], False
    return SIZES.get(f"{c}.{tool.removeprefix('rec-')}", {}).get({"graph_mb": "graph", "out_mb": "out"}[key]), False


corpora = sys.argv[1:] or list(TITLES)
worse = []
print("## Main table\n")
for c in corpora:
    print(f"### {TITLES[c]}\n")
    print("| Phase | Metric | f442d3f, first run | f442d3f, this session | 9e292de | 9e292de against first run | against this session | upstream 0.9.88, first run | upstream / 9e292de |")
    print("|---|---|---|---|---|---|---|---|---|")
    for ph, label in PHASES:
        for key, name, unit in METRICS:
            a, b, n, u = (med(c, t, ph, key) for t in ("rec-go", "old", "new", "rec-up"))
            print(f"| {label} | {name} | {cell(c, 'rec-go', ph, key, unit)} | {cell(c, 'old', ph, key, unit)} | {cell(c, 'new', ph, key, unit)} "
                  f"| {pct(n, a)} | {pct(n, b)} | {cell(c, 'rec-up', ph, key, unit)} | {ratio(u, n)} |")
            if n and b and n > b:  # same-session baseline: same disk, same load window
                worse.append((c, f"{label}, {name}", fmt(a, unit), fmt(b, unit), fmt(n, unit), pct(n, a), pct(n, b),
                              "yes" if u and n > u else "no"))
    print("\n| Output | f442d3f, first run | f442d3f, this session | 9e292de | change | upstream 0.9.88 | 9e292de / upstream |")
    print("|---|---|---|---|---|---|---|")
    for key, name in (("src_files", "files that own a node"), ("nodes", "nodes"), ("edges", "edges"), ("communities", "communities"),
                      ("graph_mb", "graph.json MB"), ("out_mb", "output directory MB")):
        (a, _), (b, _), (n, nvar), (u, uvar) = (out(c, t, key) for t in ("rec-go", "old", "new", "rec-up"))
        f = (lambda v: "n/a" if v is None else f"{v:,.1f}") if key.endswith("_mb") else (lambda v: "n/a" if v is None else f"{v:,}")
        print(f"| {name} | {f(a)} | {f(b)} | {f(n)}{' (varies)' if nvar else ''} | {pct(n, a)} | {f(u)}{' (varies)' if uvar else ''} "
              f"| {'n/a' if not n or not u else f'{n / u * 100:.0f}%'} |")
        if key.endswith("_mb") and n and a and n > a:
            worse.append((c, name, f(a), f(b), f(n), pct(n, a), pct(n, b), "yes" if u and n > u else "no"))
    rows = [r for r in R if r["corpus"] == c and r["tool"] in ("old", "new", "new-nc", "up-nc", "up-ld")]
    late = [r for r in rows if r["idle_wait_s"] >= 7000]
    print(f"\nSecond-run records: {len(rows)}. Highest CPU use by other processes during a run: "
          f"{max((r['other_cpu_pct'] or 0) for r in rows) if rows else 0:.0f}% of one core. "
          f"Longest wait for idle CPUs: {max(r['idle_wait_s'] for r in rows) if rows else 0} s. Runs that started without idle CPUs: {len(late)}.\n")

print("## Cells that got worse\n")
print("| Corpus | Cell | f442d3f, first run | f442d3f, this session | 9e292de | against first run | against this session | worse than upstream |")
print("|---|---|---|---|---|---|---|---|")
for w in worse:
    print(f"| {TITLES[w[0]]} | " + " | ".join(w[1:]) + " |")

print("\n## Split with --no-cluster\n")
for ph, label in PHASES:
    print(f"### {label}\n")
    print("| Corpus | Metric | 9e292de | 9e292de --no-cluster | cluster share | upstream, first run | upstream --no-cluster | cluster share | upstream / 9e292de, both --no-cluster |")
    print("|---|---|---|---|---|---|---|---|---|")
    for c in corpora:
        for key, name, unit in (METRICS[0], ("cpu", "CPU s, user + system", "s"), METRICS[3]):
            if key == "cpu":
                g = lambda t: None if med(c, t, ph, "user") is None else statistics.median(
                    [a + b for a, b in zip(vals(c, t, ph, "user"), vals(c, t, ph, "sys"))])
            else:
                g = lambda t: med(c, t, ph, key)
            n, nn, u, un = g("new"), g("new-nc"), g("rec-up"), g("up-nc")
            share = lambda full, nc: "n/a" if not full or not nc or key == "rss" else f"{(1 - nc / full) * 100:.0f}%"
            print(f"| {TITLES[c]} | {name} | {fmt(n, unit)} | {fmt(nn, unit)} | {share(n, nn)} | {fmt(u, unit)} | {fmt(un, unit)} | {share(u, un)} | {ratio(un, nn)} |")
    print()

print("## Upstream with the leiden extra, cold build\n")
print("| Corpus | Metric | upstream, Louvain, first run | upstream, Leiden | Leiden against Louvain | upstream --no-cluster | 9e292de | upstream Leiden / 9e292de |")
print("|---|---|---|---|---|---|---|---|")
for c in corpora:
    if not vals(c, "up-ld", "cold", "wall"):
        continue
    for key, name, unit in METRICS:
        u, l, n = med(c, "rec-up", "cold", key), med(c, "up-ld", "cold", key), med(c, "new", "cold", key)
        print(f"| {TITLES[c]} | {name} | {cell(c, 'rec-up', 'cold', key, unit)} | {cell(c, 'up-ld', 'cold', key, unit)} | {pct(l, u)} "
              f"| {cell(c, 'up-nc', 'cold', key, unit)} | {cell(c, 'new', 'cold', key, unit)} | {ratio(l, n)} |")
    for key in ("nodes", "edges", "communities"):
        (u, uv), (l, lv), (n, _) = out(c, "rec-up", key), out(c, "up-ld", key), out(c, "new", key)
        print(f"| {TITLES[c]} | {key} | {u:,}{' (varies)' if uv else ''} | {l:,}{' (varies)' if lv else ''} | {pct(l, u)} | n/a | {n:,} | n/a |")
