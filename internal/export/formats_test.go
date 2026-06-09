package export

import (
	"encoding/csv"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleGraphJSON has labels with XML/DOT/CSV metacharacters to exercise escaping.
const sampleGraphJSON = `{"directed":true,"multigraph":false,"graph":{},
"nodes":[
 {"id":"a","label":"A()","file_type":"code","source_file":"a.go","source_location":"L1","community":0,"norm_label":"a()"},
 {"id":"b","label":"B<x> \"q\"","file_type":"code","source_file":"b.go","community":1,"norm_label":"b"}
],
"links":[
 {"source":"a","target":"b","relation":"calls","confidence":"INFERRED","confidence_score":0.5}
],
"hyperedges":[]}`

func writeSample(t *testing.T) (dir, jsonPath string) {
	t.Helper()
	dir = t.TempDir()
	jsonPath = filepath.Join(dir, "graph.json")
	if err := os.WriteFile(jsonPath, []byte(sampleGraphJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, jsonPath
}

func TestGraphMLFromJSON(t *testing.T) {
	dir, jsonPath := writeSample(t)
	out := filepath.Join(dir, "graph.graphml")
	if err := GraphMLFromJSON(jsonPath, out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)

	var doc graphml
	if err := xml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}
	if len(doc.Graph.Nodes) != 2 || len(doc.Graph.Edges) != 1 {
		t.Fatalf("got %d nodes / %d edges, want 2 / 1", len(doc.Graph.Nodes), len(doc.Graph.Edges))
	}
	// The metacharacter label must round-trip through XML escaping.
	var bLabel string
	for _, n := range doc.Graph.Nodes {
		if n.ID == "b" {
			for _, d := range n.Data {
				if d.Key == "label" {
					bLabel = d.Val
				}
			}
		}
	}
	if bLabel != `B<x> "q"` {
		t.Errorf("label round-trip: got %q, want %q", bLabel, `B<x> "q"`)
	}
}

func TestDOTFromJSON(t *testing.T) {
	dir, jsonPath := writeSample(t)
	out := filepath.Join(dir, "graph.dot")
	if err := DOTFromJSON(jsonPath, out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	s := string(data)
	for _, want := range []string{
		"digraph graphify {",
		`"a" [label="A()"];`,
		`"a" -> "b" [label="calls"];`,
		`\"q\"`, // the quote in b's label is escaped
	} {
		if !strings.Contains(s, want) {
			t.Errorf("DOT output missing %q\n%s", want, s)
		}
	}
}

func TestCSVFromJSON(t *testing.T) {
	dir, jsonPath := writeSample(t)
	nodes := filepath.Join(dir, "graph.nodes.csv")
	edges := filepath.Join(dir, "graph.edges.csv")
	if err := CSVFromJSON(jsonPath, nodes, edges); err != nil {
		t.Fatal(err)
	}

	nf, _ := os.Open(nodes)
	defer nf.Close()
	rows, err := csv.NewReader(nf).ReadAll()
	if err != nil {
		t.Fatalf("nodes.csv not valid: %v", err)
	}
	if len(rows) != 3 { // header + 2 nodes
		t.Fatalf("nodes.csv has %d rows, want 3", len(rows))
	}
	if rows[0][0] != "id" || rows[0][5] != "community" {
		t.Errorf("unexpected header: %v", rows[0])
	}
	if rows[2][1] != `B<x> "q"` {
		t.Errorf("csv label round-trip: got %q", rows[2][1])
	}

	ef, _ := os.Open(edges)
	defer ef.Close()
	erows, err := csv.NewReader(ef).ReadAll()
	if err != nil {
		t.Fatalf("edges.csv not valid: %v", err)
	}
	if len(erows) != 2 || erows[1][2] != "calls" {
		t.Errorf("unexpected edges.csv: %v", erows)
	}
}
