// Package analyze derives the human-facing insights the report surfaces: the
// most-connected "god nodes", surprising cross-file connections, and file-level
// import cycles. It mirrors the intent of the Python original's analyze.py,
// trimmed to what the AST-only graph can support.
package analyze

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// GodNode is a highly-connected core abstraction.
type GodNode struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Degree int    `json:"degree"`
}

// GodNodes returns the topN most-connected real entities. File-hub nodes,
// external-dependency/concept nodes, and method stubs are excluded because they
// accumulate edges mechanically without being meaningful abstractions.
//
// excludeHubsPercentile (1-100) additionally suppresses nodes whose degree
// exceeds that percentile of the degree distribution, using the same threshold
// computation cluster() applies, so callers can look past the mega-hubs that
// dominate every ranking. Zero keeps the historical ranking.
func GodNodes(g *model.Graph, topN, excludeHubsPercentile int) []GodNode {
	ids := append([]string(nil), g.NodeIDs()...)
	hubThreshold, hasThreshold := 0, false
	if excludeHubsPercentile > 0 && len(ids) > 0 {
		degrees := make([]int, len(ids))
		for i, id := range ids {
			degrees[i] = g.Degree(id)
		}
		sort.Ints(degrees)
		idx := len(degrees) * excludeHubsPercentile / 100
		if idx > 0 {
			idx--
		}
		if idx >= len(degrees) {
			idx = len(degrees) - 1
		}
		hubThreshold, hasThreshold = degrees[idx], true
	}
	sort.SliceStable(ids, func(i, j int) bool { return g.Degree(ids[i]) > g.Degree(ids[j]) })
	var out []GodNode
	for _, id := range ids {
		if hasThreshold && g.Degree(id) > hubThreshold {
			continue
		}
		if isFileNode(g, id) || isConceptNode(g, id) || isJSONKeyNode(g, id) {
			continue
		}
		out = append(out, GodNode{ID: id, Label: g.Nodes[id].Label, Degree: g.Degree(id)})
		if len(out) >= topN {
			break
		}
	}
	return out
}

// jsonNoiseLabels are generic JSON keys (package.json, schemas) that accumulate
// edges mechanically without naming an abstraction.
var jsonNoiseLabels = map[string]bool{
	"start": true, "end": true, "name": true, "id": true, "type": true,
	"properties": true, "value": true, "key": true, "data": true, "items": true,
	"title": true, "description": true, "version": true, "dependencies": true,
	"devdependencies": true, "peerdependencies": true, "optionaldependencies": true,
	"bundleddependencies": true, "bundledependencies": true,
}

// isJSONKeyNode reports whether a node is a generic key inside a .json file.
func isJSONKeyNode(g *model.Graph, id string) bool {
	n := g.Nodes[id]
	if !strings.HasSuffix(strings.ToLower(n.SourceFile), ".json") {
		return false
	}
	return jsonNoiseLabels[strings.ToLower(strings.TrimSpace(n.Label))]
}

// semanticRelations are the LLM-inferred edge relations the enrichment stage
// emits. They are treated as insights in their own right by Surprising.
var semanticRelations = map[string]bool{
	"cites":                   true,
	"conceptually_related_to": true,
	"semantically_similar_to": true,
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
		// A semantic edge (LLM-inferred relation into a concept) is itself the
		// insight: a prose note linking to a concept is meaningful even when the
		// note looks like a file hub (its label is the file basename). So the
		// file-hub exclusion, which suppresses mechanical AST edges, is skipped
		// for semantic edges; the extract-concept exclusion still applies.
		sem := semanticRelations[e.Relation]
		if isConceptNode(g, u) || isConceptNode(g, v) {
			continue
		}
		if !sem && (isFileNode(g, u) || isFileNode(g, v)) {
			continue
		}
		uf, vf := entityLoc(g, u), entityLoc(g, v)
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
	raw := map[string][]string{}
	for _, e := range g.Edges() {
		// Type-only imports are erased at compile time, so they cannot form a
		// runtime cycle (upstream find_import_cycles skips them too).
		if e.Relation != "imports_from" || e.TypeOnly {
			continue
		}
		uf, vf := g.Nodes[e.Source].SourceFile, g.Nodes[e.Target].SourceFile
		if uf == "" || vf == "" || uf == vf {
			continue
		}
		raw[uf] = append(raw[uf], vf)
	}
	// Files become ints in sorted order, so int order is name order.
	names := make([]string, 0, len(raw))
	for s, vs := range raw {
		names = append(names, s)
		names = append(names, vs...)
	}
	sort.Strings(names)
	names = slices.Compact(names)
	idx := make(map[string]int32, len(names))
	for i, s := range names {
		idx[s] = int32(i)
	}
	n := len(names)
	adj, radj := make([][]int32, n), make([][]int32, n)
	for s, vs := range raw {
		u := idx[s]
		for _, v := range vs {
			adj[u] = append(adj[u], idx[v])
		}
		slices.Sort(adj[u])
		adj[u] = slices.Compact(adj[u])
		for _, v := range adj[u] {
			radj[v] = append(radj[v], u)
		}
	}
	comp := scc(adj)

	var cycles []Cycle
	visited := make([]bool, n)
	dist := make([]int32, n) // edges back to start, valid when stamp matches
	stamp := make([]int32, n)
	var queue, path []int32
	var start int32
	var dfs func(cur int32)
	dfs = func(cur int32) {
		if len(path) > maxLen || len(cycles) >= topN*10 {
			return
		}
		for _, nb := range adj[cur] {
			if nb == start {
				if len(path) >= 2 {
					files := make([]string, len(path))
					for i, p := range path {
						files[i] = names[p]
					}
					cycles = append(cycles, Cycle{Files: files})
				}
				continue
			}
			// The start is the largest file of its cycle, so each rotation is
			// found once. Skip nodes that cannot close the cycle within maxLen.
			if nb > start || visited[nb] || stamp[nb] != start+1 || len(path)+int(dist[nb]) > maxLen {
				continue
			}
			visited[nb] = true
			path = append(path, nb)
			dfs(nb)
			path = path[:len(path)-1]
			visited[nb] = false
		}
	}
	for s := 0; s < n && len(cycles) < topN*10; s++ {
		start = int32(s)
		// Reverse BFS inside the start's component: a cycle never leaves it.
		queue = append(queue[:0], start)
		stamp[s], dist[s] = start+1, 0
		for h := 0; h < len(queue); h++ {
			v := queue[h]
			if int(dist[v]) >= maxLen {
				continue
			}
			for _, u := range radj[v] {
				if u < start && comp[u] == comp[s] && stamp[u] != start+1 {
					stamp[u], dist[u] = start+1, dist[v]+1
					queue = append(queue, u)
				}
			}
		}
		if len(queue) == 1 {
			continue
		}
		visited[s] = true
		path = append(path[:0], start)
		dfs(start)
		visited[s] = false
	}
	sort.SliceStable(cycles, func(i, j int) bool { return len(cycles[i].Files) < len(cycles[j].Files) })
	if len(cycles) > topN {
		cycles = cycles[:topN]
	}
	return cycles
}

// scc is Tarjan's algorithm with an explicit stack; it returns a component id
// per node.
func scc(adj [][]int32) []int32 {
	n := len(adj)
	comp, low, num := make([]int32, n), make([]int32, n), make([]int32, n)
	for i := range comp {
		comp[i] = -1
	}
	var st []int32
	type frame struct{ v, i int32 }
	var cnt, nc int32
	for r := 0; r < n; r++ {
		if num[r] != 0 {
			continue
		}
		cnt++
		num[r], low[r] = cnt, cnt
		st = append(st, int32(r))
		call := []frame{{int32(r), 0}}
		for len(call) > 0 {
			f := &call[len(call)-1]
			v := f.v
			if int(f.i) < len(adj[v]) {
				w := adj[v][f.i]
				f.i++
				if num[w] == 0 {
					cnt++
					num[w], low[w] = cnt, cnt
					st = append(st, w)
					call = append(call, frame{w, 0})
				} else if comp[w] == -1 && num[w] < low[v] {
					low[v] = num[w]
				}
				continue
			}
			call = call[:len(call)-1]
			if len(call) > 0 {
				if p := call[len(call)-1].v; low[v] < low[p] {
					low[p] = low[v]
				}
			}
			if low[v] == num[v] {
				for {
					w := st[len(st)-1]
					st = st[:len(st)-1]
					comp[w] = nc
					if w == v {
						break
					}
				}
				nc++
			}
		}
	}
	return comp
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

// semanticEntityTypes are LLM-derived node file_types that, despite having no
// source file, are meaningful abstractions — a concept linked to many notes is a
// core abstraction, not a mechanical external-dependency stub. They are analysed
// as real entities (eligible for god nodes and surprising connections). Corpora
// built without --semantic never carry these types, so this never changes their
// output.
var semanticEntityTypes = map[string]bool{
	"semantic_concept": true,
	"rationale":        true,
}

// entityLoc returns a "where" key for the surprising-connection cross-file
// check. For a normal node that is its source file; for a semantic entity
// (which has none) it is the node id, so a note→concept edge is treated as
// cross-location and an edge between two distinct concepts counts as well.
func entityLoc(g *model.Graph, id string) string {
	n := g.Nodes[id]
	if n.SourceFile != "" {
		return n.SourceFile
	}
	if semanticEntityTypes[n.FileType] {
		return "concept:" + id
	}
	return ""
}

func isConceptNode(g *model.Graph, id string) bool {
	if semanticEntityTypes[g.Nodes[id].FileType] {
		return false
	}
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
