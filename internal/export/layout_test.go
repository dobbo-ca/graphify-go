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
