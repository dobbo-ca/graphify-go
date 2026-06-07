package export

import (
	"fmt"
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
	html := render(t, g, communities)

	for _, want := range []string{
		`id="search"`,         // live search box
		`function showNode`,   // click-to-inspect panel
		`"nbrs"`,              // per-node grouped neighbours emitted
		`"Calls"`,             // outgoing relation group
		`"Called by"`,         // incoming relation group
		`"dashes":true`,       // INFERRED edge rendered dashed
		`const META=false`,                  // node-level view, not the meta aggregate
		"3 nodes · 2 edges · 2 communities", // stats line
		`smooth:{type:"continuous"`,         // small graph keeps curved edges
		`iterations:300`,                    // and the full stabilization budget
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

// TestToHTMLMetaDrilldown builds a graph above metaThreshold and checks that the
// viewer opens on a named community overview that can drill into node-level
// subgraphs: community names, the full node-level data (SUB), and the drill-down
// hooks (openCommunity, the back button) must all be present.
func TestToHTMLMetaDrilldown(t *testing.T) {
	g := model.New()
	const n = metaThreshold + 10
	for i := 0; i < n; i++ {
		dir := "pkg/alpha"
		if i%2 == 1 {
			dir = "pkg/beta"
		}
		g.AddNode(model.Node{
			ID: fmt.Sprintf("n%d", i), Label: fmt.Sprintf("fn%d()", i),
			FileType: "code", SourceFile: fmt.Sprintf("%s/f%d.go", dir, i),
		})
	}
	// A couple of intra- and inter-community edges so SUB edges and a meta edge exist.
	g.AddEdge(model.Edge{Source: "n0", Target: "n2", Relation: "calls", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "n0", Target: "n1", Relation: "calls", Confidence: "INFERRED"})

	communities := map[int][]string{}
	for i := 0; i < n; i++ {
		c := i % 2
		communities[c] = append(communities[c], fmt.Sprintf("n%d", i))
	}
	html := render(t, g, communities)

	for _, want := range []string{
		`const META=true`,        // opens on the aggregate overview
		`function openCommunity`, // drill-down into a community subgraph
		`id="back"`,              // back-to-overview control
		`"pkg/alpha"`,            // community named by dominant directory (legend + meta label + NAME map)
		`"pkg/beta"`,
		`"from":"n0"`,    // node-level (SUB) edges emitted for the drill-down
		`smooth:false`,   // big graph drops curved edges for speed
		`iterations:150`, // and trims the stabilization budget
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
	}

	// The node-level data must be carried alongside the meta view so drilling has
	// something to show — every member node id should be present.
	if !strings.Contains(html, `"n499"`) {
		t.Error("node-level SUB data missing a member node")
	}
}

func render(t *testing.T, g *model.Graph, communities map[int][]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "graph.html")
	if err := ToHTML(g, communities, path); err != nil {
		t.Fatalf("ToHTML: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}
