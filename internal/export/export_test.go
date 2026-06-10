package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

func TestToJSON(t *testing.T) {
	g := model.New()
	g.AddNode(model.Node{ID: "fa", Label: "a.go", FileType: "code", SourceFile: "pkg/a.go", SourceLocation: "L1"})
	g.AddNode(model.Node{ID: "a1", Label: "Add()", FileType: "code", SourceFile: "pkg/a.go", SourceLocation: "L3"})
	g.AddEdge(model.Edge{Source: "fa", Target: "a1", Relation: "contains", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "a1", Target: "fa", Relation: "calls", Confidence: "INFERRED"})
	communities := map[int][]string{0: {"a1", "fa"}}

	path := filepath.Join(t.TempDir(), "graph.json")
	if err := ToJSON(g, communities, path, "deadbeefcafe"); err != nil {
		t.Fatalf("ToJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var out jsonGraph
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Directed || out.Multigraph {
		t.Errorf("node-link graph should be undirected/simple, got directed=%v multigraph=%v", out.Directed, out.Multigraph)
	}
	if out.BuiltAtCommit != "deadbeefcafe" {
		t.Errorf("built_at_commit = %q, want deadbeefcafe", out.BuiltAtCommit)
	}
	if out.Hyperedges == nil || len(out.Hyperedges) != 0 {
		t.Errorf("hyperedges should serialize as an empty array, got %v", out.Hyperedges)
	}
	if len(out.Nodes) != 2 || len(out.Links) != 2 {
		t.Fatalf("got %d nodes / %d links, want 2 / 2", len(out.Nodes), len(out.Links))
	}

	// Every node carries its community and a norm_label.
	for _, n := range out.Nodes {
		if n.Community == nil || *n.Community != 0 {
			t.Errorf("node %q community = %v, want 0", n.ID, n.Community)
		}
		if n.NormLabel == "" {
			t.Errorf("node %q has empty norm_label", n.ID)
		}
	}
	// confidence_score is derived from confidence when unset on the edge.
	for _, l := range out.Links {
		switch l.Confidence {
		case "EXTRACTED":
			if l.ConfidenceScore != 1.0 {
				t.Errorf("EXTRACTED link score = %v, want 1.0", l.ConfidenceScore)
			}
		case "INFERRED":
			if l.ConfidenceScore != 0.5 {
				t.Errorf("INFERRED link score = %v, want 0.5", l.ConfidenceScore)
			}
		}
	}
}

func TestToJSONComputedName(t *testing.T) {
	g := model.New()
	g.AddNode(model.Node{ID: "m1", Label: "module.this [null-label]", FileType: "code", SourceFile: "main.tf", SourceLocation: "L1", ComputedName: "eg-prod-app"})
	communities := map[int][]string{0: {"m1"}}

	path := filepath.Join(t.TempDir(), "graph.json")
	if err := ToJSON(g, communities, path, "deadbeefcafe"); err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var out jsonGraph
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(out.Nodes))
	}
	n := out.Nodes[0]
	if n.ComputedName != "eg-prod-app" {
		t.Errorf("computed_name = %q, want eg-prod-app", n.ComputedName)
	}
	if !strings.Contains(n.NormLabel, "eg-prod-app") {
		t.Errorf("norm_label = %q, want it to contain eg-prod-app", n.NormLabel)
	}
}
