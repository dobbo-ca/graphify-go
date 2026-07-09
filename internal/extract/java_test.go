package extract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractJava(t *testing.T) {
	root := "testdata/javaproj"
	files := []string{"util/Math.java", "web/Server.java"}

	// Dispatch for .java is not wired into File() yet, so call extractJava
	// directly. Reading + extracting here mirrors what File() will do once wired.
	var results []Result
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		results = append(results, extractJava(filepath.ToSlash(f), src))
	}
	ext := Resolve(results, files)

	labels := map[string]bool{}
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	for _, want := range []string{
		"Math.java", "Server.java", "Math", "Math.add()",
		"Server", "Server.start()", "Server.boot()",
	} {
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
	if !has("Server", "contains", "Server.start()") {
		t.Error("expected Server --contains--> Server.start")
	}
	// Same-file call: Server.start -> boot.
	if !has("Server.start()", "calls", "Server.boot()") {
		t.Error("expected Server.start --calls--> Server.boot")
	}
	// Cross-file call: Server.boot -> add (unique global definition in Math).
	if !has("Server.boot()", "calls", "Math.add()") {
		t.Error("expected cross-file Server.boot --calls--> Math.add")
	}

	// External import edge for util.Math.
	rels := map[string]int{}
	for _, e := range ext.Edges {
		rels[e.Relation]++
	}
	if rels["imports"] == 0 {
		t.Error("no external import edges (expected util.Math)")
	}
}

// TestExtractJavaEnum checks that enum constants become nodes linked to the enum
// type by a `case_of` edge, and that a constant's anonymous class body still
// contributes its methods (attached to the constant).
func TestExtractJavaEnum(t *testing.T) {
	src := []byte(`enum Color {
  RED,
  GREEN {
    void greet() {}
  };
}`)
	res := extractJava("Color.java", src)

	id2label := map[string]string{}
	labels := map[string]bool{}
	for _, n := range res.Nodes {
		id2label[n.ID] = n.Label
		labels[n.Label] = true
	}
	for _, want := range []string{"Color", "RED", "GREEN", "GREEN.greet()"} {
		if !labels[want] {
			t.Errorf("missing node label %q", want)
		}
	}

	has := func(srcLabel, rel, tgtLabel string) bool {
		for _, e := range res.Edges {
			if e.Relation == rel && id2label[e.Source] == srcLabel && id2label[e.Target] == tgtLabel {
				return true
			}
		}
		return false
	}
	if !has("Color", "case_of", "RED") {
		t.Error("expected Color --case_of--> RED")
	}
	if !has("Color", "case_of", "GREEN") {
		t.Error("expected Color --case_of--> GREEN")
	}
	// Anonymous-body method attaches to the constant, not the enum type.
	if !has("GREEN", "contains", "GREEN.greet()") {
		t.Error("expected GREEN --contains--> GREEN.greet")
	}
}
