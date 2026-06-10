package query

import (
	"path/filepath"
	"sort"
)

// depRelations are the edge relations that mean "source depends on target", so
// reversing them finds the things impacted when the target changes.
var depRelations = map[string]bool{
	"calls": true, "imports": true, "imports_from": true,
	"references": true, "depends_on": true, "inherits_context": true,
}

// AffectedResult separates the symbols defined in the changed files from the
// symbols transitively impacted by them.
type AffectedResult struct {
	Changed  []Node
	Impacted []Node
}

// Affected returns the graph nodes defined in changedFiles ("changed") and every
// node that transitively depends on them ("impacted") — the blast radius of a
// change. Impact propagates backwards along dependency edges: a changed callee
// reaches its callers, a changed file reaches its importers.
func Affected(g *Graph, changedFiles []string) AffectedResult {
	want := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		want[filepath.ToSlash(f)] = true
	}

	// Reverse dependency adjacency: for "A depends on B" (A->B), record B->A so a
	// changed B reaches its dependent A.
	rev := map[string][]string{}
	for _, l := range g.Links {
		if depRelations[l.Relation] {
			rev[l.Target] = append(rev[l.Target], l.Source)
		}
	}

	changedSet := map[string]bool{}
	var seeds []string
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.SourceFile != "" && want[filepath.ToSlash(n.SourceFile)] {
			changedSet[n.ID] = true
			seeds = append(seeds, n.ID)
		}
	}

	// BFS backwards from the changed nodes to collect dependents.
	impactedSet := map[string]bool{}
	queue := append([]string(nil), seeds...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dep := range rev[cur] {
			if changedSet[dep] || impactedSet[dep] {
				continue
			}
			impactedSet[dep] = true
			queue = append(queue, dep)
		}
	}

	return AffectedResult{
		Changed:  g.collect(changedSet),
		Impacted: g.collect(impactedSet),
	}
}

// collect returns the nodes whose IDs are in set, sorted by source location then
// label for stable output.
func (g *Graph) collect(set map[string]bool) []Node {
	var out []Node
	for i := range g.Nodes {
		if set[g.Nodes[i].ID] {
			out = append(out, g.Nodes[i])
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceFile != out[j].SourceFile {
			return out[i].SourceFile < out[j].SourceFile
		}
		return out[i].Label < out[j].Label
	})
	return out
}
