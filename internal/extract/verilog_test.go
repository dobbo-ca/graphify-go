package extract

import "testing"

func TestExtractVerilog(t *testing.T) {
	root := "testdata/verilogproj"
	files := []string{"rtl/alu.sv"}

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
	for _, want := range []string{
		"alu.sv",         // file node
		"add()",          // top-level function
		"compute()",      // top-level function
		"Counter",        // class as a type
		"Counter.step()", // class method scoped under the type
		"alu",            // module as a type
		"defs.svh",       // external include node
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

	// Class method scoped under its type, with a contains edge.
	if !has("Counter", "contains", "Counter.step()") {
		t.Error("expected Counter --contains--> Counter.step()")
	}
	// Same-file call inside a function body: compute -> add.
	if !has("compute()", "calls", "add()") {
		t.Error("expected compute --calls--> add")
	}
	// Same-file call inside a method body: Counter.step -> compute.
	if !has("Counter.step()", "calls", "compute()") {
		t.Error("expected Counter.step --calls--> compute")
	}
	// `include "defs.svh"` recorded as an external import.
	if !has("alu.sv", "imports", "defs.svh") {
		t.Error("expected alu.sv --imports--> defs.svh")
	}
}
