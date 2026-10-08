package main

import (
	"os"
	"path/filepath"
	"strings"
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

// TestScanRootIgnoresForeignMarker verifies a .graphify_root recording a tree
// that does not contain the cwd (a stale or committed marker) is ignored rather
// than redirecting the scan — and clobbering — that unrelated tree.
func TestScanRootIgnoresForeignMarker(t *testing.T) {
	dir := t.TempDir()
	victim := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc Alpha() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := cmdBuild(nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "graphify-out", rootFileName), []byte(victim+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := scanRoot(); got != dir {
		t.Errorf("scanRoot() = %q, want %q (foreign marker ignored)", got, dir)
	}
	if err := cmdUpdate(nil); err != nil {
		t.Fatalf("cmdUpdate = %v, want success", err)
	}
	if _, err := os.Stat(filepath.Join(victim, "graphify-out")); err == nil {
		t.Error("update wrote a graphify-out into the tree named by the foreign marker")
	}
}

// TestBuildFromSubdirKeepsSubdirOut verifies `build` writes under the tree it
// scanned instead of walking up and overwriting the repo's graph with a
// subtree-only one.
func TestBuildFromSubdirKeepsSubdirOut(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc Alpha() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.go"), []byte("package b\n\nfunc Beta() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := cmdBuild(nil); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "graphify-out", "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	if err := cmdBuild(nil); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "graphify-out", "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("build from a subdirectory overwrote the parent tree's graph")
	}
	if _, err := os.Stat(filepath.Join(sub, "graphify-out", "graph.json")); err != nil {
		t.Errorf("build from a subdirectory did not write its own graphify-out: %v", err)
	}
}

// TestUpdateExplicitSubdirRedirectsToRoot verifies an explicit path inside the
// recorded scan root updates the repo graph instead of forking a subdir one.
func TestUpdateExplicitSubdirRedirectsToRoot(t *testing.T) {
	for _, env := range []bool{false, true} {
		dir, _ := filepath.EvalSymlinks(t.TempDir()) // macOS /var symlink
		sub := filepath.Join(dir, "sub")
		other := filepath.Join(dir, "other")
		for p, src := range map[string]string{
			filepath.Join(sub, "a.go"):   "package a\n\nfunc Alpha() {}\n",
			filepath.Join(other, "b.go"): "package b\n\nfunc Beta() {}\n",
		} {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		out := filepath.Join(dir, "graphify-out")
		if env {
			out = filepath.Join(t.TempDir(), "out")
			t.Setenv("GRAPHIFY_OUT", out)
		} else {
			t.Setenv("GRAPHIFY_OUT", "")
		}
		t.Chdir(dir)
		if err := cmdBuild([]string{"."}); err != nil {
			t.Fatal(err)
		}
		arg := "."
		if env {
			arg = "sub"
		} else {
			t.Chdir(sub)
		}
		if err := cmdUpdate([]string{arg}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(sub, "graphify-out")); err == nil {
			t.Errorf("env=%v: stray sub/graphify-out created", env)
		}
		g, err := os.ReadFile(filepath.Join(out, "graph.json"))
		if err != nil || !strings.Contains(string(g), "Beta") {
			t.Errorf("env=%v: sibling symbol lost from graph (err=%v)", env, err)
		}
	}
}

// TestUpdateDotWithSiblingOutKeepsRoot verifies `update .` never widens to the
// parent of GRAPHIFY_OUT when no trusted marker exists.
func TestUpdateDotWithSiblingOutKeepsRoot(t *testing.T) {
	for _, marker := range []string{"", "."} {
		base, _ := filepath.EvalSymlinks(t.TempDir())
		repo := filepath.Join(base, "repo")
		for p, src := range map[string]string{
			filepath.Join(repo, "a.go"):              "package a\n\nfunc Alpha() {}\n",
			filepath.Join(base, "unrelated", "u.go"): "package u\n\nfunc Foreign() {}\n",
		} {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		out := filepath.Join(base, "out")
		t.Setenv("GRAPHIFY_OUT", out)
		t.Chdir(repo)
		if err := cmdBuild([]string{"."}); err != nil {
			t.Fatal(err)
		}
		if marker == "" {
			os.Remove(filepath.Join(out, rootFileName))
		} else if err := os.WriteFile(filepath.Join(out, rootFileName), []byte(marker), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := cmdUpdate([]string{"."}); err != nil {
			t.Fatal(err)
		}
		g, err := os.ReadFile(filepath.Join(out, "graph.json"))
		if err != nil || strings.Contains(string(g), "Foreign") || !strings.Contains(string(g), "Alpha") {
			t.Errorf("marker=%q: update . scanned outside the repo (err=%v)", marker, err)
		}
	}
}

// TestScanRootStaysInsideGitWorkTree verifies a committed .graphify_root naming
// an ancestor of the repository cannot widen an update above the work tree.
func TestScanRootStaysInsideGitWorkTree(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		base, _ := filepath.EvalSymlinks(t.TempDir())
		repo := filepath.Join(base, "repo")
		sub := filepath.Join(repo, "sub")
		for p, src := range map[string]string{
			filepath.Join(repo, "a.go"):            "package a\n\nfunc Alpha() {}\n",
			filepath.Join(sub, "b.go"):             "package b\n\nfunc Beta() {}\n",
			filepath.Join(base, "private", "u.go"): "package u\n\nfunc Foreign() {}\n",
		} {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		gitRun(t, repo, "init")
		t.Setenv("GRAPHIFY_OUT", "")
		t.Chdir(repo)
		if err := cmdBuild([]string{"."}); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(repo, "graphify-out")
		if err := os.WriteFile(filepath.Join(out, rootFileName), []byte(".."), 0o644); err != nil {
			t.Fatal(err)
		}
		var args []string
		if explicit {
			t.Chdir(sub)
			args = []string{"."}
		}
		if err := cmdUpdate(args); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(base, "graphify-out")); err == nil {
			t.Errorf("explicit=%v: update wrote a graph above the work tree", explicit)
		}
		g, err := os.ReadFile(filepath.Join(out, "graph.json"))
		if err != nil || strings.Contains(string(g), "Foreign") || !strings.Contains(string(g), "Beta") {
			t.Errorf("explicit=%v: update scanned above the work tree (err=%v)", explicit, err)
		}
	}
}
