package query

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const sampleGraph = `{
  "directed": false, "multigraph": false, "graph": {},
  "nodes": [
    {"id":"x_a","label":"a()","file_type":"code","source_file":"x.go","source_location":"L1","community":0,"norm_label":"a()"},
    {"id":"x_b","label":"b()","file_type":"code","source_file":"x.go","source_location":"L5","community":0,"norm_label":"b()"},
    {"id":"y_c","label":"c()","file_type":"code","source_file":"y.go","source_location":"L1","community":1,"norm_label":"c()"}
  ],
  "links": [
    {"source":"x_a","target":"x_b","relation":"calls","confidence":"INFERRED"},
    {"source":"x_b","target":"y_c","relation":"calls","confidence":"INFERRED"}
  ]
}`

func loadSample(t *testing.T) *Graph {
	t.Helper()
	p := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(p, []byte(sampleGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestQueryMatches(t *testing.T) {
	g := loadSample(t)
	m, err := Query(g, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m[0].Label != "a()" {
		t.Fatalf("query a = %+v, want single a()", m)
	}
	if m[0].Location != "x.go:1" {
		t.Errorf("location = %q, want x.go:1", m[0].Location)
	}
}

func TestExplainNeighbors(t *testing.T) {
	g := loadSample(t)
	ex, err := Explain(g, "b()")
	if err != nil {
		t.Fatal(err)
	}
	// b is called by a (incoming) and calls c (outgoing).
	var inA, outC bool
	for _, n := range ex.Neighbors {
		if n.Label == "a()" && n.Direction == "<-" {
			inA = true
		}
		if n.Label == "c()" && n.Direction == "->" {
			outC = true
		}
	}
	if !inA || !outC {
		t.Errorf("neighbors wrong: %+v", ex.Neighbors)
	}
}

func TestPath(t *testing.T) {
	g := loadSample(t)
	p, err := Path(g, "a()", "c()")
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 3 || p[0].Label != "a()" || p[2].Label != "c()" {
		t.Fatalf("path = %+v, want a->b->c", p)
	}
}

func TestPathEdges(t *testing.T) {
	g := loadSample(t)
	res, err := PathEdges(g, "a()", "c()", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 3 || len(res.Edges) != 2 {
		t.Fatalf("got %d nodes / %d edges, want 3 / 2", len(res.Nodes), len(res.Edges))
	}
	for i, e := range res.Edges {
		if e.Relation != "calls" || e.Confidence != "INFERRED" || !e.Forward {
			t.Errorf("edge %d = %+v, want forward calls/INFERRED", i, e)
		}
	}
	// Reversed query orients both hops backwards against the stored edges.
	rev, err := PathEdges(g, "c()", "a()", 8)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range rev.Edges {
		if e.Forward {
			t.Errorf("edge %d should be backward: %+v", i, e)
		}
	}
}

func TestPathEdgesSameNode(t *testing.T) {
	g := loadSample(t)
	_, err := PathEdges(g, "a()", "a()", 8)
	var same *SameNodeError
	if !errors.As(err, &same) {
		t.Fatalf("err = %v, want *SameNodeError", err)
	}
	if same.ID != "x_a" {
		t.Errorf("resolved ID = %q, want x_a", same.ID)
	}
}

func TestPathEdgesMaxHops(t *testing.T) {
	g := loadSample(t)
	_, err := PathEdges(g, "a()", "c()", 1)
	var over *MaxHopsError
	if !errors.As(err, &over) {
		t.Fatalf("err = %v, want *MaxHopsError", err)
	}
	if over.MaxHops != 1 || over.Hops != 2 {
		t.Errorf("got max=%d hops=%d, want max=1 hops=2", over.MaxHops, over.Hops)
	}
}
