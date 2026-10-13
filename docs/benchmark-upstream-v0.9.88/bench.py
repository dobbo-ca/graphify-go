#!/usr/bin/env python3
"""Interleaved cold build + no-change update for three tools on the aligned corpora.

usage: bench.py <runs> <corpus>...        appends to runs/results.jsonl
One process tree at a time. Each run goes through run1.py (nice 19, RSS watchdog).
"""
import json, os, shutil, subprocess, sys, time
from pathlib import Path

S = Path(__file__).resolve().parent
CGO = Path.home() / ".cache/nocgo-c41e/bin/final-base"
OUTROOT = Path(os.environ.get("BENCH_OUT", S))  # the SSD is full: later runs write to a RAM disk
TOOLS = {
    "go": ([str(S / "bin/gfy-nocgo"), "build"], [str(S / "bin/gfy-nocgo"), "update"]),
    "cgo": ([str(CGO), "build"], [str(CGO), "update"]),
    "up": ([str(S / "venv/bin/graphify"), "update"], [str(S / "venv/bin/graphify"), "update"]),
}
ENV = {k: v for k, v in os.environ.items() if not k.endswith("_API_KEY")}
ENV.update(GRAPHIFY_NO_AUTO_REFRESH="1", GRAPHIFY_NO_TIPS="1", GRAPHIFY_NO_BACKUP="1",
           GRAPHIFY_VIZ_NODE_LIMIT="0", GRAPHIFY_MAX_GRAPH_BYTES="8GB")
ORDERS = [["go", "cgo", "up"], ["up", "go", "cgo"], ["cgo", "up", "go"]]


def idle():
    out = subprocess.run(["top", "-l", "2", "-n", "0", "-s", "1"], capture_output=True, text=True).stdout
    line = [l for l in out.splitlines() if "CPU usage" in l][-1]
    return float(line.split("sys,")[1].split("%")[0])


def quiet():
    """Wait until other work leaves the CPUs (another session can run a job)."""
    waited = 0
    while True:
        i = idle()
        if i >= 85 or waited >= 7000:
            return i, waited
        time.sleep(15)
        waited += 16


def one(corpus, tool, phase, run, cmd, out):
    if shutil.disk_usage(OUTROOT).free < 2.5 * 2**30:
        sys.exit("STOP: less than 2.5 GB free on the disk")
    i, waited = quiet()
    log = S / "runs" / f"{corpus}.{tool}.{phase}.{run}.log"
    r = subprocess.run([sys.executable, str(S / "run1.py"), str(S / "corp" / corpus), str(log), "--"] + cmd + [str(S / "corp" / corpus)],
                       env={**ENV, "GRAPHIFY_OUT": str(out), "QUIET_LOAD": "999"}, capture_output=True, text=True)
    rec = json.loads(r.stdout.strip().splitlines()[-1])
    rec.update(corpus=corpus, tool=tool, phase=phase, run=run, out_dev="ram" if OUTROOT != S else "ssd", idle_before=i, idle_wait_s=waited,
               tail=log.read_text(errors="replace").strip().splitlines()[-1:][0:1])
    gj = out / "graph.json"
    rec["graph_mb"] = round(gj.stat().st_size / 2**20, 1) if gj.exists() else None
    if phase == "cold" and gj.exists():
        st = subprocess.run([sys.executable, str(S / "graphstats.py"), str(gj)], capture_output=True, text=True)
        try:
            s = json.loads(st.stdout)
            rec.update(nodes=s["nodes"], edges=s["edges"], communities=s["communities"], src_files=s["distinct_source_files"])
            (S / "runs" / f"{corpus}.{tool}.stats.{run}.json").write_text(st.stdout)
        except Exception as e:
            rec["stats_error"] = str(e)[:200]
    if phase == "nochange" and rec["exit"] == 0:
        du = subprocess.run(["du", "-sk", str(out)], capture_output=True, text=True).stdout.split()[0]
        rec["out_mb"] = round(int(du) / 1024, 1)
        snap = S / "snap" / corpus / f"{tool}.graph.json"
        if OUTROOT != S and not snap.exists():
            snap = OUTROOT / "snap" / corpus / f"{tool}.graph.json"
        if tool != "cgo" and not snap.exists():
            snap.parent.mkdir(parents=True, exist_ok=True)
            subprocess.run(["cp", "-c", str(gj), str(snap)])
    with open(S / "runs/results.jsonl", "a") as f:
        f.write(json.dumps(rec) + "\n")
    print(json.dumps({k: rec.get(k) for k in ("corpus", "tool", "phase", "run", "wall", "user", "sys", "rss_tree_mb", "rss_max_mb", "procs", "exit", "killed", "nodes", "edges", "idle_before", "other_cpu_pct")}), flush=True)
    return rec


done = set()
if (S / "runs/results.jsonl").exists():
    for l in open(S / "runs/results.jsonl"):
        r = json.loads(l)
        if r["phase"] == "nochange" or r["killed"] or r["exit"]:
            done.add((r["corpus"], r["tool"], r["run"]))
runs = int(sys.argv[1])
for corpus in sys.argv[2:]:
    dead = set()
    for run in range(1, runs + 1):
        for tool in ORDERS[(run - 1) % 3]:
            if tool in dead or (corpus, tool, run) in done:
                continue
            out = OUTROOT / "out" / corpus / f"zz-{tool}-7c2e"  # a neutral name: both tools skip scanned dirs named like the out dir
            shutil.rmtree(out, ignore_errors=True)
            build, update = TOOLS[tool]
            rec = one(corpus, tool, "cold", run, build, out)
            if rec["killed"] or rec["exit"] != 0:
                dead.add(tool)  # do not repeat a run that the watchdog stopped
                continue
            time.sleep(3)  # both tools distrust an mtime younger than 2 s
            one(corpus, tool, "nochange", run, update, out)
            shutil.rmtree(out, ignore_errors=True)  # the disk is nearly full
