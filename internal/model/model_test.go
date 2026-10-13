package model

import (
	"math/rand"
	"reflect"
	"slices"
	"testing"
)

// Edges whose ids hold NUL collided under the joined dedup key of v0.10.1.
func TestAddEdgeNULIDsCollide(t *testing.T) {
	g := New()
	for _, id := range []string{"a", "\x00a", "a\x00"} {
		g.AddNode(Node{ID: id})
	}
	g.AddEdge(Edge{Source: "a", Target: "\x00a", Relation: Rel("r")})
	g.AddEdge(Edge{Source: "a\x00", Target: "a", Relation: Rel("r")})
	if n := len(g.Edges()); n != 1 {
		t.Fatalf("edges = %d, want 1", n)
	}
}

// Both directions, a second relation and a self-loop each count once.
func TestNeighborsAreDistinct(t *testing.T) {
	g := New()
	for _, id := range []string{"c", "a", "b"} {
		g.AddNode(Node{ID: id})
	}
	for _, e := range []Edge{{Source: "a", Target: "b", Relation: Rel("r")}, {Source: "b", Target: "a", Relation: Rel("r")},
		{Source: "a", Target: "b", Relation: Rel("s")}, {Source: "a", Target: "a", Relation: Rel("r")}, {Source: "c", Target: "a", Relation: Rel("r")}} {
		g.AddEdge(e)
	}
	if got := g.Neighbors("a"); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("Neighbors(a) = %v", got)
	}
	if g.Degree("a") != 3 || g.Degree("b") != 1 || g.Degree("x") != 0 {
		t.Fatalf("degrees = %d %d %d", g.Degree("a"), g.Degree("b"), g.Degree("x"))
	}
	if !g.HasEdge("b", "a") || !g.HasEdge("a", "a") || g.HasEdge("b", "c") || g.HasEdge("b", "x") {
		t.Fatal("HasEdge wrong")
	}
}

// AddEdgesAt must leave the graph AddEdge leaves, edge and neighbour order
// included, also when ids with NUL make different edges collide. It keeps an
// edge that names its nodes already where it is, copies the others, and
// writes to none.
func TestAddEdgesAtMatchesAddEdge(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	kept := 0
	for _, ids := range [][]string{{"a", "b", "c", "d", "e"}, {"a", "\x00a", "a\x00", "a\x00a", "b"}} {
		for round := 0; round < 200; round++ {
			one, bulk := New(), New()
			for _, id := range ids {
				one.AddNode(Node{ID: id})
				bulk.AddNode(Node{ID: id})
			}
			n := rng.Intn(40)
			edges, ptrs := make([]Edge, n), make([]*Edge, n)
			order := make([]int32, n)
			src, tgt := make([]uint32, n), make([]uint32, n)
			for i := range edges {
				src[i], tgt[i] = uint32(rng.Intn(len(ids))), uint32(rng.Intn(len(ids)))
				edges[i] = Edge{Source: "x", Target: "y", Relation: Rel([]string{"r", "s", "a\x00r"}[rng.Intn(3)]), Weight: float64(i)}
				if rng.Intn(2) == 0 {
					edges[i].Source = ids[src[i]]
				}
				if rng.Intn(2) == 0 {
					edges[i].Target = ids[tgt[i]]
				}
				order[i], ptrs[i] = int32(i), &edges[i]
			}
			before := slices.Clone(edges)
			rng.Shuffle(n, func(i, j int) { order[i], order[j] = order[j], order[i] })
			for _, i := range order {
				if rng.Intn(8) == 0 {
					src[i] = NoNode
					continue
				}
				e := edges[i]
				e.Source, e.Target = ids[src[i]], ids[tgt[i]]
				one.AddEdge(e)
			}
			bulk.AddEdgesAt(ptrs, order, src, tgt)
			if !slices.Equal(edges, before) {
				t.Fatal("AddEdgesAt wrote to the caller's edges")
			}
			for _, e := range bulk.edges {
				if slices.Contains(ptrs, e) {
					kept++
				}
			}
			if len(one.edges) != len(bulk.edges) {
				t.Fatalf("edges = %d, want %d", len(bulk.edges), len(one.edges))
			}
			for i := range one.edges {
				if *one.edges[i] != *bulk.edges[i] {
					t.Fatalf("edge %d = %+v, want %+v", i, *bulk.edges[i], *one.edges[i])
				}
			}
			if !reflect.DeepEqual(one.adj, bulk.adj) || !reflect.DeepEqual(one.pairs, bulk.pairs) {
				t.Fatalf("neighbours differ: %v, want %v", bulk.adj, one.adj)
			}
		}
	}
	if kept == 0 {
		t.Error("no edge was kept in place")
	}
}
