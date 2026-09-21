package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/cache"
	"github.com/dobbo-ca/graphify-go/internal/detect"
	"github.com/dobbo-ca/graphify-go/internal/extract"
)

// TestIncrementalUpdate checks that `update` reuses the cache and produces the
// same graph a full build would: byte-identical output when nothing changed,
// and a single re-parse when one file changes.
func TestIncrementalUpdate(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package p\n\nfunc A() { B() }\n")
	write("b.go", "package p\n\nfunc B() {}\n")

	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build: %v", err)
	}
	// build writes the stat sidecar with an entry per collected file.
	statIdx := cache.LoadStat(filepath.Join(root, "graphify-out", cache.StatFileName), cache.Stamp(version))
	if len(statIdx) != 2 {
		t.Errorf("stat sidecar: got %d entries, want 2", len(statIdx))
	}
	graphPath := filepath.Join(root, "graphify-out", "graph.json")
	built, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}

	// An unchanged update reuses every file and re-parses none.
	files, _ := detect.CollectFiles(root)
	prev := cache.Load(filepath.Join(root, "graphify-out", cache.FileName), cache.Stamp(version))
	prevStat := cache.LoadStat(filepath.Join(root, "graphify-out", cache.StatFileName), cache.Stamp(version))
	_, _, _, stats := assemble(root, files, prev, prevStat)
	if stats.parsed != 0 || stats.reused != len(files) || stats.dropped != 0 {
		t.Errorf("unchanged update: got %+v, want 0 reparsed / %d reused / 0 removed", stats, len(files))
	}

	if err := cmdUpdate([]string{root}); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(built, updated) {
		t.Error("incremental update changed graph.json despite no source change")
	}

	// Changing one file re-parses exactly that file.
	write("b.go", "package p\n\nfunc B() {}\n\nfunc C() {}\n")
	files, _ = detect.CollectFiles(root)
	prev = cache.Load(filepath.Join(root, "graphify-out", cache.FileName), cache.Stamp(version))
	prevStat = cache.LoadStat(filepath.Join(root, "graphify-out", cache.StatFileName), cache.Stamp(version))
	_, _, _, stats = assemble(root, files, prev, prevStat)
	if stats.parsed != 1 || stats.reused != len(files)-1 {
		t.Errorf("one-file change: got %+v, want 1 reparsed / %d reused", stats, len(files)-1)
	}
	if err := cmdUpdate([]string{root}); err != nil {
		t.Fatalf("update after change: %v", err)
	}
	changed, _ := os.ReadFile(graphPath)
	if bytes.Equal(built, changed) {
		t.Error("expected graph.json to change after editing a source file")
	}
}

// TestUpdateAllowsLegitShrink is the regression for the #479/#1116 interaction:
// removing symbols from a re-parsed file legitimately shrinks the graph, and
// `update` must refresh graph.json rather than warn and keep the larger stale one.
func TestUpdateAllowsLegitShrink(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package p\n\nfunc A() {}\n\nfunc B() {}\n\nfunc C() {}\n")
	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build: %v", err)
	}
	outDir := filepath.Join(root, "graphify-out")
	graphPath := filepath.Join(outDir, "graph.json")
	before := graphNodeCount(t, graphPath)
	reportBefore, err := os.ReadFile(filepath.Join(outDir, "GRAPH_REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}

	// Remove B and C — the graph must shrink and the write must not be refused.
	write("a.go", "package p\n\nfunc A() {}\n")
	if err := cmdUpdate([]string{root}); err != nil {
		t.Fatalf("update: %v", err)
	}
	after := graphNodeCount(t, graphPath)
	if after >= before {
		t.Errorf("legit shrink not applied: before=%d after=%d (graph.json should have refreshed to fewer nodes)", before, after)
	}
	// The legit write path refreshes every output, not just graph.json.
	reportAfter, err := os.ReadFile(filepath.Join(outDir, "GRAPH_REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(reportBefore, reportAfter) {
		t.Error("legit shrink did not refresh GRAPH_REPORT.md")
	}
}

// TestRefusedShrinkAbortsAllOutputs is the regression for graphify-go-1fz.2: on an
// ILLEGITIMATE refused shrink (a lost node's source file was skipped this run),
// writeOutputs must leave graphify-out entirely untouched — graph.json, the report,
// the cache, and the stat sidecar — rather than keeping the larger graph.json while
// still rewriting the other three from the rejected smaller graph. With force set the
// same shrink is accepted and every output is rewritten.
func TestRefusedShrinkAbortsAllOutputs(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package p\n\nfunc A() { B() }\n")
	write("b.go", "package p\n\nfunc B() {}\n")
	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build: %v", err)
	}

	outDir := filepath.Join(root, "graphify-out")
	paths := map[string]string{
		"graph.json":      filepath.Join(outDir, "graph.json"),
		"GRAPH_REPORT.md": filepath.Join(outDir, "GRAPH_REPORT.md"),
		"cache":           filepath.Join(outDir, cache.FileName),
		"stat":            filepath.Join(outDir, cache.StatFileName),
	}
	snapshot := func() map[string][]byte {
		m := make(map[string][]byte, len(paths))
		for name, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("read %s: %v", p, err)
			}
			m[name] = b
		}
		return m
	}
	before := snapshot()

	// Simulate an update where b.go was skipped (failed to read/parse): its result
	// is absent, so the graph shrinks and the lost node's source (b.go) lands in the
	// skipped set — an illegitimate shrink CheckShrink must refuse.
	files := []string{"a.go", "b.go"}
	ra, err := extract.File(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	results := []extract.Result{ra}
	newCache := cache.Cache{"a.go": {Hash: "h", Result: ra}}
	newStat := cache.StatIndex{}

	// force=false: the refusal must abort every write, leaving all four outputs byte-identical.
	if _, _, err := writeOutputs(root, detect.WalkReport{Files: files}, results, newCache, newStat, semanticOpts{}, false, false); err != nil {
		t.Fatalf("writeOutputs (refused shrink) returned error, want a nil-error warning: %v", err)
	}
	for name, b := range snapshot() {
		if !bytes.Equal(before[name], b) {
			t.Errorf("refused shrink rewrote %s; every output must be left untouched", name)
		}
	}

	// force=true: the same shrink is accepted and every output is rewritten.
	if _, _, err := writeOutputs(root, detect.WalkReport{Files: files}, results, newCache, newStat, semanticOpts{}, true, false); err != nil {
		t.Fatalf("writeOutputs (forced shrink): %v", err)
	}
	for name, b := range snapshot() {
		if bytes.Equal(before[name], b) {
			t.Errorf("forced shrink did not rewrite %s; force must write all outputs", name)
		}
	}
}

// graphNodeCount reads a graph.json and returns its node count.
func graphNodeCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var g struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return len(g.Nodes)
}

// TestForceBypassesCache is the regression for graphify-go-2af.6: --force (and
// GRAPHIFY_FORCE=1) mean a full re-scan, so a poisoned cache entry — one whose
// hash still matches the file but whose stored result is wrong — is re-parsed
// rather than reused. A plain update keeps trusting it.
func TestForceBypassesCache(t *testing.T) {
	root := t.TempDir()
	src := "package p\n\nfunc A() {}\n\nfunc C() {}\n"
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build: %v", err)
	}
	graphPath := filepath.Join(root, "graphify-out", "graph.json")
	full := graphNodeCount(t, graphPath)

	// Poison the cache: keep the (correct) hash of a.go, but store the result of
	// a version without C. Any run that trusts the cache will drop C.
	cachePath := filepath.Join(root, "graphify-out", cache.FileName)
	poison := func() {
		c := cache.Load(cachePath, cache.Stamp(version))
		e := c["a.go"]
		e.Result = extract.FileFromBytes("a.go", []byte("package p\n\nfunc A() {}\n"))
		c["a.go"] = e
		if err := cache.Save(cachePath, cache.Stamp(version), c); err != nil {
			t.Fatal(err)
		}
	}

	poison()
	if err := cmdUpdate([]string{root}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := graphNodeCount(t, graphPath); got >= full {
		t.Fatalf("plain update: got %d nodes, want fewer than %d (the poisoned entry should have been reused)", got, full)
	}

	poison()
	if err := cmdUpdate([]string{root, "--force"}); err != nil {
		t.Fatalf("update --force: %v", err)
	}
	if got := graphNodeCount(t, graphPath); got != full {
		t.Errorf("update --force: got %d nodes, want %d (--force must ignore the cache)", got, full)
	}

	poison()
	t.Setenv("GRAPHIFY_FORCE", "1")
	if err := cmdUpdate([]string{root}); err != nil {
		t.Fatalf("update GRAPHIFY_FORCE=1: %v", err)
	}
	if got := graphNodeCount(t, graphPath); got != full {
		t.Errorf("GRAPHIFY_FORCE=1 update: got %d nodes, want %d", got, full)
	}
}
