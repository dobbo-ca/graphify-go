// Package cluster runs community detection (Louvain, via gonum) on the graph,
// then post-processes the result the way the Python original does: oversized
// communities are split, and IDs are assigned by descending size with a lexical
// tie-break so the same grouping always yields the same IDs across runs.
package cluster

import (
	"sort"

	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/graph/community"
	"gonum.org/v1/gonum/graph/simple"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

const (
	maxCommunityFraction = 0.25 // communities larger than this share of the graph are split
	minSplitSize         = 10   // ...but only if they have at least this many nodes
	seed                 = 42
)

// Cluster partitions g into communities, returning communityID -> sorted node IDs.
func Cluster(g *model.Graph) map[int][]string {
	if g.NumNodes() == 0 {
		return map[int][]string{}
	}
	groups := louvain(g, g.NodeIDs())

	// Split any community that dominates the graph.
	maxSize := minSplitSize
	if frac := int(float64(g.NumNodes()) * maxCommunityFraction); frac > maxSize {
		maxSize = frac
	}
	var final [][]string
	for _, nodes := range groups {
		if len(nodes) > maxSize {
			final = append(final, splitOversized(g, nodes)...)
		} else {
			final = append(final, nodes)
		}
	}
	return reindexBySize(final)
}

// louvain runs gonum's Louvain modularization over the induced subgraph on the
// given node set and returns the detected communities as string-ID groups.
func louvain(g *model.Graph, nodeSet []string) [][]string {
	idOf := make(map[string]int64, len(nodeSet))
	nameOf := make(map[int64]string, len(nodeSet))
	in := make(map[string]bool, len(nodeSet))
	gg := simple.NewUndirectedGraph()
	for i, id := range nodeSet {
		idOf[id] = int64(i)
		nameOf[int64(i)] = id
		in[id] = true
		gg.AddNode(simple.Node(int64(i)))
	}
	for _, e := range g.Edges() {
		if e.Source == e.Target || !in[e.Source] || !in[e.Target] {
			continue
		}
		f, t := idOf[e.Source], idOf[e.Target]
		if gg.HasEdgeBetween(f, t) {
			continue
		}
		gg.SetEdge(simple.Edge{F: simple.Node(f), T: simple.Node(t)})
	}

	reduced := community.Modularize(gg, 1.0, rand.NewSource(seed))
	var out [][]string
	for _, comm := range reduced.Communities() {
		group := make([]string, 0, len(comm))
		for _, n := range comm {
			group = append(group, nameOf[n.ID()])
		}
		out = append(out, group)
	}
	return out
}

// splitOversized runs a second Louvain pass on a community's subgraph. If it
// doesn't break apart (or has no internal edges), the community is returned
// unchanged so the caller doesn't lose nodes.
func splitOversized(g *model.Graph, nodes []string) [][]string {
	sub := louvain(g, nodes)
	if len(sub) <= 1 {
		return [][]string{nodes}
	}
	return sub
}

// reindexBySize sorts communities by descending size (lexical tie-break on
// sorted member IDs) and assigns sequential IDs, making the mapping reproducible.
func reindexBySize(groups [][]string) map[int][]string {
	for i := range groups {
		sort.Strings(groups[i])
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if len(groups[i]) != len(groups[j]) {
			return len(groups[i]) > len(groups[j])
		}
		return less(groups[i], groups[j])
	})
	out := make(map[int][]string, len(groups))
	for i, g := range groups {
		out[i] = g
	}
	return out
}

func less(a, b []string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// Cohesion is the ratio of actual intra-community edges to the maximum possible.
func Cohesion(g *model.Graph, nodes []string) float64 {
	n := len(nodes)
	if n <= 1 {
		return 1.0
	}
	in := make(map[string]bool, n)
	for _, id := range nodes {
		in[id] = true
	}
	actual := 0
	for _, id := range nodes {
		for _, nb := range g.Neighbors(id) {
			if in[nb] && id < nb {
				actual++
			}
		}
	}
	return float64(actual) / (float64(n) * float64(n-1) / 2)
}

// NodeCommunity inverts communities into node ID -> community ID.
func NodeCommunity(communities map[int][]string) map[string]int {
	m := map[string]int{}
	for cid, nodes := range communities {
		for _, n := range nodes {
			m[n] = cid
		}
	}
	return m
}

// Scores returns each community's cohesion score.
func Scores(g *model.Graph, communities map[int][]string) map[int]float64 {
	s := map[int]float64{}
	for cid, nodes := range communities {
		s[cid] = Cohesion(g, nodes)
	}
	return s
}
