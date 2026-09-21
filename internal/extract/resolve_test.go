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

// TestResolveNoCrossFamilyCall checks that a call whose name is uniquely defined
// only in a different language family never binds: a Python call to `process`
// must not resolve to a Java `process` method (a phantom name collision), while
// a call resolvable within its own family still binds.
func TestResolveNoCrossFamilyCall(t *testing.T) {
	results := []Result{
		{Defs: []Def{{ID: "JAVA", Name: "process", File: "Svc.java"}}},
		{Defs: []Def{{ID: "PYHELPER", Name: "helper", File: "util.py"}}},
		{
			Defs: []Def{{ID: "PYMAIN", Name: "run", File: "main.py"}},
			Calls: []Call{
				{CallerID: "PYMAIN", Callee: "process", File: "main.py", Loc: "L2"}, // cross-family, must not bind
				{CallerID: "PYMAIN", Callee: "helper", File: "main.py", Loc: "L3"},  // same-family, must bind
			},
		},
	}
	files := []string{"Svc.java", "util.py", "main.py"}
	ext := Resolve(results, files)

	var es []edge
	for _, e := range ext.Edges {
		es = append(es, edge{e.Source, e.Target, e.Relation})
	}
	if hasCall(es, "PYMAIN", "JAVA") {
		t.Error("did not expect PYMAIN --calls--> JAVA (cross language family)")
	}
	if !hasCall(es, "PYMAIN", "PYHELPER") {
		t.Error("expected PYMAIN --calls--> PYHELPER (same language family)")
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

// TestResolveSameNameMethodsInOneFile checks that when one file declares two
// types each owning a method of the same name, an unqualified call to that name
// binds to neither (drop-on-ambiguity) instead of the last one parsed, while an
// unambiguous same-file name still resolves.
func TestResolveSameNameMethodsInOneFile(t *testing.T) {
	results := []Result{{
		Defs: []Def{
			{ID: "x_a_build", Name: "Build", File: "x.cs"},
			{ID: "x_a_helper", Name: "Helper", File: "x.cs"},
			{ID: "x_b_helper", Name: "Helper", File: "x.cs"},
			{ID: "x_b_only", Name: "Only", File: "x.cs"},
		},
		Calls: []Call{
			{CallerID: "x_a_build", Callee: "Helper", File: "x.cs", Loc: "L3"},
			{CallerID: "x_a_build", Callee: "Only", File: "x.cs", Loc: "L4"},
		},
	}}
	ext := Resolve(results, []string{"x.cs"})

	var es []edge
	for _, e := range ext.Edges {
		es = append(es, edge{e.Source, e.Target, e.Relation})
	}
	if hasCall(es, "x_a_build", "x_b_helper") {
		t.Error("did not expect x_a_build --calls--> x_b_helper (ambiguous same-file name)")
	}
	if hasCall(es, "x_a_build", "x_a_helper") {
		t.Error("did not expect a guessed edge to x_a_helper either; the name is ambiguous")
	}
	if !hasCall(es, "x_a_build", "x_b_only") {
		t.Error("expected x_a_build --calls--> x_b_only (unambiguous same-file name)")
	}
}
