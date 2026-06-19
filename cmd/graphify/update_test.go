package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/cache"
	"github.com/dobbo-ca/graphify-go/internal/detect"
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
	statIdx := cache.LoadStat(filepath.Join(root, "graphify-out", cache.StatFileName))
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
	prev := cache.Load(filepath.Join(root, "graphify-out", cache.FileName))
	prevStat := cache.LoadStat(filepath.Join(root, "graphify-out", cache.StatFileName))
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
	prev = cache.Load(filepath.Join(root, "graphify-out", cache.FileName))
	prevStat = cache.LoadStat(filepath.Join(root, "graphify-out", cache.StatFileName))
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
