package extract

import "testing"

func TestExtractRust(t *testing.T) {
	root := "testdata/rsproj"
	files := []string{"util/math.rs", "web/server.rs"}

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
	for _, want := range []string{"math.rs", "server.rs", "add()", "Server", "Server.start()", "boot()"} {
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
	// Inherent impl method scoped under its type, with a contains edge.
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
}
