package cluster

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// twoTriangles builds two triangles (a1-a2-a3, b1-b2-b3) joined by a single
// bridge edge a1-b1 — a graph with clear community structure.
func twoTriangles() *model.Graph {
	g := model.New()
	for _, id := range []string{"a1", "a2", "a3", "b1", "b2", "b3"} {
		g.AddNode(model.Node{ID: id, Label: id, SourceFile: "pkg/" + id + ".go"})
	}
	edge := func(s, t string) {
		g.AddEdge(model.Edge{Source: s, Target: t, Relation: "calls"})
	}
	edge("a1", "a2")
	edge("a2", "a3")
	edge("a3", "a1")
	edge("b1", "b2")
	edge("b2", "b3")
	edge("b3", "b1")
	edge("a1", "b1") // bridge
	return g
}

func TestClusterEmpty(t *testing.T) {
	if got := Cluster(model.New()); len(got) != 0 {
		t.Errorf("Cluster(empty) = %v, want empty", got)
	}
}

func TestClusterCoversEveryNodeOnce(t *testing.T) {
	g := twoTriangles()
	communities := Cluster(g)
	if len(communities) == 0 {
		t.Fatal("Cluster returned no communities for a non-empty graph")
	}
	seen := map[string]int{}
	for _, nodes := range communities {
		for _, n := range nodes {
			seen[n]++
		}
	}
	if len(seen) != g.NumNodes() {
		t.Errorf("communities cover %d nodes, want %d", len(seen), g.NumNodes())
	}
	for id, count := range seen {
		if count != 1 {
			t.Errorf("node %q assigned to %d communities, want 1", id, count)
		}
	}
}

func TestCohesion(t *testing.T) {
	g := twoTriangles()
	// A full triangle has all 3 possible internal edges.
	if got := Cohesion(g, []string{"a1", "a2", "a3"}); got != 1.0 {
		t.Errorf("Cohesion(triangle) = %v, want 1.0", got)
	}
	// A single node is trivially cohesive.
	if got := Cohesion(g, []string{"a1"}); got != 1.0 {
		t.Errorf("Cohesion(single) = %v, want 1.0", got)
	}
	// Two unconnected nodes (a2 and b2 share no edge) have no internal edges.
	if got := Cohesion(g, []string{"a2", "b2"}); got != 0.0 {
		t.Errorf("Cohesion(unconnected pair) = %v, want 0.0", got)
	}
}

func TestNodeCommunityAndScores(t *testing.T) {
	g := twoTriangles()
	communities := Cluster(g)

	nc := NodeCommunity(communities)
	for cid, nodes := range communities {
		for _, n := range nodes {
			if nc[n] != cid {
				t.Errorf("NodeCommunity[%q] = %d, want %d", n, nc[n], cid)
			}
		}
	}

	scores := Scores(g, communities)
	if len(scores) != len(communities) {
		t.Errorf("Scores has %d entries, want %d", len(scores), len(communities))
	}
	for cid := range communities {
		if _, ok := scores[cid]; !ok {
			t.Errorf("Scores missing community %d", cid)
		}
	}
}

func TestClusterDeterministic(t *testing.T) {
	a := Cluster(twoTriangles())
	b := Cluster(twoTriangles())
	if len(a) != len(b) {
		t.Fatalf("community count not deterministic: %d vs %d", len(a), len(b))
	}
	for cid, nodes := range a {
		if len(nodes) != len(b[cid]) {
			t.Errorf("community %d differs across runs", cid)
			continue
		}
		for i := range nodes {
			if nodes[i] != b[cid][i] {
				t.Errorf("community %d member %d differs: %q vs %q", cid, i, nodes[i], b[cid][i])
			}
		}
	}
}
