package extract

import "testing"

func TestExtractAndResolve(t *testing.T) {
	root := "testdata/proj"
	files := []string{"util/math.go", "web/server.ts"}

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
	for _, n := range ext.Nodes {
		labels[n.Label] = true
	}
	for _, want := range []string{"math.go", "server.ts", "Add()", "Calc", "Calc.Sum()", "Server", "Server.start()", "boot()"} {
		if !labels[want] {
			t.Errorf("missing node label %q", want)
		}
	}

	// Relation presence checks.
	rels := map[string]int{}
	for _, e := range ext.Edges {
		rels[e.Relation]++
	}
	if rels["contains"] == 0 {
		t.Error("no contains edges")
	}
	if rels["calls"] == 0 {
		t.Error("no calls edges (expected Sum->Add and boot->start)")
	}
	if rels["imports"] == 0 {
		t.Error("no external import edges (expected express)")
	}

	// Specific: Sum calls Add (same-file resolution).
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		id2label[n.ID] = n.Label
	}
	found := false
	for _, e := range ext.Edges {
		if e.Relation == "calls" && id2label[e.Source] == "Calc.Sum()" && id2label[e.Target] == "Add()" {
			found = true
		}
	}
	if !found {
		t.Error("expected Calc.Sum --calls--> Add")
	}
}
