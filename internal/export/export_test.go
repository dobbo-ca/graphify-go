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

func TestCheckShrink(t *testing.T) {
	comm := map[int][]string{}
	nA := model.Node{ID: "a", Label: "a", FileType: "code", SourceFile: "pkg/a.go"}
	nB := model.Node{ID: "b", Label: "b", FileType: "code", SourceFile: "pkg/b.go"}
	nC := model.Node{ID: "c", Label: "c", FileType: "code", SourceFile: "pkg/c.go"}

	graphWith := func(nodes ...model.Node) *model.Graph {
		g := model.New()
		for _, n := range nodes {
			g.AddNode(n)
		}
		return g
	}
	// seed writes an on-disk graph.json of the given nodes and returns its path.
	seed := func(nodes ...model.Node) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "graph.json")
		if err := ToJSON(graphWith(nodes...), comm, path, "seed", true); err != nil {
			t.Fatalf("seed: %v", err)
		}
		return path
	}

	// root has none of the pkg/*.go source files on disk, so a lost node's source
	// reads as deleted (not excluded) unless a subtest creates it.
	root := t.TempDir()

	// Growth (or same size) is never a shrink, regardless of skipped.
	if err := CheckShrink(seed(nA, nB), graphWith(nA, nB, nC), map[string]bool{"pkg/b.go": true}, root, false); err != nil {
		t.Errorf("growth: %v, want nil", err)
	}

	// Fail closed (#1795): b left the corpus but its source still exists on disk,
	// so it was excluded (e.g. a new ignore rule), not deleted — refuse the shrink.
	excludedRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(excludedRoot, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(excludedRoot, "pkg", "b.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckShrink(seed(nA, nB), graphWith(nA), map[string]bool{}, excludedRoot, false); !errors.Is(err, ErrGraphShrink) {
		t.Errorf("excluded-but-present source err = %v, want ErrGraphShrink", err)
	}

	// b's file left the corpus and is absent from root's disk: a deletion — allowed.
	if err := CheckShrink(seed(nA, nB), graphWith(nA), map[string]bool{}, root, false); err != nil {
		t.Errorf("deletion (source gone from disk): %v, want nil", err)
	}

	// Deletion: b's file no longer current, so it never appears in skipped — allowed.
	if err := CheckShrink(seed(nA, nB), graphWith(nA), map[string]bool{"pkg/z.go": true}, root, false); err != nil {
		t.Errorf("deletion shrink: %v, want nil", err)
	}

	// Silent loss: b's source file is still present but was skipped this run — refuse.
	if err := CheckShrink(seed(nA, nB), graphWith(nA), map[string]bool{"pkg/b.go": true}, root, false); !errors.Is(err, ErrGraphShrink) {
		t.Errorf("skipped-file shrink err = %v, want ErrGraphShrink", err)
	}

	// force bypasses even a skipped-file shrink.
	if err := CheckShrink(seed(nA, nB), graphWith(nA), map[string]bool{"pkg/b.go": true}, root, true); err != nil {
		t.Errorf("force bypass: %v, want nil", err)
	}

	// Absent target: nothing to lose.
	if err := CheckShrink(filepath.Join(t.TempDir(), "graph.json"), graphWith(nA), nil, root, false); err != nil {
		t.Errorf("absent target: %v, want nil", err)
	}

	// Empty (whitespace-only) target: proceed even with a would-be skip set.
	empty := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(empty, []byte("  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckShrink(empty, graphWith(nA), map[string]bool{"pkg/b.go": true}, root, false); err != nil {
		t.Errorf("empty target: %v, want nil", err)
	}

	// Unparseable existing graph: fail safe.
	corrupt := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(corrupt, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckShrink(corrupt, graphWith(nA), nil, root, false); !errors.Is(err, ErrGraphUnverifiable) {
		t.Errorf("unparseable err = %v, want ErrGraphUnverifiable", err)
	}
}

// A node lost to an in-place rebuild (its source is still in the new graph) is a
// normal refactor and must be allowed even when the source file exists on disk —
// the currentSources carve-out must short-circuit before the on-disk check.
func TestCheckShrinkAllowsRebuildInPlace(t *testing.T) {
	comm := map[int][]string{}
	graphWith := func(nodes ...model.Node) *model.Graph {
		g := model.New()
		for _, n := range nodes {
			g.AddNode(n)
		}
		return g
	}
	a1 := model.Node{ID: "a1", Label: "a1", FileType: "code", SourceFile: "pkg/a.go"}
	a2 := model.Node{ID: "a2", Label: "a2", FileType: "code", SourceFile: "pkg/a.go"}

	path := filepath.Join(t.TempDir(), "graph.json")
	if err := ToJSON(graphWith(a1, a2), comm, path, "seed", true); err != nil {
		t.Fatalf("seed: %v", err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("package p"), 0o644); err != nil {
		t.Fatal(err)
	}
	// New graph dropped a2 but pkg/a.go is still built (a1 present) — in-place
	// refactor. Must be allowed despite pkg/a.go existing on disk.
	if err := CheckShrink(path, graphWith(a1), map[string]bool{}, root, false); err != nil {
		t.Errorf("rebuild-in-place shrink must be allowed with source on disk, got %v", err)
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

func TestToJSONEdgeWeight(t *testing.T) {
	g := model.New()
	g.AddNode(model.Node{ID: "a", Label: "a", FileType: "code", SourceFile: "pkg/a.go"})
	g.AddNode(model.Node{ID: "b", Label: "b", FileType: "code", SourceFile: "pkg/b.go"})
	// One edge carries a non-zero weight, the other leaves it unset.
	g.AddEdge(model.Edge{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED", Weight: 3.5})
	g.AddEdge(model.Edge{Source: "b", Target: "a", Relation: "calls", Confidence: "EXTRACTED"})

	path := filepath.Join(t.TempDir(), "graph.json")
	if err := ToJSON(g, map[int][]string{}, path, "c1", false); err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// A weighted edge serializes its weight; an unweighted edge omits the field.
	var out jsonGraph
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Links) != 2 {
		t.Fatalf("got %d links, want 2", len(out.Links))
	}
	for _, l := range out.Links {
		want := 0.0
		if l.Source == "a" {
			want = 3.5
		}
		if l.Weight != want {
			t.Errorf("link %s->%s weight = %v, want %v", l.Source, l.Target, l.Weight, want)
		}
	}

	// omitempty: the unweighted edge must not emit a "weight" key at all, so
	// existing consumers and round-trips are unaffected.
	var raw struct {
		Links []map[string]any `json:"links"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	for _, l := range raw.Links {
		_, hasWeight := l["weight"]
		if l["source"] == "a" && !hasWeight {
			t.Errorf("weighted edge is missing the weight key: %v", l)
		}
		if l["source"] == "b" && hasWeight {
			t.Errorf("unweighted edge must omit the weight key: %v", l)
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
