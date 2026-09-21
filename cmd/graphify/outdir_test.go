package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOutDirWalksUpFromSubdir verifies read commands find the repo's graph from
// a nested subdirectory instead of failing on the bare relative literal.
func TestOutDirWalksUpFromSubdir(t *testing.T) {
	dir := t.TempDir()
	writeTestGraph(t, filepath.Join(dir, "graphify-out", "graph.json"),
		`{"nodes":[{"id":"alpha","label":"alpha"}],"links":[]}`)
	sub := filepath.Join(dir, "internal", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	if err := cmdExplain([]string{"alpha"}); err != nil {
		t.Errorf("cmdExplain alpha from subdir = %v, want success (graph found by upward walk)", err)
	}
}

// TestOutDirHonoursEnv verifies GRAPHIFY_OUT overrides both the upward walk and
// the cwd-local default.
func TestOutDirHonoursEnv(t *testing.T) {
	dir := t.TempDir()
	// A cwd-local graph that must lose to the env override.
	writeTestGraph(t, filepath.Join(dir, "graphify-out", "graph.json"),
		`{"nodes":[{"id":"alpha","label":"alpha"}],"links":[]}`)
	alt := filepath.Join(dir, "elsewhere")
	writeTestGraph(t, filepath.Join(alt, "graph.json"),
		`{"nodes":[{"id":"beta","label":"beta"}],"links":[]}`)
	t.Chdir(dir)
	t.Setenv("GRAPHIFY_OUT", alt)

	if got := outDir(); got != alt {
		t.Errorf("outDir() = %q, want %q", got, alt)
	}
	if err := cmdExplain([]string{"beta"}); err != nil {
		t.Errorf("cmdExplain beta = %v, want success (GRAPHIFY_OUT graph loaded)", err)
	}
	if err := cmdExplain([]string{"alpha"}); err == nil {
		t.Error("cmdExplain alpha = nil, want miss (env graph has no alpha)")
	}
}

// TestUpdateFromSubdirUpdatesRepoGraph verifies `update` with no path argument
// refreshes the graph the read commands load rather than building a stray
// second one rooted at the cwd.
func TestUpdateFromSubdirUpdatesRepoGraph(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc Alpha() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := cmdBuild(nil); err != nil {
		t.Fatal(err)
	}
	root, err := os.ReadFile(filepath.Join(dir, "graphify-out", rootFileName))
	if err != nil {
		t.Fatalf("build did not record %s: %v", rootFileName, err)
	}
	if len(root) == 0 {
		t.Fatalf("%s is empty", rootFileName)
	}

	sub := filepath.Join(dir, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	if err := cmdUpdate(nil); err != nil {
		t.Fatalf("cmdUpdate from subdir = %v, want success", err)
	}
	if _, err := os.Stat(filepath.Join(sub, "graphify-out")); err == nil {
		t.Error("update from a subdirectory built a stray graphify-out under the cwd")
	}
}
