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
		g.AddEdge(model.Edge{Source: "fa", Target: id, Relation: "contains", Confidence: "EXTRACTED"})
	}
	for _, id := range []string{"b1", "b2", "b3"} {
		g.AddNode(model.Node{ID: id, Label: id + "()", SourceFile: "pkg/b.go"})
		g.AddEdge(model.Edge{Source: "fb", Target: id, Relation: "contains", Confidence: "EXTRACTED"})
	}
	g.AddEdge(model.Edge{Source: "a1", Target: "a2", Relation: "calls", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "b1", Target: "b2", Relation: "calls", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "a1", Target: "b1", Relation: "calls", Confidence: "INFERRED"})
	g.AddEdge(model.Edge{Source: "fa", Target: "fb", Relation: "imports_from", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "fb", Target: "fa", Relation: "imports_from", Confidence: "EXTRACTED"})
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
