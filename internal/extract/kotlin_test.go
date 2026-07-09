package extract

import "testing"

func TestExtractKotlin(t *testing.T) {
	root := "testdata/ktproj"
	files := []string{"util/math.kt", "web/server.kt"}

	var results []Result
	for _, f := range files {
		r, err := File(root, f)
		if err != nil {
			t.Fatalf("File(%s): %v", f, err)
		}
		results = append(results, r)
	}
	ext := Resolve(results, files)

	labels := map[string]bool{}
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	for _, want := range []string{"math.kt", "server.kt", "add()", "Server", "Server.start()", "Server.boot()"} {
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
	// Method scoped under its class, with a contains edge.
	if !has("Server", "contains", "Server.start()") {
		t.Error("expected Server --contains--> Server.start")
	}
	// Same-file call: Server.start -> boot (method, resolved locally).
	if !has("Server.start()", "calls", "Server.boot()") {
		t.Error("expected Server.start --calls--> Server.boot")
	}
	// Cross-file call: boot -> add (unique global definition).
	if !has("Server.boot()", "calls", "add()") {
		t.Error("expected cross-file Server.boot --calls--> add")
	}
	// External import edge for kotlin.math.sqrt.
	rels := map[string]int{}
	for _, e := range ext.Edges {
		rels[e.Relation]++
	}
	if rels["imports"] == 0 {
		t.Error("no external import edges (expected kotlin.math.sqrt)")
	}
}

// TestExtractKotlinEnum checks that enum entries become nodes linked to the enum
// class by a `case_of` edge, and that an entry's anonymous class body still
// contributes its methods (attached to the entry).
func TestExtractKotlinEnum(t *testing.T) {
	src := []byte(`enum class Color {
  RED,
  GREEN {
    fun greet() {}
  };
}`)
	res := extractKotlin("Color.kt", src)

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
	// Anonymous-body method attaches to the entry, not the enum class.
	if !has("GREEN", "contains", "GREEN.greet()") {
		t.Error("expected GREEN --contains--> GREEN.greet")
	}
}
