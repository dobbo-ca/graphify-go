package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// TestToHTMLNodeLevelHooks builds a tiny graph and checks that the node-level
// viewer emits the interactive hooks (search box, inspect panel, relation-grouped
// neighbours) and that edge confidence reaches the page as dashed/solid styling.
func TestToHTMLNodeLevelHooks(t *testing.T) {
	g := model.New()
	g.AddNode(model.Node{ID: "a", Label: "a()", FileType: "code", SourceFile: "x.go", SourceLocation: "L1"})
	g.AddNode(model.Node{ID: "b", Label: "b()", FileType: "code", SourceFile: "x.go", SourceLocation: "L9"})
	g.AddNode(model.Node{ID: "c", Label: "c()", FileType: "code", SourceFile: "y.go", SourceLocation: "L3"})
	g.AddEdge(model.Edge{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "c", Target: "a", Relation: "calls", Confidence: "INFERRED"})

	communities := map[int][]string{0: {"a", "b"}, 1: {"c"}}
	path := filepath.Join(t.TempDir(), "graph.html")
	if err := ToHTML(g, communities, path); err != nil {
		t.Fatalf("ToHTML: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	html := string(data)

	for _, want := range []string{
		`id="search"`,         // live search box
		`function showNode`,   // click-to-inspect panel
		`"nbrs"`,              // per-node grouped neighbours emitted
		`"Calls"`,             // outgoing relation group
		`"Called by"`,         // incoming relation group
		`"dashes":true`,       // INFERRED edge rendered dashed
		`const META=false`,    // node-level view, not the meta aggregate
		"3 nodes · 2 edges · 2 communities", // stats line
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
	}

	// EXTRACTED edges must not be dashed: with one EXTRACTED and one INFERRED
	// edge, exactly one "dashes":true should appear.
	if n := strings.Count(html, `"dashes":true`); n != 1 {
		t.Errorf(`"dashes":true count = %d, want 1 (only the INFERRED edge)`, n)
	}
}
