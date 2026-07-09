package export

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// graphOfSize returns a graph with n distinct nodes and no edges.
func graphOfSize(n int) *model.Graph {
	g := model.New()
	for i := 0; i < n; i++ {
		id := "n" + strconv.Itoa(i)
		g.AddNode(model.Node{ID: id, Label: id, FileType: "code", SourceFile: "pkg/a.go"})
	}
	return g
}

// nodeCount reads a node-link graph.json and returns its node count.
func nodeCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out jsonGraph
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return len(out.Nodes)
}

func TestToJSONAntiShrink(t *testing.T) {
	comm := map[int][]string{}
	path := filepath.Join(t.TempDir(), "graph.json")

	// Seed disk with a 5-node graph.
	if err := ToJSON(graphOfSize(5), comm, path, "c1", false); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	if got := nodeCount(t, path); got != 5 {
		t.Fatalf("seed node count = %d, want 5", got)
	}

	// A smaller (2-node) graph is refused and the 5-node file is left intact.
	err := ToJSON(graphOfSize(2), comm, path, "c2", false)
	if !errors.Is(err, ErrGraphShrink) {
		t.Fatalf("shrink write err = %v, want ErrGraphShrink", err)
	}
	if got := nodeCount(t, path); got != 5 {
		t.Errorf("after refused shrink node count = %d, want 5 (file must be unchanged)", got)
	}

	// force=true overwrites even though it shrinks.
	if err := ToJSON(graphOfSize(2), comm, path, "c3", true); err != nil {
		t.Fatalf("forced shrink write: %v", err)
	}
	if got := nodeCount(t, path); got != 2 {
		t.Errorf("after forced shrink node count = %d, want 2", got)
	}

	// A larger graph is never refused.
	if err := ToJSON(graphOfSize(9), comm, path, "c4", false); err != nil {
		t.Fatalf("growth write: %v", err)
	}
	if got := nodeCount(t, path); got != 9 {
		t.Errorf("after growth node count = %d, want 9", got)
	}
}

func TestToJSONEmptyAndAbsentTargetWrites(t *testing.T) {
	comm := map[int][]string{}

	// Absent target: writes normally.
	absent := filepath.Join(t.TempDir(), "graph.json")
	if err := ToJSON(graphOfSize(3), comm, absent, "c1", false); err != nil {
		t.Fatalf("absent-target write: %v", err)
	}
	if got := nodeCount(t, absent); got != 3 {
		t.Errorf("absent-target node count = %d, want 3", got)
	}

	// Empty (whitespace-only) target: writes normally, no shrink refusal.
	empty := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(empty, []byte("   \n"), 0o644); err != nil {
		t.Fatalf("touch empty: %v", err)
	}
	if err := ToJSON(graphOfSize(1), comm, empty, "c2", false); err != nil {
		t.Fatalf("empty-target write: %v", err)
	}
	if got := nodeCount(t, empty); got != 1 {
		t.Errorf("empty-target node count = %d, want 1", got)
	}
}

func TestToJSONUnparseableExistingRefused(t *testing.T) {
	comm := map[int][]string{}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	err := ToJSON(graphOfSize(3), comm, path, "c1", false)
	if !errors.Is(err, ErrGraphUnverifiable) {
		t.Fatalf("unparseable-target err = %v, want ErrGraphUnverifiable", err)
	}
	// File is left untouched (still the corrupt bytes).
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if string(data) != "{ this is not json" {
		t.Errorf("corrupt file was modified: %q", data)
	}

	// force overwrites the corrupt file.
	if err := ToJSON(graphOfSize(3), comm, path, "c2", true); err != nil {
		t.Fatalf("forced overwrite of corrupt file: %v", err)
	}
	if got := nodeCount(t, path); got != 3 {
		t.Errorf("after forced overwrite node count = %d, want 3", got)
	}
}

func TestToJSON(t *testing.T) {
	g := model.New()
	g.AddNode(model.Node{ID: "fa", Label: "a.go", FileType: "code", SourceFile: "pkg/a.go", SourceLocation: "L1"})
	g.AddNode(model.Node{ID: "a1", Label: "Add()", FileType: "code", SourceFile: "pkg/a.go", SourceLocation: "L3"})
	g.AddEdge(model.Edge{Source: "fa", Target: "a1", Relation: "contains", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "a1", Target: "fa", Relation: "calls", Confidence: "INFERRED"})
	communities := map[int][]string{0: {"a1", "fa"}}

	path := filepath.Join(t.TempDir(), "graph.json")
	if err := ToJSON(g, communities, path, "deadbeefcafe", false); err != nil {
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
	if err := ToJSON(g, communities, path, "deadbeefcafe", false); err != nil {
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
