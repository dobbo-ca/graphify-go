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

func TestRemapToPreviousEmpty(t *testing.T) {
	if got := RemapToPrevious(map[int][]string{}, map[string]int{"a": 0}); len(got) != 0 {
		t.Errorf("RemapToPrevious(empty) = %v, want empty", got)
	}
}

// nodeCommunityOf inverts a communities map into node -> community ID.
func nodeCommunityOf(communities map[int][]string) map[string]int {
	m := map[string]int{}
	for cid, nodes := range communities {
		for _, n := range nodes {
			m[n] = cid
		}
	}
	return m
}

func TestRemapToPreviousStableWhenUnchanged(t *testing.T) {
	// Same grouping, but the new run permuted the integer IDs.
	prev := map[int][]string{0: {"a1", "a2", "a3"}, 1: {"b1", "b2"}}
	current := map[int][]string{0: {"b1", "b2"}, 1: {"a1", "a2", "a3"}}

	got := RemapToPrevious(current, nodeCommunityOf(prev))

	// After remap, IDs should line up with prev exactly.
	if !equalMembers(got[0], []string{"a1", "a2", "a3"}) {
		t.Errorf("community 0 = %v, want a1,a2,a3", got[0])
	}
	if !equalMembers(got[1], []string{"b1", "b2"}) {
		t.Errorf("community 1 = %v, want b1,b2", got[1])
	}
}

func TestRemapToPreviousGreedyByOverlap(t *testing.T) {
	// Old community 0 = {a1,a2,a3}, 1 = {b1,b2}.
	prev := map[int][]string{0: {"a1", "a2", "a3"}, 1: {"b1", "b2"}}
	// New community 5 overlaps old-0 most (a1,a2); new 9 overlaps old-1 most (b1,b2).
	current := map[int][]string{5: {"a1", "a2"}, 9: {"b1", "b2", "a3"}}

	got := RemapToPrevious(current, nodeCommunityOf(prev))

	if !equalMembers(got[0], []string{"a1", "a2"}) {
		t.Errorf("expected new {a1,a2} to inherit old ID 0, got[0]=%v", got[0])
	}
	if !equalMembers(got[1], []string{"a3", "b1", "b2"}) {
		t.Errorf("expected new {b1,b2,a3} to inherit old ID 1, got[1]=%v", got[1])
	}
}

func TestRemapToPreviousFreshIDsAvoidCollisions(t *testing.T) {
	// One new community matches old ID 0; an unmatched one must NOT reuse 0.
	prev := map[int][]string{0: {"a1", "a2"}}
	current := map[int][]string{7: {"a1", "a2"}, 8: {"c1", "c2", "c3"}}

	got := RemapToPrevious(current, nodeCommunityOf(prev))

	if !equalMembers(got[0], []string{"a1", "a2"}) {
		t.Errorf("matched community should keep old ID 0, got[0]=%v", got[0])
	}
	// The unmatched community gets the next free ID (1), not 0.
	if !equalMembers(got[1], []string{"c1", "c2", "c3"}) {
		t.Errorf("unmatched community should get fresh ID 1, got[1]=%v", got[1])
	}
	if len(got) != 2 {
		t.Errorf("got %d communities, want 2 (no collision/overwrite)", len(got))
	}
}

func TestRemapToPreviousNoPreviousIsDeterministic(t *testing.T) {
	// With no previous assignment, all IDs are fresh, ordered size desc then
	// lexically — matching reindexBySize's contract.
	current := map[int][]string{3: {"b"}, 7: {"a"}, 1: {"x", "y", "z"}}

	got := RemapToPrevious(current, map[string]int{})

	if !equalMembers(got[0], []string{"x", "y", "z"}) {
		t.Errorf("largest community should be ID 0, got[0]=%v", got[0])
	}
	// Single-node {a} and {b} tie on size; lexical tie-break puts {a} first.
	if !equalMembers(got[1], []string{"a"}) {
		t.Errorf("expected {a} at ID 1, got[1]=%v", got[1])
	}
	if !equalMembers(got[2], []string{"b"}) {
		t.Errorf("expected {b} at ID 2, got[2]=%v", got[2])
	}
}

// equalMembers reports whether two node slices contain the same members,
// ignoring order.
func equalMembers(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		seen[x]--
	}
	for _, c := range seen {
		if c != 0 {
			return false
		}
	}
	return true
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
