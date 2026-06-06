// Package graph assembles extraction output into the undirected knowledge graph
// the rest of the pipeline operates on. It mirrors the Python original's
// build_from_json: deterministic edge ordering, ID-normalization remap for edge
// endpoints, dropping of dangling and phantom cross-language inferred calls, and
// first-seen-direction-wins for parallel edges.
package graph

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// langFamily groups file extensions that share a runtime. Extensions in
// different families cannot legitimately have an inferred call between them, so
// such edges are phantom (label collisions across languages) and dropped.
var langFamily = map[string]string{
	".go": "go", ".rs": "rust",
	".js": "js", ".jsx": "js", ".mjs": "js", ".cjs": "js", ".ts": "js", ".tsx": "js",
	".py": "py",
}

// Build assembles an extraction into a graph.
func Build(ext model.Extraction) *model.Graph {
	g := model.New()
	for _, n := range ext.Nodes {
		g.AddNode(n)
	}

	// Map a normalized form of every node ID back to the real ID, so an edge
	// endpoint that differs only in punctuation/casing still connects.
	normToID := make(map[string]string, g.NumNodes())
	for _, id := range g.NodeIDs() {
		normToID[idutil.NormalizeID(id)] = id
	}

	edges := append([]model.Edge(nil), ext.Edges...)
	sort.SliceStable(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Relation < b.Relation
	})

	for _, e := range edges {
		src, tgt := e.Source, e.Target
		if g.Nodes[src] == nil {
			if id, ok := normToID[idutil.NormalizeID(src)]; ok {
				src = id
			}
		}
		if g.Nodes[tgt] == nil {
			if id, ok := normToID[idutil.NormalizeID(tgt)]; ok {
				tgt = id
			}
		}
		if g.Nodes[src] == nil || g.Nodes[tgt] == nil {
			continue // dangling edge to an external/stdlib node — expected
		}
		if e.Relation == "calls" && e.Confidence == "INFERRED" && crossLanguage(g, src, tgt) {
			continue
		}
		e.Source, e.Target = src, tgt
		g.AddEdge(e)
	}
	return g
}

func crossLanguage(g *model.Graph, src, tgt string) bool {
	fa := langFamily[strings.ToLower(filepath.Ext(g.Nodes[src].SourceFile))]
	fb := langFamily[strings.ToLower(filepath.Ext(g.Nodes[tgt].SourceFile))]
	return fa != "" && fb != "" && fa != fb
}
