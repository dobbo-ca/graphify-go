package extract

import "testing"

func TestExtractPython(t *testing.T) {
	root := "testdata/pyproj"
	files := []string{"util/math.py", "web/server.py"}

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
	for _, want := range []string{"math.py", "server.py", "add()", "Server", "Server.start()", "boot()"} {
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
	// Same-file call: Server.start -> boot.
	if !has("Server.start()", "calls", "boot()") {
		t.Error("expected Server.start --calls--> boot")
	}
	// Cross-file call: boot -> add (unique global definition).
	if !has("boot()", "calls", "add()") {
		t.Error("expected cross-file boot --calls--> add")
	}
	// External import edge for os.
	rels := map[string]int{}
	for _, e := range ext.Edges {
		rels[e.Relation]++
	}
	if rels["imports"] == 0 {
		t.Error("no external import edges (expected os)")
	}
}
