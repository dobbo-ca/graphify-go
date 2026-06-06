// Package analyze derives the human-facing insights the report surfaces: the
// most-connected "god nodes", surprising cross-file connections, and file-level
// import cycles. It mirrors the intent of the Python original's analyze.py,
// trimmed to what the AST-only graph can support.
package analyze

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// GodNode is a highly-connected core abstraction.
type GodNode struct {
	ID     string
	Label  string
	Degree int
}

// GodNodes returns the topN most-connected real entities. File-hub nodes,
// external-dependency/concept nodes, and method stubs are excluded because they
// accumulate edges mechanically without being meaningful abstractions.
func GodNodes(g *model.Graph, topN int) []GodNode {
	ids := append([]string(nil), g.NodeIDs()...)
	sort.SliceStable(ids, func(i, j int) bool { return g.Degree(ids[i]) > g.Degree(ids[j]) })
	var out []GodNode
	for _, id := range ids {
		if isFileNode(g, id) || isConceptNode(g, id) {
			continue
		}
		out = append(out, GodNode{ID: id, Label: g.Nodes[id].Label, Degree: g.Degree(id)})
		if len(out) >= topN {
			break
		}
	}
	return out
}

// Surprise is a non-obvious cross-file connection.
type Surprise struct {
	Source, Target string
	Relation       string
	Confidence     string
	SourceFiles    [2]string
	Note           string
}

// Surprising returns up to topN cross-file edges between real entities, ranked
// by how non-obvious they are (AMBIGUOUS > INFERRED > EXTRACTED, with a bonus
// for bridging communities).
func Surprising(g *model.Graph, communities map[int][]string, topN int) []Surprise {
	nc := cluster.NodeCommunity(communities)
	confRank := map[string]int{"AMBIGUOUS": 3, "INFERRED": 2, "EXTRACTED": 1}

	type scored struct {
		s     Surprise
		score int
	}
	var cands []scored
	for _, e := range g.Edges() {
		if e.Relation == "imports" || e.Relation == "imports_from" || e.Relation == "contains" {
			continue
		}
		u, v := e.Source, e.Target
		if isConceptNode(g, u) || isConceptNode(g, v) || isFileNode(g, u) || isFileNode(g, v) {
			continue
		}
		uf, vf := g.Nodes[u].SourceFile, g.Nodes[v].SourceFile
		if uf == "" || vf == "" || uf == vf {
			continue
		}
		score := confRank[e.Confidence]
		note := ""
		if cu, cv := nc[u], nc[v]; cu != cv {
			score++
			note = "bridges separate communities"
		}
		cands = append(cands, scored{Surprise{
			Source: g.Nodes[u].Label, Target: g.Nodes[v].Label,
			Relation: e.Relation, Confidence: e.Confidence,
			SourceFiles: [2]string{uf, vf}, Note: note,
		}, score})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	var out []Surprise
	for _, c := range cands {
		if len(out) >= topN {
			break
		}
		out = append(out, c.s)
	}
	return out
}

// Cycle is a circular file-level import dependency.
type Cycle struct {
	Files []string
}

// ImportCycles finds circular dependencies in the file-level import graph built
// from imports_from edges. Cycles are bounded in length and deduplicated by
// rotation, shortest first.
func ImportCycles(g *model.Graph, maxLen, topN int) []Cycle {
	adj := map[string][]string{}
	for _, e := range g.Edges() {
		if e.Relation != "imports_from" {
			continue
		}
		uf, vf := g.Nodes[e.Source].SourceFile, g.Nodes[e.Target].SourceFile
		if uf == "" || vf == "" || uf == vf {
			continue
		}
		adj[uf] = append(adj[uf], vf)
	}
	seen := map[string]bool{}
	var cycles []Cycle
	starts := make([]string, 0, len(adj))
	for s := range adj {
		starts = append(starts, s)
	}
	sort.Strings(starts)

	var dfs func(start, cur string, path []string, visited map[string]bool)
	dfs = func(start, cur string, path []string, visited map[string]bool) {
		if len(path) > maxLen || len(cycles) >= topN*10 {
			return
		}
		nbrs := append([]string(nil), adj[cur]...)
		sort.Strings(nbrs)
		for _, nb := range nbrs {
			if nb == start && len(path) >= 2 {
				if key := rotateKey(path); !seen[key] {
					seen[key] = true
					cycles = append(cycles, Cycle{Files: append([]string(nil), path...)})
				}
				continue
			}
			if nb > start || visited[nb] { // only explore nodes >= start to avoid duplicate rotations
				continue
			}
			visited[nb] = true
			dfs(start, nb, append(path, nb), visited)
			visited[nb] = false
		}
	}
	for _, s := range starts {
		dfs(s, s, []string{s}, map[string]bool{s: true})
	}
	sort.SliceStable(cycles, func(i, j int) bool { return len(cycles[i].Files) < len(cycles[j].Files) })
	if len(cycles) > topN {
		cycles = cycles[:topN]
	}
	return cycles
}

func rotateKey(path []string) string {
	min, idx := path[0], 0
	for i, p := range path {
		if p < min {
			min, idx = p, i
		}
	}
	rot := append(append([]string(nil), path[idx:]...), path[:idx]...)
	return strings.Join(rot, "\x00")
}

func isFileNode(g *model.Graph, id string) bool {
	n := g.Nodes[id]
	if n.Label == "" {
		return false
	}
	if n.SourceFile != "" && n.Label == filepath.Base(n.SourceFile) {
		return true
	}
	if strings.HasPrefix(n.Label, ".") && strings.HasSuffix(n.Label, "()") {
		return true
	}
	if strings.HasSuffix(n.Label, "()") && g.Degree(id) <= 1 {
		return true
	}
	return false
}

func isConceptNode(g *model.Graph, id string) bool {
	src := g.Nodes[id].SourceFile
	if src == "" {
		return true
	}
	base := src
	if i := strings.LastIndex(src, "/"); i >= 0 {
		base = src[i+1:]
	}
	return !strings.Contains(base, ".")
}
