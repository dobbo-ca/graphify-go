package extract

import "testing"

func TestExtractJulia(t *testing.T) {
	root := "testdata/juliaproj"
	files := []string{"math.jl", "geometry.jl"}

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
		"math.jl", "geometry.jl",
		"MathUtils", "MathUtils.square()", "cube()",
		"Circle", "Shapes", "Shapes.area()", "Shapes.scale()",
		"LinearAlgebra", "Printf",
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

	// Module function scoped under its module, with a contains edge.
	if !has("Shapes", "contains", "Shapes.area()") {
		t.Error("expected Shapes --contains--> Shapes.area()")
	}
	// Same-file call: Shapes.area -> Shapes.scale.
	if !has("Shapes.area()", "calls", "Shapes.scale()") {
		t.Error("expected Shapes.area --calls--> Shapes.scale")
	}
	// Cross-file call: Shapes.scale -> MathUtils.square (unique global square).
	if !has("Shapes.scale()", "calls", "MathUtils.square()") {
		t.Error("expected cross-file Shapes.scale --calls--> MathUtils.square")
	}

	// `using LinearAlgebra` records an external import.
	hasImport := func(spec string) bool {
		for _, e := range ext.Edges {
			if e.Relation == "imports" && id2label[e.Target] == spec {
				return true
			}
		}
		return false
	}
	if !hasImport("LinearAlgebra") {
		t.Error("expected import of LinearAlgebra")
	}
}
