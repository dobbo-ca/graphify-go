#!/usr/bin/env python3
"""Interleaved cold build + no-change update for three tools on the aligned corpora.

usage: bench.py <runs> <corpus>...        appends to runs/results.jsonl
env:   BENCH_RESULTS  file name under runs/ (default results.jsonl)
       BENCH_TOOLS    comma list of tool keys, in the order of run 1 (default go,cgo,up)
       BENCH_PHASES   cold,nochange (default) or cold
       BENCH_DISK_BUDGET_GB  stop when the disk has lost this much free space since
                             bench.py started, whatever the cause (default 32)
       BENCH_SWAP_BUDGET_GB  stop when used swap has grown this much (default 8)
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
    # second run, after the vi5 gap work: old = f442d3f measured again in the same session
    "old": ([str(S / "bin/gfy-nocgo"), "build"], [str(S / "bin/gfy-nocgo"), "update"]),
    "new": ([str(S / "bin/gfy-9e292"), "build"], [str(S / "bin/gfy-9e292"), "update"]),
    "new-nc": ([str(S / "bin/gfy-9e292"), "build", "--no-cluster"], [str(S / "bin/gfy-9e292"), "update", "--no-cluster"]),
    "up-nc": ([str(S / "venv/bin/graphify"), "update", "--no-cluster"], [str(S / "venv/bin/graphify"), "update", "--no-cluster"]),
    "up-ld": ([str(S / "venv-leiden/bin/graphify"), "update"], [str(S / "venv-leiden/bin/graphify"), "update"]),
}
RESULTS = S / "runs" / os.environ.get("BENCH_RESULTS", "results.jsonl")
ORDER = os.environ.get("BENCH_TOOLS", "go,cgo,up").split(",")
PHASES = os.environ.get("BENCH_PHASES", "cold,nochange").split(",")
OUTROOT.mkdir(parents=True, exist_ok=True)
START_FREE = shutil.disk_usage(OUTROOT).free / 2**30
MIN_FREE = max(10.0, START_FREE - float(os.environ.get("BENCH_DISK_BUDGET_GB", "32")))
START_SWAP = float(subprocess.run(["sysctl", "-n", "vm.swapusage"], capture_output=True, text=True).stdout.split("used = ")[1].split("M")[0]) / 1024
MAX_SWAP = START_SWAP + float(os.environ.get("BENCH_SWAP_BUDGET_GB", "8"))
ENV = {k: v for k, v in os.environ.items() if not k.endswith("_API_KEY")}
ENV.update(GRAPHIFY_NO_AUTO_REFRESH="1", GRAPHIFY_NO_TIPS="1", GRAPHIFY_NO_BACKUP="1",
           GRAPHIFY_VIZ_NODE_LIMIT="0", GRAPHIFY_MAX_GRAPH_BYTES="8GB")


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
    if shutil.disk_usage(OUTROOT).free / 2**30 < MIN_FREE:
        sys.exit(f"STOP: free space below {MIN_FREE:.1f} GB (started at {START_FREE:.1f} GB)")
    i, waited = quiet()
    log = S / "runs" / f"{corpus}.{tool}.{phase}.{run}.log"
    r = subprocess.run([sys.executable, str(S / "run1.py"), str(S / "corp" / corpus), str(log), "--"] + cmd + [str(S / "corp" / corpus)],
                       env={**ENV, "GRAPHIFY_OUT": str(out), "QUIET_LOAD": "999",
                            "DISK_MIN_FREE_GB": str(MIN_FREE), "SWAP_MAX_GB": str(MAX_SWAP)}, capture_output=True, text=True)
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
    if phase == PHASES[-1] and rec["exit"] == 0:
        du = subprocess.run(["du", "-sk", str(out)], capture_output=True, text=True).stdout.split()[0]
        rec["out_mb"] = round(int(du) / 1024, 1)
        snap = S / "snap" / corpus / f"{tool}.graph.json"
        if OUTROOT != S and not snap.exists():
            snap = OUTROOT / "snap" / corpus / f"{tool}.graph.json"
        if tool in ("go", "up") and not snap.exists():
            snap.parent.mkdir(parents=True, exist_ok=True)
            subprocess.run(["cp", "-c", str(gj), str(snap)])
    with open(RESULTS, "a") as f:
        f.write(json.dumps(rec) + "\n")
    if rec["killed"] in ("disk_limit", "swap_limit"):
        shutil.rmtree(out, ignore_errors=True)
        sys.exit(f"STOP: {rec['killed']} in {corpus} {tool} {phase} {run}")
    print(json.dumps({k: rec.get(k) for k in ("corpus", "tool", "phase", "run", "wall", "user", "sys", "rss_tree_mb", "rss_max_mb", "procs", "exit", "killed", "nodes", "edges", "idle_before", "other_cpu_pct")}), flush=True)
    return rec


done = set()
if RESULTS.exists():
    for l in open(RESULTS):
        r = json.loads(l)
        if r["phase"] == PHASES[-1] or r["killed"] in ("rss_limit", "timeout") or (r["exit"] and not r["killed"]):
            done.add((r["corpus"], r["tool"], r["run"]))
runs = int(sys.argv[1])
for corpus in sys.argv[2:]:
    dead = set()
    for run in range(1, runs + 1):
        k = (run - 1) % len(ORDER)  # rotate: the last tool of a round starts the next round
        for tool in ORDER[-k:] + ORDER[:-k] if k else ORDER:
            if tool in dead or (corpus, tool, run) in done:
                continue
            out = OUTROOT / "out" / corpus / f"zz-{tool}-7c2e"  # a neutral name: both tools skip scanned dirs named like the out dir
            shutil.rmtree(out, ignore_errors=True)
            build, update = TOOLS[tool]
            rec = one(corpus, tool, "cold", run, build, out)
            if rec["killed"] or rec["exit"] != 0:
                dead.add(tool)  # do not repeat a run that the watchdog stopped
                continue
            if "nochange" not in PHASES:
                shutil.rmtree(out, ignore_errors=True)
                continue
            time.sleep(3)  # both tools distrust an mtime younger than 2 s
            one(corpus, tool, "nochange", run, update, out)
            shutil.rmtree(out, ignore_errors=True)  # the disk is nearly full
