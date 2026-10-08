package extract

import (
	"os"
	"path/filepath"
	"strings"
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

// An export macro between `class`/`struct` and the name must not hide the type.
func TestCppExportMacroClass(t *testing.T) {
	src := "class Base {};\n" +
		"class Q_CORE_EXPORT\nWidget : public Base { public: void show() {} };\n" +
		"struct API Thing { int x; };\n" +
		"class MY_WIDGET final : public Base {};\n" +
		"class MACRO OTHER final : public Base {};\n" +
		"void F() {\n  for (class API v : items) {}\n  class API w{1};\n}\n"
	if out := blankCppExportMacros([]byte(src)); len(out) != len(src) ||
		!strings.Contains(string(out), "class              \nWidget") ||
		!strings.Contains(string(out), "(class API v :") || !strings.Contains(string(out), "class API w{1}") {
		t.Errorf("blanking changed offsets or touched a variable:\n%s", out)
	}

	res := FileFromBytes("w.h", []byte(src))
	labels := map[string]string{}
	for _, n := range res.Nodes {
		labels[n.Label] = n.SourceLocation
	}
	for _, want := range []string{"Widget", "Widget.show()", "Thing", "MY_WIDGET", "OTHER"} {
		if _, ok := labels[want]; !ok {
			t.Errorf("missing node %q in %v", want, labels)
		}
	}
	if labels["Thing"] != "L4" {
		t.Errorf("Thing at %s, want L4", labels["Thing"])
	}
	inherits := false
	for _, r := range res.TypeRefs {
		if r.FromID == "w_widget" && r.Name == "Base" && r.Relation == "inherits" {
			inherits = true
		}
	}
	if !inherits {
		t.Errorf("no Widget inherits Base ref in %v", res.TypeRefs)
	}
}
