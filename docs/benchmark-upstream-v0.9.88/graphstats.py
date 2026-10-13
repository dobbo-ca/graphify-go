#!/usr/bin/env python3
"""Print comparable stats for one graph.json (upstream graphify or graphify-go).

usage: graphstats.py <graph.json>
Handles "links" (node-link) and "edges" (upstream raw --no-cluster) schemas.
"""
import json
import os
import sys
from collections import Counter


def _key(v):
    return v if isinstance(v, str) and v else "(missing)"


def stats(data):
    nodes = data.get("nodes") or []
    links_key = "links" if isinstance(data.get("links"), list) else "edges"
    links = data.get(links_key) or []
    files = {n["source_file"] for n in nodes
             if isinstance(n.get("source_file"), str) and n["source_file"]}
    exts = Counter(os.path.splitext(f)[1].lower() or "(none)" for f in files)
    comms = {n["community"] for n in nodes if n.get("community") is not None}
    by = lambda items, k: dict(Counter(_key(i.get(k)) for i in items).most_common())
    return {
        "nodes": len(nodes),
        "edges": len(links),
        "nodes_by_file_type": by(nodes, "file_type"),
        "edges_by_relation": by(links, "relation"),
        "edges_by_confidence": by(links, "confidence"),
        "communities": len(comms),
        "distinct_source_files": len(files),
        "source_files_by_extension": dict(exts.most_common(25)),
        # context needed to read the numbers above
        "nodes_without_source_file": len(nodes) - sum(
            1 for n in nodes if isinstance(n.get("source_file"), str) and n["source_file"]),
        "links_key": links_key,
        "directed": data.get("directed"),
        "multigraph": data.get("multigraph"),
    }


def _demo():
    a = {"nodes": [{"id": "a", "file_type": "code", "source_file": "x/A.GO", "community": 0},
                   {"id": "b", "file_type": "concept", "source_file": "", "community": None}],
         "links": [{"source": "a", "target": "b", "relation": "imports", "confidence": "EXTRACTED"}]}
    b = {"nodes": a["nodes"], "edges": a["links"]}  # upstream raw schema
    for d in (a, b):
        s = stats(d)
        assert (s["nodes"], s["edges"], s["communities"], s["distinct_source_files"]) == (2, 1, 1, 1), s
        assert s["source_files_by_extension"] == {".go": 1} and s["nodes_without_source_file"] == 1, s
    assert stats(b)["links_key"] == "edges"


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    if sys.argv[1] == "--selftest":
        _demo()
        sys.exit(0)
    with open(sys.argv[1], "rb") as f:
        print(json.dumps(stats(json.load(f)), indent=2, ensure_ascii=False))
