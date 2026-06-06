package graph

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

func TestBuildDropsDanglingAndCrossLangCalls(t *testing.T) {
	ext := model.Extraction{
		Nodes: []model.Node{
			{ID: "a", Label: "a()", FileType: "code", SourceFile: "x.go"},
			{ID: "b", Label: "b()", FileType: "code", SourceFile: "y.ts"},
			{ID: "c", Label: "c()", FileType: "code", SourceFile: "z.go"},
		},
		Edges: []model.Edge{
			{Source: "a", Target: "c", Relation: "calls", Confidence: "INFERRED"},      // same family, kept
			{Source: "a", Target: "b", Relation: "calls", Confidence: "INFERRED"},      // cross-language, dropped
			{Source: "a", Target: "ghost", Relation: "calls", Confidence: "EXTRACTED"}, // dangling, dropped
		},
	}
	g := Build(ext)
	if g.NumNodes() != 3 {
		t.Fatalf("nodes = %d, want 3", g.NumNodes())
	}
	if !g.HasEdge("a", "c") {
		t.Error("same-language inferred call a->c was dropped")
	}
	if g.HasEdge("a", "b") {
		t.Error("cross-language inferred call a->b should be dropped")
	}
	if g.NumEdges() != 1 {
		t.Errorf("edges = %d, want 1 (only a->c)", g.NumEdges())
	}
}

func TestBuildKeepsBothDirections(t *testing.T) {
	ext := model.Extraction{
		Nodes: []model.Node{
			{ID: "a", Label: "a()", SourceFile: "x.go"},
			{ID: "b", Label: "b()", SourceFile: "x.go"},
		},
		Edges: []model.Edge{
			{Source: "a", Target: "b", Relation: "calls", Confidence: "INFERRED"},
			{Source: "b", Target: "a", Relation: "calls", Confidence: "INFERRED"},
		},
	}
	if g := Build(ext); g.NumEdges() != 2 {
		t.Errorf("edges = %d, want 2 (both directions kept)", g.NumEdges())
	}
}
