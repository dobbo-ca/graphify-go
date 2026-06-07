package export

import (
	"math"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// TestLayoutPositions checks the precomputed layout is sane (finite, spread out,
// one position per node) and deterministic (stable graph.html across rebuilds).
func TestLayoutPositions(t *testing.T) {
	g := model.New()
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		g.AddNode(model.Node{ID: id, Label: id, SourceFile: "x.go"})
	}
	g.AddEdge(model.Edge{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "b", Target: "c", Relation: "calls", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "c", Target: "d", Relation: "calls", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "d", Target: "e", Relation: "calls", Confidence: "EXTRACTED"})
	nc := cluster.NodeCommunity(cluster.Cluster(g))

	p := layoutPositions(g, nc)
	if len(p) != g.NumNodes() {
		t.Fatalf("positions = %d, want %d", len(p), g.NumNodes())
	}
	var minX, minY, maxX, maxY = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for id, xy := range p {
		if math.IsNaN(xy.X) || math.IsNaN(xy.Y) || math.IsInf(xy.X, 0) || math.IsInf(xy.Y, 0) {
			t.Fatalf("node %s has non-finite position %+v", id, xy)
		}
		minX, maxX = math.Min(minX, xy.X), math.Max(maxX, xy.X)
		minY, maxY = math.Min(minY, xy.Y), math.Max(maxY, xy.Y)
	}
	if maxX-minX < 1 || maxY-minY < 1 {
		t.Errorf("layout collapsed: bbox %.2f x %.2f", maxX-minX, maxY-minY)
	}

	// Deterministic: a second run must match exactly (committed graph.html stability).
	p2 := layoutPositions(g, nc)
	for id, xy := range p {
		if p2[id] != xy {
			t.Fatalf("non-deterministic layout for %s: %+v vs %+v", id, xy, p2[id])
		}
	}
}

// TestBarnesHutExact checks the quadtree repulsion equals brute force when the
// opening angle is 0 (every cell is opened), validating the tree math.
func TestBarnesHutExact(t *testing.T) {
	rng := newTestRand()
	const n = 200
	Px := make([]float64, n)
	Py := make([]float64, n)
	mass := make([]float64, n)
	for i := range Px {
		Px[i], Py[i], mass[i] = rng()*1000, rng()*1000, 1+rng()*4
	}
	root := buildQuad(Px, Py, mass)
	for i := 0; i < n; i++ {
		// brute force on body i
		var bfx, bfy float64
		for j := 0; j < n; j++ {
			if j == i {
				continue
			}
			dx, dy := Px[i]-Px[j], Py[i]-Py[j]
			d2 := dx*dx + dy*dy
			if d2 < 1e-4 {
				continue
			}
			f := mass[i] * repK * mass[j] / d2
			bfx += dx * f
			bfy += dy * f
		}
		tfx, tfy := root.force(Px[i], Py[i], mass[i], 0) // theta=0 → exact
		if math.Abs(tfx-bfx) > 1e-6 || math.Abs(tfy-bfy) > 1e-6 {
			t.Fatalf("node %d: tree (%.6f,%.6f) != brute (%.6f,%.6f)", i, tfx, tfy, bfx, bfy)
		}
	}
}

// newTestRand returns a deterministic [0,1) generator (avoids importing rand in
// two forms; keeps the test reproducible).
func newTestRand() func() float64 {
	s := uint64(88172645463325252)
	return func() float64 {
		s ^= s << 13
		s ^= s >> 7
		s ^= s << 17
		return float64(s%1_000_000) / 1_000_000
	}
}
