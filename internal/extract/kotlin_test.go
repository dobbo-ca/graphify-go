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
