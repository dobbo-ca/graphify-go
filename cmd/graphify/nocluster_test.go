package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestBuildNoCluster proves --no-cluster writes the raw extraction: every node
// lands with a null community, whereas a plain build over the same corpus assigns
// communities. Mirrors upstream `update --no-cluster`.
func TestBuildNoCluster(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package p\n\nfunc A() { B() }\n")
	write("b.go", "package p\n\nfunc B() {}\n")

	graphPath := filepath.Join(root, "graphify-out", "graph.json")

	// Plain build assigns communities: at least one node carries a non-null community.
	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build: %v", err)
	}
	if !anyNodeClustered(t, graphPath) {
		t.Fatal("plain build produced no community assignments; fixture cannot detect the difference")
	}

	// --no-cluster build: every node's community must be null.
	if err := cmdBuild([]string{root, "--no-cluster"}); err != nil {
		t.Fatalf("build --no-cluster: %v", err)
	}
	if anyNodeClustered(t, graphPath) {
		t.Error("--no-cluster build assigned a community; expected raw extraction with no clustering")
	}
}

// anyNodeClustered reports whether any node in graph.json has a non-null community.
func anyNodeClustered(t *testing.T, path string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var g struct {
		Nodes []struct {
			Community *int `json:"community"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	for _, n := range g.Nodes {
		if n.Community != nil {
			return true
		}
	}
	return false
}
