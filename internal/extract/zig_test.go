package extract

import "testing"

func TestExtractZig(t *testing.T) {
	root := "testdata/zigproj"
	files := []string{"util/math.zig", "web/server.zig"}

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
	for _, want := range []string{"math.zig", "server.zig", "add()", "Server", "Server.start()", "boot()"} {
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
	// Method scoped under its container type, with a contains edge.
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

	// External import edge for std (@import("std")).
	rels := map[string]int{}
	for _, e := range ext.Edges {
		rels[e.Relation]++
	}
	if rels["imports"] == 0 {
		t.Error("no external import edges (expected std)")
	}
}
