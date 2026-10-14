#!/usr/bin/env python3
"""Build the aligned corpus: only files that BOTH tools extract by default.

usage: prep.py <name> <source-repo>
writes: corp/<name>/ (hardlinks), prep/<name>/files.json

Package manifests are added to the common file set. graphify-go reads them in a
separate pass, so they are not in its extraction cache, and without them Go
module imports do not resolve for either tool.
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

# Same rule as CollectManifests in internal/detect/detect.go: four file names,
# outside the directories graphify-go never enters and outside .gitignore.
MANIFESTS = {"go.mod", "pyproject.toml", "pom.xml", "Cargo.toml"}
SKIP_DIRS = {"node_modules", ".git", ".hg", ".svn", "dist", "build", "target", "out", "vendor", "coverage",
             "__snapshots__", ".next", ".nuxt", ".turbo", ".svelte-kit", ".cache", ".idea", ".vscode", ".worktrees",
             "graphify-out", ".graphify", ".terraform", ".nox", ".tox", "site-packages", "lib64", ".eggs",
             ".pytest_cache", ".mypy_cache", ".ruff_cache"}


def manifests():
    found = []
    for d, dirs, files in os.walk(src):
        dirs[:] = [x for x in dirs if x not in SKIP_DIRS and not x.endswith(".egg-info")
                   and not (x == "worktrees" and Path(d).name.startswith("."))
                   and not Path(d, x, ".git").is_file()]  # linked worktree or submodule
        found += [os.path.relpath(os.path.join(d, f), src) for f in files if f in MANIFESTS]
    ign = subprocess.run(["git", "-C", str(src), "check-ignore", "--stdin"], input="\n".join(found),
                         capture_output=True, text=True).stdout.splitlines()
    return sorted(set(found) - set(ign) - set(both))


added = manifests()


def by_ext(paths):
    c = collections.Counter(Path(p).suffix.lower() or "(none)" for p in paths)
    return dict(c.most_common(15))


def by_top(paths):
    c = collections.Counter(p.split("/")[0] if "/" in p else "(root)" for p in paths)
    return dict(c.most_common(10))


dst = S / "corp" / name
shutil.rmtree(dst, ignore_errors=True)
nbytes = 0
for p in both + added:
    t = dst / p
    t.parent.mkdir(parents=True, exist_ok=True)
    s = (src / p).resolve()
    os.link(s, t)
    if p not in added:  # both_mb counts the common files only
        nbytes += s.stat().st_size

info = {
    "corpus": name, "source": str(src),
    "commit": subprocess.run(["git", "-C", str(src), "rev-parse", "--short", "HEAD"], capture_output=True, text=True).stdout.strip(),
    "go_default": len(go), "up_default": len(up), "both": len(both), "both_mb": round(nbytes / 2**20, 1),
    "manifests_added": len(added), "aligned_total": len(both) + len(added),
    "up_detected": d["detected"],
    "only_go": len(go - up), "only_go_ext": by_ext(go - up), "only_go_top": by_top(go - up),
    "only_up": len(up - go), "only_up_ext": by_ext(up - go), "only_up_top": by_top(up - go),
    "both_ext": by_ext(both),
}
json.dump(info, open(prep / "files.json", "w"), indent=1)
json.dump(both, open(prep / "both.json", "w"))
print(json.dumps(info))
