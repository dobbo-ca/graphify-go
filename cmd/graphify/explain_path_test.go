package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestGraph writes a graph.json with the given JSON body at path.
func writeTestGraph(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCmdExplainPathGraphOverride verifies that --graph makes explain and path
// load an alternate graph.json (whose nodes exist only there), while the default
// graphify-out/graph.json still backs the commands when no --graph is given.
func TestCmdExplainPathGraphOverride(t *testing.T) {
	dir := t.TempDir()
	// Default graph: only "alpha". Alt graph: "beta" -> "gamma".
	writeTestGraph(t, filepath.Join(dir, "graphify-out", "graph.json"),
		`{"nodes":[{"id":"alpha","label":"alpha"}],"links":[]}`)
	writeTestGraph(t, filepath.Join(dir, "graphify-out", "alt.json"),
		`{"nodes":[{"id":"beta","label":"beta"},{"id":"gamma","label":"gamma"}],"links":[{"source":"beta","target":"gamma","relation":"calls"}]}`)

	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	// Canonical cwd so containment checks match across symlinked tmpdirs.
	cwd, _ := os.Getwd()
	alt := filepath.Join(cwd, "graphify-out", "alt.json")

	// explain resolves alt-only "beta" only when pointed at the alt graph.
	if err := cmdExplain([]string{"beta", "--graph", alt}); err != nil {
		t.Errorf("cmdExplain beta --graph alt = %v, want success (alt graph loaded)", err)
	}
	if err := cmdExplain([]string{"beta", "--graph=" + alt}); err != nil {
		t.Errorf("cmdExplain beta --graph=alt = %v, want success (alt graph loaded)", err)
	}
	// "alpha" lives only in the default graph, so against the alt graph it must miss.
	if err := cmdExplain([]string{"alpha", "--graph", alt}); err == nil {
		t.Error("cmdExplain alpha --graph alt = nil, want miss (alt graph has no alpha)")
	}

	// path traverses beta -> gamma only in the alt graph.
	if err := cmdPath([]string{"beta", "gamma", "--graph", alt}); err != nil {
		t.Errorf("cmdPath beta gamma --graph alt = %v, want success (alt graph loaded)", err)
	}

	// Without --graph the default graph.json still backs both commands.
	if err := cmdExplain([]string{"alpha"}); err != nil {
		t.Errorf("cmdExplain alpha (default graph) = %v, want success", err)
	}
	if err := cmdExplain([]string{"beta"}); err == nil {
		t.Error("cmdExplain beta (default graph) = nil, want miss (default graph has no beta)")
	}
}

// TestCmdExplainPathGraphTraversal ensures the --graph override on explain and
// path is contained: a path escaping graphify-out is rejected.
func TestCmdExplainPathGraphTraversal(t *testing.T) {
	dir := t.TempDir()
	writeTestGraph(t, filepath.Join(dir, "graphify-out", "graph.json"),
		`{"nodes":[{"id":"alpha","label":"alpha"}],"links":[]}`)
	secret := filepath.Join(dir, "secrets.json")
	if err := os.WriteFile(secret, []byte(`{"nodes":[],"links":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	cwd, _ := os.Getwd()

	for _, p := range []string{secret, "../secrets.json", filepath.Join(cwd, "secrets.json")} {
		if err := cmdExplain([]string{"alpha", "--graph", p}); err == nil {
			t.Errorf("cmdExplain --graph %q = nil, want containment error", p)
		}
		if err := cmdPath([]string{"alpha", "alpha", "--graph", p}); err == nil {
			t.Errorf("cmdPath --graph %q = nil, want containment error", p)
		}
	}
}
