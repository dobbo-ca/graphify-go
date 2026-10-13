#!/usr/bin/env python3
"""Build the aligned corpus: only files that BOTH tools extract by default.

usage: prep.py <name> <source-repo>
writes: corp/<name>/ (hardlinks), prep/<name>/files.json
"""
import collections, json, os, shutil, subprocess, sys
from pathlib import Path

S = Path(__file__).resolve().parent
name, src = sys.argv[1], Path(sys.argv[2]).resolve()
prep = S / "prep" / name
shutil.rmtree(prep, ignore_errors=True)
prep.mkdir(parents=True)
env = {**os.environ, "GRAPHIFY_NO_AUTO_REFRESH": "1", "GRAPHIFY_NO_TIPS": "1"}

# graphify-go default file set: the keys of the extraction cache after a build
goout = prep / "go-out"
r = subprocess.run(["nice", "-n", "19", str(S / "bin/gfy-nocgo"), "build", str(src)],
                   env={**env, "GRAPHIFY_OUT": str(goout)}, capture_output=True, text=True)
(prep / "go-build.log").write_text(r.stdout + r.stderr)
go = set(json.load(open(goout / ".graphify_cache.json"))["files"].keys())

# upstream default file set: detect(), then keep what `update` hands to an extractor
code = r"""
import sys, json
from pathlib import Path
from graphify.detect import detect
from graphify.extract import _get_extractor
root = Path(sys.argv[1])
d = detect(root, cache_root=Path(sys.argv[2]))
f = d['files']
keep = [x for x in f['code'] + f['document'] if _get_extractor(Path(x))]
print(json.dumps({'keep': keep, 'detected': {k: len(v) for k, v in f.items()},
                  'unclassified': len(d.get('unclassified', []))}))
"""
r = subprocess.run(["nice", "-n", "19", str(S / "venv/bin/python"), "-c", code, str(src), str(prep / "up-cache")],
                   env=env, capture_output=True, text=True, cwd=str(src))
(prep / "up-detect.log").write_text(r.stderr)
d = json.loads(r.stdout.strip().splitlines()[-1])


def rel(p):
    p = Path(p)
    return str(p.relative_to(src)) if p.is_absolute() else str(p)


up = {rel(p) for p in d["keep"]}
go = {rel(p) for p in go}
both = sorted(go & up)


def by_ext(paths):
    c = collections.Counter(Path(p).suffix.lower() or "(none)" for p in paths)
    return dict(c.most_common(15))


def by_top(paths):
    c = collections.Counter(p.split("/")[0] if "/" in p else "(root)" for p in paths)
    return dict(c.most_common(10))


dst = S / "corp" / name
shutil.rmtree(dst, ignore_errors=True)
nbytes = 0
for p in both:
    t = dst / p
    t.parent.mkdir(parents=True, exist_ok=True)
    s = (src / p).resolve()
    os.link(s, t)
    nbytes += s.stat().st_size

info = {
    "corpus": name, "source": str(src),
    "commit": subprocess.run(["git", "-C", str(src), "rev-parse", "--short", "HEAD"], capture_output=True, text=True).stdout.strip(),
    "go_default": len(go), "up_default": len(up), "both": len(both), "both_mb": round(nbytes / 2**20, 1),
    "up_detected": d["detected"],
    "only_go": len(go - up), "only_go_ext": by_ext(go - up), "only_go_top": by_top(go - up),
    "only_up": len(up - go), "only_up_ext": by_ext(up - go), "only_up_top": by_top(up - go),
    "both_ext": by_ext(both),
}
json.dump(info, open(prep / "files.json", "w"), indent=1)
json.dump(both, open(prep / "both.json", "w"))
print(json.dumps(info))
