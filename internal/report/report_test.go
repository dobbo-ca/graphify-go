package report

import (
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

func reportGraph() *model.Graph {
	g := model.New()
	g.AddNode(model.Node{ID: "fa", Label: "a.go", SourceFile: "pkg/a.go"})
	g.AddNode(model.Node{ID: "fb", Label: "b.go", SourceFile: "pkg/b.go"})
	for _, id := range []string{"a1", "a2", "a3"} {
		g.AddNode(model.Node{ID: id, Label: id + "()", SourceFile: "pkg/a.go"})
		g.AddEdge(model.Edge{Source: "fa", Target: id, Relation: model.RelContains, Confidence: model.ConfExtracted})
	}
	for _, id := range []string{"b1", "b2", "b3"} {
		g.AddNode(model.Node{ID: id, Label: id + "()", SourceFile: "pkg/b.go"})
		g.AddEdge(model.Edge{Source: "fb", Target: id, Relation: model.RelContains, Confidence: model.ConfExtracted})
	}
	g.AddEdge(model.Edge{Source: "a1", Target: "a2", Relation: model.RelCalls, Confidence: model.ConfExtracted})
	g.AddEdge(model.Edge{Source: "b1", Target: "b2", Relation: model.RelCalls, Confidence: model.ConfExtracted})
	g.AddEdge(model.Edge{Source: "a1", Target: "b1", Relation: model.RelCalls, Confidence: model.ConfInferred})
	g.AddEdge(model.Edge{Source: "fa", Target: "fb", Relation: model.RelImportsFrom, Confidence: model.ConfExtracted})
	g.AddEdge(model.Edge{Source: "fb", Target: "fa", Relation: model.RelImportsFrom, Confidence: model.ConfExtracted})
	return g
}

func TestGenerateSections(t *testing.T) {
	g := reportGraph()
	communities := cluster.Cluster(g)
	out := Generate(g, communities, "testroot", "abcdef1234567")

	for _, want := range []string{
		"# Graph Report - testroot",
		"## Summary",
		"## God Nodes",
		"## Surprising Connections",
		"## Import Cycles",
		"## Communities",
		"## Graph Freshness",
		"Built from commit: `abcdef12`", // commit truncated to 8 chars
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing section/line %q", want)
		}
	}
}

func TestGenerateOmitsFreshnessWithoutCommit(t *testing.T) {
	g := reportGraph()
	communities := cluster.Cluster(g)
	out := Generate(g, communities, "testroot", "")
	if strings.Contains(out, "## Graph Freshness") {
		t.Error("freshness section should be omitted when no commit is provided")
	}
	if !strings.Contains(out, "nodes ·") {
		t.Error("summary line should always be present")
	}
}

func TestGenerateOmitsGapSectionsWhenClean(t *testing.T) {
	// A single fully-connected community of real nodes: no ambiguous edges, no
	// isolated nodes, no thin communities.
	g := model.New()
	g.AddNode(model.Node{ID: "fc", Label: "c.go", SourceFile: "pkg/c.go"})
	ids := []string{"c1", "c2", "c3"}
	for _, id := range ids {
		g.AddNode(model.Node{ID: id, Label: id + "()", SourceFile: "pkg/c.go"})
		g.AddEdge(model.Edge{Source: "fc", Target: id, Relation: model.RelContains, Confidence: model.ConfExtracted})
	}
	g.AddEdge(model.Edge{Source: "c1", Target: "c2", Relation: model.RelCalls, Confidence: model.ConfExtracted})
	g.AddEdge(model.Edge{Source: "c2", Target: "c3", Relation: model.RelCalls, Confidence: model.ConfExtracted})
	g.AddEdge(model.Edge{Source: "c3", Target: "c1", Relation: model.RelCalls, Confidence: model.ConfExtracted})

	out := Generate(g, cluster.Cluster(g), "testroot", "")
	for _, unwanted := range []string{"## Ambiguous Edges", "## Knowledge Gaps"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("clean graph should omit %q", unwanted)
		}
	}
}

func TestGenerateAmbiguousAndGapSections(t *testing.T) {
	g := reportGraph()
	// An AMBIGUOUS edge between two real nodes.
	g.AddEdge(model.Edge{Source: "a3", Target: "b3", Relation: model.Rel("uses"), Confidence: model.ConfAmbiguous, SourceFile: model.FileOf("pkg/a.go")})
	// An isolated real node (degree 1, real source file) hanging off its file hub.
	g.AddNode(model.Node{ID: "lonely", Label: "lonely()", SourceFile: "pkg/a.go"})
	g.AddEdge(model.Edge{Source: "fa", Target: "lonely", Relation: model.RelContains, Confidence: model.ConfExtracted})

	out := Generate(g, cluster.Cluster(g), "testroot", "")

	for _, want := range []string{
		"## Ambiguous Edges - Review These",
		"`a3()` --uses--> `b3()`  [AMBIGUOUS]",
		"pkg/a.go · relation: uses",
		"## Knowledge Gaps",
		"isolated node(s):",
		"`lonely()`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q", want)
		}
	}
}

func TestGenerateHighAmbiguityWarning(t *testing.T) {
	g := model.New()
	g.AddNode(model.Node{ID: "x", Label: "x()", SourceFile: "pkg/a.go"})
	g.AddNode(model.Node{ID: "y", Label: "y()", SourceFile: "pkg/a.go"})
	g.AddEdge(model.Edge{Source: "x", Target: "y", Relation: model.RelCalls, Confidence: model.ConfAmbiguous, SourceFile: model.FileOf("pkg/a.go")})

	out := Generate(g, cluster.Cluster(g), "testroot", "")
	if !strings.Contains(out, "High ambiguity: 100% of edges are AMBIGUOUS.") {
		t.Errorf("expected high-ambiguity warning, got:\n%s", out)
	}
}

func TestGenerateOverBudgetLine(t *testing.T) {
	g := reportGraph()
	communities := cluster.Cluster(g)
	base := Generate(g, communities, "testroot", "")
	const line = "- 2 file(s) skipped: parser made no progress (GRAPHIFY_PARSE_BUDGET)\n"
	if strings.Contains(base, "skipped") {
		t.Fatal("report without stopped files mentions skipped files")
	}
	g.OverBudget = 2
	out := Generate(g, communities, "testroot", "")
	if strings.Replace(out, line, "", 1) != base {
		t.Errorf("report with 2 stopped files is not the base plus one line:\n%s", out)
	}
	if !strings.Contains(out, line+"\n## ") {
		t.Error("the line is not the last of the Summary block")
	}
}
