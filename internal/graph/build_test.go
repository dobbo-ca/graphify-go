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

func TestBuildDropsCrossLangImportsKeepsConcept(t *testing.T) {
	ext := model.Extraction{
		Nodes: []model.Node{
			{ID: "worker", Label: "worker.rb", FileType: "code", SourceFile: "backend/worker.rb"},
			{ID: "time_ts", Label: "time.ts", FileType: "code", SourceFile: "src/time.ts"},
			{ID: "util_ts", Label: "util.ts", FileType: "code", SourceFile: "src/util.ts"},
			{ID: "pkg", Label: "package.json", FileType: "concept", SourceFile: "package.json"},
		},
		Edges: []model.Edge{
			// cross-language (ruby -> jsts), EXTRACTED: phantom, dropped
			{Source: "worker", Target: "time_ts", Relation: "imports_from", Confidence: "EXTRACTED"},
			// same family (jsts -> jsts): kept
			{Source: "time_ts", Target: "util_ts", Relation: "imports", Confidence: "EXTRACTED"},
			// concept (empty family) -> code: kept
			{Source: "pkg", Target: "time_ts", Relation: "references", Confidence: "EXTRACTED"},
		},
	}
	g := Build(ext)
	if g.HasEdge("worker", "time_ts") {
		t.Error("cross-language imports_from worker->time_ts should be dropped")
	}
	if !g.HasEdge("time_ts", "util_ts") {
		t.Error("same-family import time_ts->util_ts was dropped")
	}
	if !g.HasEdge("pkg", "time_ts") {
		t.Error("concept->code reference pkg->time_ts should be kept")
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

func TestBuildKeepsProvenanceOnIDCollision(t *testing.T) {
	ext := model.Extraction{
		Nodes: []model.Node{
			{ID: "model", Label: "Model Doc", FileType: "concept", SourceFile: "model.md", SourceLocation: "L1"},
			// external concept node for `import model`, appended after per-file nodes
			{ID: "model", Label: "model", FileType: "concept"},
		},
	}
	g := Build(ext)
	n := g.Nodes["model"]
	if n.SourceFile != "model.md" || n.Label != "Model Doc" || n.SourceLocation != "L1" {
		t.Errorf("collision erased provenance: %+v", *n)
	}
}

func TestBuildFillsMissingAttributesFromSameFile(t *testing.T) {
	ext := model.Extraction{
		Nodes: []model.Node{
			{ID: "a", Label: "a()", FileType: "code", SourceFile: "x.go", SourceLocation: "L7"},
			{ID: "a", Label: "a()", SourceFile: "x.go"},
		},
	}
	n := Build(ext).Nodes["a"]
	if n.FileType != "code" || n.SourceLocation != "L7" {
		t.Errorf("same-file re-add lost attributes: %+v", *n)
	}
}
