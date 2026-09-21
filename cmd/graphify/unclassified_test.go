package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/query"
)

// A corpus written mostly in a language no extractor covers must say so: the
// unclassified count and the offending extension ride in graph.json's
// graph-level attributes, so `validate` and the MCP graph_stats tool can tell
// an agent the graph covers only a sliver of the repo.
func TestBuildRecordsUnclassifiedCorpus(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 40; i++ {
		p := filepath.Join(root, fmt.Sprintf("View%d.swift", i))
		if err := os.WriteFile(p, []byte("struct V {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package p\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build: %v", err)
	}

	g, err := query.Load(filepath.Join(root, "graphify-out", "graph.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if g.Attrs.UnclassifiedFiles != 40 {
		t.Errorf("UnclassifiedFiles = %d, want 40 (attrs %+v)", g.Attrs.UnclassifiedFiles, g.Attrs)
	}
	got := g.UnclassifiedSummary()
	if !strings.Contains(got, "40") || !strings.Contains(got, ".swift") {
		t.Errorf("summary %q: want the count 40 and the extension .swift named", got)
	}
}

// A fully-classified corpus reports no unclassified line at all, so the signal
// stays meaningful.
func TestBuildFullyClassifiedCorpusHasNoUnclassifiedLine(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package p\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build: %v", err)
	}
	g, err := query.Load(filepath.Join(root, "graphify-out", "graph.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := g.UnclassifiedSummary(); got != "" {
		t.Errorf("summary = %q, want empty for a fully-classified corpus", got)
	}
}

// graphWithUnclassified writes a minimal graph.json carrying the coverage
// attributes under root/graphify-out and returns its path.
func graphWithUnclassified(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "graphify-out")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "graph.json")
	const doc = `{"directed":false,"multigraph":false,
  "graph":{"unclassified_files":40,"unclassified_extensions":{".swift":40}},
  "nodes":[{"id":"a","label":"a()","file_type":"code","source_file":"main.go","source_location":"L1","community":0,"norm_label":"a()"}],
  "links":[]}`
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// graph_stats is agent-parsed: the coverage line must appear there, not only in
// the human report.
func TestToolGraphStatsReportsUnclassified(t *testing.T) {
	g, err := query.Load(graphWithUnclassified(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	out := newMCPServer(g).toolGraphStats(nil)
	if !strings.Contains(out, "Unclassified: 40 file(s) no extractor handles (.swift 40)") {
		t.Errorf("graph_stats missing the coverage line:\n%s", out)
	}
}

// `graphify validate` prints the coverage line alongside its verdict, and a
// thinly-covered corpus is still a structurally OK graph (exit 0).
func TestCmdValidateReportsUnclassified(t *testing.T) {
	root := t.TempDir()
	graphWithUnclassified(t, root)
	t.Chdir(root)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	cmdErr := cmdValidate()
	os.Stdout = orig
	w.Close()
	out, _ := io.ReadAll(r)

	if cmdErr != nil {
		t.Fatalf("validate: %v", cmdErr)
	}
	if !strings.Contains(string(out), "Unclassified: 40 file(s) no extractor handles (.swift 40)") {
		t.Errorf("validate missing the coverage line:\n%s", out)
	}
}
