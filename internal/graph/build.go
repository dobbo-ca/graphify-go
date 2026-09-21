// Package graph assembles extraction output into the undirected knowledge graph
// the rest of the pipeline operates on. It mirrors the Python original's
// build_from_json: deterministic edge ordering, ID-normalization remap for edge
// endpoints, dropping of dangling and phantom cross-language inferred calls, and
// first-seen-direction-wins for parallel edges.
package graph

import (
	"sort"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/langfamily"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// Build assembles an extraction into a graph.
func Build(ext model.Extraction) *model.Graph {
	g := model.New()
	for _, n := range ext.Nodes {
		addNodeKeepingProvenance(g, n)
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
		switch e.Relation {
		case "calls":
			if e.Confidence == "INFERRED" && crossLanguage(g, src, tgt) {
				continue
			}
		case "imports", "imports_from", "references":
			// Same phantom-collision guard as calls, but EXTRACTED: an
			// unresolved `import time` must not bind by bare stem onto a
			// `time.ts` across a language boundary (#1749). crossLanguage is
			// false when either endpoint is a non-code node (empty family),
			// so config/manifest/md -> code references survive.
			if crossLanguage(g, src, tgt) {
				continue
			}
		}
		e.Source, e.Target = src, tgt
		g.AddEdge(e)
	}
	return g
}

// addNodeKeepingProvenance inserts n, but on an ID collision it keeps whichever
// record actually has provenance instead of plain last-write-wins: an external
// import node (empty SourceFile) must not erase the real file:line node it
// happens to collide with. When both records name the same source file, the
// incoming one inherits any attribute it left empty.
func addNodeKeepingProvenance(g *model.Graph, n model.Node) {
	prev := g.Nodes[n.ID]
	if prev == nil {
		g.AddNode(n)
		return
	}
	if n.SourceFile == "" && prev.SourceFile != "" {
		return
	}
	if n.SourceFile == prev.SourceFile {
		if n.Label == "" {
			n.Label = prev.Label
		}
		if n.FileType == "" {
			n.FileType = prev.FileType
		}
		if n.SourceLocation == "" {
			n.SourceLocation = prev.SourceLocation
		}
		if n.ComputedName == "" {
			n.ComputedName = prev.ComputedName
		}
	}
	g.AddNode(n)
}

func crossLanguage(g *model.Graph, src, tgt string) bool {
	return langfamily.Cross(g.Nodes[src].SourceFile, g.Nodes[tgt].SourceFile)
}
