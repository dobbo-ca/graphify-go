package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/query"
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

// TestCmdPathUndirected verifies that path follows edge direction by default and
// only traverses backwards with --undirected, pointing the user at the flag.
func TestCmdPathUndirected(t *testing.T) {
	dir := t.TempDir()
	writeTestGraph(t, filepath.Join(dir, "graphify-out", "graph.json"),
		`{"nodes":[{"id":"beta","label":"beta"},{"id":"gamma","label":"gamma"}],"links":[{"source":"beta","target":"gamma","relation":"calls"}]}`)

	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	err := cmdPath([]string{"gamma", "beta"})
	if err == nil || !strings.Contains(err.Error(), "--undirected") {
		t.Fatalf("cmdPath gamma beta = %v, want no-directed-path error hinting --undirected", err)
	}
	if err := cmdPath([]string{"gamma", "beta", "--undirected"}); err != nil {
		t.Errorf("cmdPath gamma beta --undirected = %v, want success", err)
	}
}

// TestCmdPathAnnotatesHops verifies that path prints each hop's relation and
// confidence, and flips the arrow when the stored edge runs against the
// direction of travel rather than asserting a call that does not exist.
func TestCmdPathAnnotatesHops(t *testing.T) {
	dir := t.TempDir()
	writeTestGraph(t, filepath.Join(dir, "graphify-out", "graph.json"),
		`{"nodes":[{"id":"a","label":"a()"},{"id":"b","label":"b()"},{"id":"c","label":"c()"}],`+
			`"links":[{"source":"a","target":"b","relation":"calls","confidence":"INFERRED"},`+
			`{"source":"c","target":"b","relation":"imports","confidence":"EXTRACTED"}]}`)

	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	out := captureStdout(t, func() {
		if err := cmdPath([]string{"a()", "c()", "--undirected"}); err != nil {
			t.Errorf("cmdPath a c --undirected = %v, want success", err)
		}
	})
	want := "a() --calls [INFERRED]--> b() <--imports [EXTRACTED]-- c()\n"
	if out != want {
		t.Errorf("path output = %q, want %q", out, want)
	}
}

// TestExplainLinesCapsAndGroups checks that explain shows at most
// explainConnCap connections in full and folds the rest into per-file counts.
func TestExplainLinesCapsAndGroups(t *testing.T) {
	var nbrs []query.Neighbor
	for i := 0; i < explainConnCap+5; i++ {
		nbrs = append(nbrs, query.Neighbor{Label: "n", Relation: "calls", Direction: "<-", File: "a.go"})
	}
	for i := 0; i < 3; i++ {
		nbrs = append(nbrs, query.Neighbor{Label: "n", Relation: "calls", Direction: "->", File: "b.go"})
	}
	lines := explainLines(nbrs)
	// 20 full lines + summary + 2 grouped file lines.
	if len(lines) != explainConnCap+3 {
		t.Fatalf("lines = %d (%v), want %d", len(lines), lines, explainConnCap+3)
	}
	if want := "  ... 8 more connections. Grouped by file:"; lines[explainConnCap] != want {
		t.Errorf("summary = %q, want %q", lines[explainConnCap], want)
	}
	// Highest count first: 5 cut neighbours in a.go, then 3 in b.go.
	if !strings.Contains(lines[explainConnCap+1], "a.go") || !strings.HasSuffix(lines[explainConnCap+1], " 5") {
		t.Errorf("first group = %q, want a.go with 5", lines[explainConnCap+1])
	}
	if !strings.Contains(lines[explainConnCap+2], "b.go") || !strings.HasSuffix(lines[explainConnCap+2], " 3") {
		t.Errorf("second group = %q, want b.go with 3", lines[explainConnCap+2])
	}
}

// TestExplainLinesShortListUngrouped checks that a node under the cap prints
// every connection with no grouped tail.
func TestExplainLinesShortListUngrouped(t *testing.T) {
	lines := explainLines([]query.Neighbor{{Label: "n", Relation: "calls", Direction: "->", File: "a.go"}})
	if len(lines) != 1 {
		t.Fatalf("lines = %v, want 1 connection line", lines)
	}
	if got := explainLines(nil); len(got) != 1 || got[0] != "  (no connections)" {
		t.Errorf("explainLines(nil) = %v, want (no connections)", got)
	}
}
