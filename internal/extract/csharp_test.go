package extract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractCSharp(t *testing.T) {
	root := "testdata/csharpproj"
	files := []string{"util/math.cs", "web/server.cs"}

	// .cs is not yet wired into File(); call the extractor directly per file.
	var results []Result
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", f, err)
		}
		results = append(results, extractCSharp(filepath.ToSlash(f), src))
	}
	ext := Resolve(results, files)

	labels := map[string]bool{}
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	for _, want := range []string{"math.cs", "server.cs", "Add()", "Server", "Server.Start()", "Boot()"} {
		if !labels[want] {
			t.Errorf("missing node label %q", want)
		}
	}

	has := func(srcLabel, rel, tgtLabel string) bool {
		for _, e := range ext.Edges {
			if e.Relation == rel && id2label[e.Source] == srcLabel && id2label[e.Target] == tgtLabel {
				return true
			}
		}
		return false
	}
	// Method scoped under its type, with a contains edge.
	if !has("Server", "contains", "Server.Start()") {
		t.Error("expected Server --contains--> Server.Start")
	}
	// Same-file call: Server.Start -> Boot.
	if !has("Server.Start()", "calls", "Boot()") {
		t.Error("expected Server.Start --calls--> Boot")
	}
	// Cross-file call: Boot -> Add (unique global definition).
	if !has("Boot()", "calls", "Add()") {
		t.Error("expected cross-file Boot --calls--> Add")
	}

	// External import edge for System.Text.
	rels := map[string]int{}
	for _, e := range ext.Edges {
		rels[e.Relation]++
	}
	if rels["imports"] == 0 {
		t.Error("no external import edges (expected System.Text)")
	}
}
