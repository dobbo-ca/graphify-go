package extract

import "testing"

// hasCall scans resolved edges for a src->tgt calls edge.
func hasCall(edges []edge, src, tgt string) bool {
	for _, e := range edges {
		if e.Relation == "calls" && e.Source == src && e.Target == tgt {
			return true
		}
	}
	return false
}

type edge = struct {
	Source, Target, Relation string
}

// TestResolveAmbiguousByImport checks that a call to a name defined in two files
// resolves to the one the caller actually imports, instead of being skipped.
func TestResolveAmbiguousByImport(t *testing.T) {
	results := []Result{
		{Defs: []Def{{ID: "A", Name: "helper", File: "a/util.js"}}},
		{Defs: []Def{{ID: "B", Name: "helper", File: "b/util.js"}}},
		{
			Defs:  []Def{{ID: "MAIN", Name: "run", File: "main.js"}},
			Calls: []Call{{CallerID: "MAIN", Callee: "helper", File: "main.js", Loc: "L2"}},
			Imps:  []Imp{{FileID: "mainfile", File: "main.js", Spec: "./a/util", Loc: "L1"}},
		},
	}
	files := []string{"a/util.js", "b/util.js", "main.js"}
	ext := Resolve(results, files)

	var es []edge
	for _, e := range ext.Edges {
		es = append(es, edge{e.Source, e.Target, e.Relation})
	}
	if !hasCall(es, "MAIN", "A") {
		t.Error("expected MAIN --calls--> A (the imported helper)")
	}
	if hasCall(es, "MAIN", "B") {
		t.Error("did not expect MAIN --calls--> B (not imported)")
	}
}

// TestResolveAmbiguousBySameDir checks the same-package (same directory)
// tiebreaker: an ambiguous name with one definition in the caller's directory
// resolves there, mirroring same-package calls in Go.
func TestResolveAmbiguousBySameDir(t *testing.T) {
	results := []Result{
		{Defs: []Def{{ID: "LOCAL", Name: "doIt", File: "pkg/a.go"}}},
		{Defs: []Def{{ID: "FAR", Name: "doIt", File: "other/b.go"}}},
		{
			Defs:  []Def{{ID: "CALLER", Name: "run", File: "pkg/c.go"}},
			Calls: []Call{{CallerID: "CALLER", Callee: "doIt", File: "pkg/c.go", Loc: "L3"}},
		},
	}
	files := []string{"pkg/a.go", "other/b.go", "pkg/c.go"}
	ext := Resolve(results, files)

	var es []edge
	for _, e := range ext.Edges {
		es = append(es, edge{e.Source, e.Target, e.Relation})
	}
	if !hasCall(es, "CALLER", "LOCAL") {
		t.Error("expected CALLER --calls--> LOCAL (same directory)")
	}
	if hasCall(es, "CALLER", "FAR") {
		t.Error("did not expect CALLER --calls--> FAR (different directory)")
	}
}
