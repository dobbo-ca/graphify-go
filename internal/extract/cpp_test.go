package extract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractCpp(t *testing.T) {
	root := "testdata/cppproj"
	files := []string{"util/math.cpp", "web/server.cpp"}

	// C++ dispatch is wired centrally; call extractCpp directly here so the test
	// is self-contained.
	var results []Result
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", f, err)
		}
		results = append(results, extractCpp(filepath.ToSlash(f), src))
	}
	ext := Resolve(results, files)

	labels := map[string]bool{}
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	for _, want := range []string{"math.cpp", "server.cpp", "add()", "Server", "Server.start()", "boot()"} {
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
	// Inline method scoped under its class, with a contains edge.
	if !has("Server", "contains", "Server.start()") {
		t.Error("expected Server --contains--> Server.start")
	}
	// Same-file call: Server.start -> boot.
	if !has("Server.start()", "calls", "boot()") {
		t.Error("expected Server.start --calls--> boot")
	}
	// Cross-file call: boot -> add (unique global definition).
	if !has("boot()", "calls", "add()") {
		t.Error("expected cross-file boot --calls--> add")
	}

	// External import edge for <string>.
	rels := map[string]int{}
	for _, e := range ext.Edges {
		rels[e.Relation]++
	}
	if rels["imports"] == 0 {
		t.Error("no external import edges (expected string)")
	}
}

// A `.h` declaring a C++ class routes to the C++ extractor; the C grammar has
// no class_specifier and would emit a function-shaped `Widget()`.
func TestCppHeaderRoutesToCppExtractor(t *testing.T) {
	labels := func(src string) map[string]bool {
		out := map[string]bool{}
		for _, n := range FileFromBytes("w.h", []byte(src)).Nodes {
			out[n.Label] = true
		}
		return out
	}

	got := labels("class Widget { public: void show(); virtual int area() const = 0; int count; };\n")
	if !got["Widget"] || got["Widget()"] {
		t.Errorf("C++ header: want class node Widget, got %v", got)
	}

	// A plain C header keeps the C extractor's output.
	plain := "#include <stdio.h>\nstruct point { int x; };\nint add(int a, int b);\n"
	want := map[string]bool{}
	for _, n := range extractC("w.h", []byte(plain)).Nodes {
		want[n.Label] = true
	}
	if got := labels(plain); len(got) != len(want) || isCppHeader([]byte(plain)) {
		t.Errorf("plain C header: got %v, want %v", got, want)
	}
}
