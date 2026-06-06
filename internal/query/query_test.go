package query

import (
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
