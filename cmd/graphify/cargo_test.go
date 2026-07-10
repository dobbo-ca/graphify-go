package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBuildArgs(t *testing.T) {
	cases := []struct {
		args            []string
		wantRoot        string
		wantCargo       bool
		wantNoManifests bool
		wantForce       bool
		wantNoCluster   bool
	}{
		{nil, ".", false, false, false, false},
		{[]string{"--cargo"}, ".", true, false, false, false},
		{[]string{"some/path"}, "some/path", false, false, false, false},
		{[]string{"some/path", "--cargo"}, "some/path", true, false, false, false},
		{[]string{"--cargo", "some/path"}, "some/path", true, false, false, false},
		{[]string{"--force"}, ".", false, false, true, false},
		{[]string{"some/path", "--cargo", "--force"}, "some/path", true, false, true, false},
		{[]string{"--no-manifests"}, ".", false, true, false, false},
		{[]string{"--no-cluster"}, ".", false, false, false, true},
		{[]string{"some/path", "--cargo", "--no-cluster"}, "some/path", true, false, false, true},
	}
	for _, c := range cases {
		root, cargo, noManifests, force, noCluster := parseBuildArgs(c.args)
		if root != c.wantRoot || cargo != c.wantCargo || noManifests != c.wantNoManifests || force != c.wantForce || noCluster != c.wantNoCluster {
			t.Errorf("parseBuildArgs(%v) = (%q, %v, %v, %v, %v), want (%q, %v, %v, %v, %v)", c.args, root, cargo, noManifests, force, noCluster, c.wantRoot, c.wantCargo, c.wantNoManifests, c.wantForce, c.wantNoCluster)
		}
	}
}

// TestBuildCargoFlag proves --cargo adds crate nodes/edges to graph.json and that
// a plain build over the same corpus does not, so the pass stays opt-in.
func TestBuildCargoFlag(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A real Rust source file so CollectFiles finds a corpus; the crate edges
	// come from the manifests, independent of the source extractor.
	write("app/src/lib.rs", "pub fn run() {}\n")
	write("core/src/lib.rs", "pub fn helper() {}\n")
	write("Cargo.toml", "[workspace]\nmembers = [\"app\", \"core\"]\n")
	write("app/Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\n\n[dependencies]\ncore = { path = \"../core\" }\nserde = \"1\"\n")
	write("core/Cargo.toml", "[package]\nname = \"core\"\nversion = \"0.1.0\"\n")

	graphPath := filepath.Join(root, "graphify-out", "graph.json")

	// Plain build: no crate nodes.
	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := readGraph(t, graphPath); strings.Contains(got, "crate:app") {
		t.Error("plain build must not emit crate nodes")
	}

	// --cargo build: crate nodes and the internal edge appear.
	if err := cmdBuild([]string{root, "--cargo"}); err != nil {
		t.Fatalf("build --cargo: %v", err)
	}
	var g struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
		Links []struct {
			Source   string `json:"source"`
			Target   string `json:"target"`
			Relation string `json:"relation"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(readGraph(t, graphPath)), &g); err != nil {
		t.Fatalf("unmarshal graph: %v", err)
	}
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	if !ids["crate:app"] || !ids["crate:core"] {
		t.Errorf("--cargo build missing crate nodes; have %v", ids)
	}
	if ids["crate:serde"] {
		t.Error("registry dep serde leaked into the graph")
	}
	found := false
	for _, l := range g.Links {
		if l.Source == "crate:app" && l.Target == "crate:core" && l.Relation == "crate_depends_on" {
			found = true
		}
	}
	if !found {
		t.Error("--cargo build missing crate:app -crate_depends_on-> crate:core edge")
	}
}

func readGraph(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read graph: %v", err)
	}
	return string(b)
}
