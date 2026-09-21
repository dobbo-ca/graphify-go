package analyze

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/extract"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// tsCycleGraph extracts and resolves a two-file TS corpus where a.ts imports
// from b.ts and b.ts imports back from a.ts, then builds the graph the report
// runs cycle detection over. aImport is the import line used in a.ts.
func tsCycleGraph(t *testing.T, aImport string) *model.Graph {
	t.Helper()
	srcs := map[string][]byte{
		"a.ts": []byte(aImport + "\nexport function mkA(): number { return 1; }\n"),
		"b.ts": []byte("import { mkA } from './a';\nexport class B { v = mkA(); }\n"),
	}
	files := []string{"a.ts", "b.ts"}
	var results []extract.Result
	for _, f := range files {
		results = append(results, extract.FileFromBytes(f, srcs[f]))
	}
	ext := extract.Resolve(results, files)

	g := model.New()
	for _, n := range ext.Nodes {
		g.AddNode(n)
	}
	for _, e := range ext.Edges {
		g.AddEdge(e)
	}
	return g
}

// A type-only import is erased at compile time, so the round trip it closes is
// not a runtime cycle; a value import in the same position still is.
func TestImportCyclesSkipsTypeOnlyImports(t *testing.T) {
	typeOnly := tsCycleGraph(t, "import type { B } from './b';")
	if cycles := ImportCycles(typeOnly, 5, 20); len(cycles) != 0 {
		t.Errorf("expected no cycles for a type-only import, got %v", cycles)
	}

	for _, imp := range []string{
		"import { B } from './b';",
		"import { type B, other } from './b';", // mixed: still a value import
	} {
		if cycles := ImportCycles(tsCycleGraph(t, imp), 5, 20); len(cycles) != 1 {
			t.Errorf("expected 1 cycle for %q, got %d: %v", imp, len(cycles), cycles)
		}
	}
}
