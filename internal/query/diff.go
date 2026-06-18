package query

import (
	"fmt"
	"sort"
	"strings"
)

// DiffNode identifies a node added or removed between two snapshots.
type DiffNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// DiffEdge identifies an edge added or removed between two snapshots.
type DiffEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
}

// DiffResult is the realized node/edge delta between two graph snapshots: the
// before/after complement to Affected's predicted blast radius.
type DiffResult struct {
	NewNodes     []DiffNode `json:"new_nodes"`
	RemovedNodes []DiffNode `json:"removed_nodes"`
	NewEdges     []DiffEdge `json:"new_edges"`
	RemovedEdges []DiffEdge `json:"removed_edges"`
	Summary      string     `json:"summary"`
}

// Diff compares two loaded graph snapshots and returns the nodes and edges added
// or removed going from old to new. The graph is directed, so edges are compared
// by a direction-aware key (source, target, relation): reversing an edge counts
// as one removed and one added.
func Diff(old, new *Graph) DiffResult {
	oldNodes := nodeLabels(old)
	newNodes := nodeLabels(new)

	var res DiffResult
	for id, label := range newNodes {
		if _, ok := oldNodes[id]; !ok {
			res.NewNodes = append(res.NewNodes, DiffNode{ID: id, Label: label})
		}
	}
	for id, label := range oldNodes {
		if _, ok := newNodes[id]; !ok {
			res.RemovedNodes = append(res.RemovedNodes, DiffNode{ID: id, Label: label})
		}
	}

	oldEdges := edgeKeys(old)
	newEdges := edgeKeys(new)
	for k, e := range newEdges {
		if _, ok := oldEdges[k]; !ok {
			res.NewEdges = append(res.NewEdges, e)
		}
	}
	for k, e := range oldEdges {
		if _, ok := newEdges[k]; !ok {
			res.RemovedEdges = append(res.RemovedEdges, e)
		}
	}

	sortNodes(res.NewNodes)
	sortNodes(res.RemovedNodes)
	sortEdges(res.NewEdges)
	sortEdges(res.RemovedEdges)
	res.Summary = diffSummary(res)
	return res
}

func nodeLabels(g *Graph) map[string]string {
	m := make(map[string]string, len(g.Nodes))
	for i := range g.Nodes {
		n := &g.Nodes[i]
		label := n.Label
		if label == "" {
			label = n.ID
		}
		m[n.ID] = label
	}
	return m
}

// edgeKey identifies a directed edge by source, target, and relation. The graph
// is directed, so the endpoints are not sorted: A->B and B->A are distinct.
func edgeKey(source, target, relation string) string {
	return source + "\x00" + target + "\x00" + relation
}

func edgeKeys(g *Graph) map[string]DiffEdge {
	m := make(map[string]DiffEdge, len(g.Links))
	for _, l := range g.Links {
		k := edgeKey(l.Source, l.Target, l.Relation)
		if _, ok := m[k]; !ok {
			m[k] = DiffEdge{Source: l.Source, Target: l.Target, Relation: l.Relation}
		}
	}
	return m
}

func sortNodes(ns []DiffNode) {
	sort.Slice(ns, func(i, j int) bool {
		if ns[i].Label != ns[j].Label {
			return ns[i].Label < ns[j].Label
		}
		return ns[i].ID < ns[j].ID
	})
}

func sortEdges(es []DiffEdge) {
	sort.Slice(es, func(i, j int) bool {
		if es[i].Source != es[j].Source {
			return es[i].Source < es[j].Source
		}
		if es[i].Target != es[j].Target {
			return es[i].Target < es[j].Target
		}
		return es[i].Relation < es[j].Relation
	})
}

func diffSummary(r DiffResult) string {
	var parts []string
	if n := len(r.NewNodes); n > 0 {
		parts = append(parts, fmt.Sprintf("%d new %s", n, plural(n, "node")))
	}
	if n := len(r.NewEdges); n > 0 {
		parts = append(parts, fmt.Sprintf("%d new %s", n, plural(n, "edge")))
	}
	if n := len(r.RemovedNodes); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s removed", n, plural(n, "node")))
	}
	if n := len(r.RemovedEdges); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s removed", n, plural(n, "edge")))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
