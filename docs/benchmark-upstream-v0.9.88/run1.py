#!/usr/bin/env python3
"""Run one command under nice 19 with a memory watchdog; print one JSON line.

usage: run1.py <cwd> <logfile> -- cmd args...
env:   RSS_LIMIT_GB (default 40), TIMEOUT_S (default 5400)
       DISK_MIN_FREE_GB  stop the run when free space on the disk of GRAPHIFY_OUT
                         falls below this (default: free at start - 32, at least 10)
       SWAP_MAX_GB       stop the run when used swap goes above this
                         (default: used at start + 8)

wall     monotonic seconds around the child
user/sys os.wait4 rusage: the child plus every descendant it waited for
rss_max  ru_maxrss: the largest single process in the tree
rss_tree peak of the summed RSS of the whole process tree, sampled every 0.1 s
         (this is the number to compare when one tool uses a process pool)
disk_min_free_gb, swap_max_gb  lowest free space and highest used swap during the run
"""
import json, os, signal, subprocess, sys, threading, time

LIMIT = float(os.environ.get("RSS_LIMIT_GB", "40")) * 2**30
TIMEOUT = float(os.environ.get("TIMEOUT_S", "5400"))
GB = 2**30
DISK = os.environ.get("GRAPHIFY_OUT") or "."


def free_gb():
    p = os.path.abspath(DISK)
    while not os.path.exists(p):  # the tool makes the out dir later
        p = os.path.dirname(p)
    st = os.statvfs(p)
    return st.f_bavail * st.f_frsize / GB


def swap_gb():
    out = subprocess.run(["sysctl", "-n", "vm.swapusage"], capture_output=True, text=True).stdout
    return float(out.split("used = ")[1].split("M")[0]) / 1024  # "total = 3072.00M  used = 1999.88M ..."


def tree_rss(root):
    out = subprocess.run(["ps", "-Ao", "pid=,ppid=,rss=,pcpu="], capture_output=True, text=True).stdout
    kids, rss, cpu = {}, {}, {}
    for line in out.splitlines():
        p = line.split()
        if len(p) == 4:
            kids.setdefault(int(p[1]), []).append(int(p[0]))
            rss[int(p[0])] = int(p[2]) * 1024
            cpu[int(p[0])] = float(p[3])
    total, n, mine, stack = 0, 0, 0.0, [root]
    while stack:
        pid = stack.pop()
        if pid in rss:
            total += rss[pid]
            mine += cpu[pid]
            n += 1
        stack.extend(kids.get(pid, []))
    return total, n, sum(cpu.values()) - mine  # last: CPU % of every other process


def main():
    sep = sys.argv.index("--")
    cwd, log = sys.argv[1], sys.argv[2]
    cmd = sys.argv[sep + 1:]
    f0, s0 = free_gb(), swap_gb()
    min_free = float(os.environ.get("DISK_MIN_FREE_GB") or max(10.0, f0 - 32))
    max_swap = float(os.environ.get("SWAP_MAX_GB") or s0 + 8)
    with open(log, "w") as lf:
        t0 = time.monotonic()
        proc = subprocess.Popen(["nice", "-n", "19"] + cmd, cwd=cwd, stdout=lf, stderr=subprocess.STDOUT,
                                stdin=subprocess.DEVNULL, start_new_session=True)
        st = {"peak": 0, "procs": 0, "killed": "", "other": [], "done": False, "free": f0, "swap": s0}

        def sample():  # watchdog thread; the main thread blocks in wait4 for an exact wall time
            while not st["done"]:
                total, n, o = tree_rss(proc.pid)
                if st["done"]:
                    break
                st["other"].append(o)
                st["peak"], st["procs"] = max(st["peak"], total), max(st["procs"], n)
                st["free"] = min(st["free"], free_gb())
                if len(st["other"]) % 10 == 0:  # sysctl once a second
                    st["swap"] = max(st["swap"], swap_gb())
                if total > LIMIT:
                    st["killed"] = "rss_limit"
                elif st["free"] < min_free:
                    st["killed"] = "disk_limit"
                elif st["swap"] > max_swap:
                    st["killed"] = "swap_limit"
                elif time.monotonic() - t0 > TIMEOUT:
                    st["killed"] = "timeout"
                if st["killed"]:
                    os.killpg(proc.pid, signal.SIGKILL)
                    break
                time.sleep(0.1)

        th = threading.Thread(target=sample, daemon=True)
        th.start()
        _, status, ru = os.wait4(proc.pid, 0)
        wall = time.monotonic() - t0
        st["done"] = True
        th.join()
    peak, procs, killed, other = st["peak"], st["procs"], st["killed"], st["other"]
    print(json.dumps({
        "wall": round(wall, 3), "user": round(ru.ru_utime, 3), "sys": round(ru.ru_stime, 3),
        "rss_max_mb": round(ru.ru_maxrss / 2**20, 1), "rss_tree_mb": round(peak / 2**20, 1),
        "procs": procs, "exit": os.waitstatus_to_exitcode(status), "killed": killed,
        "other_cpu_pct": round(sum(other) / len(other), 0) if other else None,
        "disk_min_free_gb": round(st["free"], 1), "swap_max_gb": round(st["swap"], 1),
    }))


main()
