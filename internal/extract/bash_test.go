package extract

import "testing"

func TestExtractBash(t *testing.T) {
	root := "testdata/bashproj"
	files := []string{"util/math.sh", "app/main.sh"}

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
	for _, want := range []string{"math.sh", "main.sh", "add()", "double()", "boot()"} {
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

	// File contains its function definition.
	if !has("math.sh", "contains", "add()") {
		t.Error("expected math.sh --contains--> add()")
	}
	// Same-file call: double -> add.
	if !has("double()", "calls", "add()") {
		t.Error("expected double --calls--> add")
	}
	// Cross-file call: boot (app/main.sh) -> add (util/math.sh), unique global.
	if !has("boot()", "calls", "add()") {
		t.Error("expected cross-file boot --calls--> add")
	}

	// `source "../util/math.sh"` resolves to a corpus file (imports_from).
	if !has("main.sh", "imports_from", "math.sh") {
		t.Error("expected main.sh --imports_from--> math.sh")
	}

	// `. /etc/profile` is an unresolved (external) include.
	rels := map[string]int{}
	for _, e := range ext.Edges {
		rels[e.Relation]++
	}
	if rels["imports"] == 0 {
		t.Error("no external import edges (expected /etc/profile)")
	}
}
