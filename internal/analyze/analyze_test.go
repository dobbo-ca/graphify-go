package analyze

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// sampleGraph builds two files (a.go, b.go) each with three functions, file
// hubs that "contain" their functions, a cross-file inferred call a1->b1, and a
// mutual file-level import cycle a.go <-> b.go.
func sampleGraph() *model.Graph {
	g := model.New()
	g.AddNode(model.Node{ID: "fa", Label: "a.go", SourceFile: "pkg/a.go"})
	g.AddNode(model.Node{ID: "fb", Label: "b.go", SourceFile: "pkg/b.go"})
	for _, id := range []string{"a1", "a2", "a3"} {
		g.AddNode(model.Node{ID: id, Label: id + "()", SourceFile: "pkg/a.go"})
	}
	for _, id := range []string{"b1", "b2", "b3"} {
		g.AddNode(model.Node{ID: id, Label: id + "()", SourceFile: "pkg/b.go"})
	}

	contains := func(file, child string) {
		g.AddEdge(model.Edge{Source: file, Target: child, Relation: "contains", Confidence: "EXTRACTED"})
	}
	for _, id := range []string{"a1", "a2", "a3"} {
		contains("fa", id)
	}
	for _, id := range []string{"b1", "b2", "b3"} {
		contains("fb", id)
	}

	calls := func(s, t, conf string) {
		g.AddEdge(model.Edge{Source: s, Target: t, Relation: "calls", Confidence: conf})
	}
	calls("a1", "a2", "EXTRACTED")
	calls("a1", "a3", "EXTRACTED")
	calls("b1", "b2", "EXTRACTED")
	calls("b1", "b3", "EXTRACTED")
	calls("a1", "b1", "INFERRED") // cross-file: surprising

	// File-level import cycle.
	g.AddEdge(model.Edge{Source: "fa", Target: "fb", Relation: "imports_from", Confidence: "EXTRACTED"})
	g.AddEdge(model.Edge{Source: "fb", Target: "fa", Relation: "imports_from", Confidence: "EXTRACTED"})
	return g
}

func TestGodNodesExcludeFileHubs(t *testing.T) {
	g := sampleGraph()
	gods := GodNodes(g, 10)
	if len(gods) == 0 {
		t.Fatal("expected at least one god node")
	}
	for _, n := range gods {
		if n.Label == "a.go" || n.Label == "b.go" {
			t.Errorf("god nodes should exclude file hubs, got %q", n.Label)
		}
	}
	// Results are degree-sorted descending.
	for i := 1; i < len(gods); i++ {
		if gods[i-1].Degree < gods[i].Degree {
			t.Errorf("god nodes not sorted by descending degree: %d before %d", gods[i-1].Degree, gods[i].Degree)
		}
	}
	if got := GodNodes(g, 2); len(got) != 2 {
		t.Errorf("GodNodes(topN=2) returned %d, want 2", len(got))
	}
}

func TestSurprisingFindsCrossFileCall(t *testing.T) {
	g := sampleGraph()
	communities := map[int][]string{0: {"a1", "a2", "a3", "fa"}, 1: {"b1", "b2", "b3", "fb"}}
	surprises := Surprising(g, communities, 5)
	found := false
	for _, s := range surprises {
		if s.Source == "a1()" && s.Target == "b1()" && s.Relation == "calls" {
			found = true
			if s.Note == "" {
				t.Error("expected a community-bridging note on a1->b1")
			}
		}
		if s.SourceFiles[0] == s.SourceFiles[1] {
			t.Errorf("surprising connection should be cross-file, got %v", s.SourceFiles)
		}
	}
	if !found {
		t.Error("expected surprising connection a1() --calls--> b1()")
	}
}

func TestImportCyclesDetectsMutualImport(t *testing.T) {
	g := sampleGraph()
	cycles := ImportCycles(g, 5, 20)
	if len(cycles) != 1 {
		t.Fatalf("expected exactly 1 cycle, got %d: %v", len(cycles), cycles)
	}
	if len(cycles[0].Files) != 2 {
		t.Errorf("expected a 2-file cycle, got %v", cycles[0].Files)
	}
	files := map[string]bool{}
	for _, f := range cycles[0].Files {
		files[f] = true
	}
	if !files["pkg/a.go"] || !files["pkg/b.go"] {
		t.Errorf("cycle should contain both files, got %v", cycles[0].Files)
	}
}

func TestImportCyclesNoneWhenAcyclic(t *testing.T) {
	g := model.New()
	g.AddNode(model.Node{ID: "fa", Label: "a.go", SourceFile: "pkg/a.go"})
	g.AddNode(model.Node{ID: "fb", Label: "b.go", SourceFile: "pkg/b.go"})
	g.AddEdge(model.Edge{Source: "fa", Target: "fb", Relation: "imports_from", Confidence: "EXTRACTED"})
	if cycles := ImportCycles(g, 5, 20); len(cycles) != 0 {
		t.Errorf("expected no cycles in acyclic graph, got %v", cycles)
	}
}

// semanticGraph builds two prose notes that both link to one LLM-derived
// concept hub (file_type semantic_concept, no source file), plus a second
// concept. The concept hub should surface as a god node, and a note->concept
// edge should be eligible as a surprising connection.
func semanticGraph() *model.Graph {
	g := model.New()
	g.AddNode(model.Node{ID: "note_a", Label: "Note A", FileType: "document", SourceFile: "learnings/a.md"})
	g.AddNode(model.Node{ID: "note_b", Label: "Note B", FileType: "document", SourceFile: "learnings/b.md"})
	g.AddNode(model.Node{ID: "concept_appconfig", Label: "AppConfig", FileType: "semantic_concept"})
	g.AddNode(model.Node{ID: "concept_flux", Label: "Flux", FileType: "semantic_concept"})
	sem := func(s, t string) {
		g.AddEdge(model.Edge{Source: s, Target: t, Relation: "cites", Confidence: "INFERRED", ConfidenceScore: 0.8})
	}
	sem("note_a", "concept_appconfig")
	sem("note_b", "concept_appconfig")
	sem("note_a", "concept_flux")
	return g
}

func TestGodNodesIncludesSemanticConcepts(t *testing.T) {
	gods := GodNodes(semanticGraph(), 10)
	found := false
	for _, n := range gods {
		if n.ID == "concept_appconfig" {
			found = true
			if n.Degree != 2 {
				t.Errorf("AppConfig concept degree = %d, want 2", n.Degree)
			}
		}
	}
	if !found {
		t.Errorf("semantic concept hub not surfaced as a god node; got %+v", gods)
	}
}

func TestSurprisingIncludesNoteToConcept(t *testing.T) {
	g := semanticGraph()
	// Each note in its own community so a note->concept edge bridges them.
	communities := map[int][]string{0: {"note_a", "concept_flux"}, 1: {"note_b"}, 2: {"concept_appconfig"}}
	s := Surprising(g, communities, 5)
	if len(s) == 0 {
		t.Fatalf("no surprising connections surfaced for a semantically enriched graph")
	}
}

// fileHubNoteGraph mirrors the wiki corpus: prose-note nodes whose label is the
// file basename (so they look like file hubs) link to a shared concept. The
// note->concept semantic edge is the insight and must still surface as a
// surprising connection even though the note is a "file hub".
func fileHubNoteGraph() *model.Graph {
	g := model.New()
	g.AddNode(model.Node{ID: "na", Label: "a.md", FileType: "document", SourceFile: "learnings/a.md"})
	g.AddNode(model.Node{ID: "nb", Label: "b.md", FileType: "document", SourceFile: "learnings/b.md"})
	g.AddNode(model.Node{ID: "c1", Label: "Shared Concept", FileType: "semantic_concept"})
	g.AddEdge(model.Edge{Source: "na", Target: "c1", Relation: "conceptually_related_to", Confidence: "INFERRED", ConfidenceScore: 0.9})
	g.AddEdge(model.Edge{Source: "nb", Target: "c1", Relation: "conceptually_related_to", Confidence: "INFERRED", ConfidenceScore: 0.9})
	return g
}

func TestSurprisingIncludesSemanticEdgeFromFileHubNote(t *testing.T) {
	g := fileHubNoteGraph()
	communities := map[int][]string{0: {"na"}, 1: {"nb", "c1"}}
	s := Surprising(g, communities, 5)
	if len(s) == 0 {
		t.Fatalf("note->concept semantic edge not surfaced as surprising")
	}
}
