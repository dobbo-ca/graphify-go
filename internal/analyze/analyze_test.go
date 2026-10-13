package analyze

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
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
		g.AddEdge(model.Edge{Source: file, Target: child, Relation: model.RelContains, Confidence: model.ConfExtracted})
	}
	for _, id := range []string{"a1", "a2", "a3"} {
		contains("fa", id)
	}
	for _, id := range []string{"b1", "b2", "b3"} {
		contains("fb", id)
	}

	calls := func(s, t, conf string) {
		g.AddEdge(model.Edge{Source: s, Target: t, Relation: model.RelCalls, Confidence: model.Conf(conf)})
	}
	calls("a1", "a2", "EXTRACTED")
	calls("a1", "a3", "EXTRACTED")
	calls("b1", "b2", "EXTRACTED")
	calls("b1", "b3", "EXTRACTED")
	calls("a1", "b1", "INFERRED") // cross-file: surprising

	// File-level import cycle.
	g.AddEdge(model.Edge{Source: "fa", Target: "fb", Relation: model.RelImportsFrom, Confidence: model.ConfExtracted})
	g.AddEdge(model.Edge{Source: "fb", Target: "fa", Relation: model.RelImportsFrom, Confidence: model.ConfExtracted})
	return g
}

func TestGodNodesExcludeFileHubs(t *testing.T) {
	g := sampleGraph()
	gods := GodNodes(g, 10, 0)
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
	if got := GodNodes(g, 2, 0); len(got) != 2 {
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
	g.AddEdge(model.Edge{Source: "fa", Target: "fb", Relation: model.RelImportsFrom, Confidence: model.ConfExtracted})
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
		g.AddEdge(model.Edge{Source: s, Target: t, Relation: model.RelCites, Confidence: model.ConfInferred, ConfidenceScore: 0.8})
	}
	sem("note_a", "concept_appconfig")
	sem("note_b", "concept_appconfig")
	sem("note_a", "concept_flux")
	return g
}

func TestGodNodesIncludesSemanticConcepts(t *testing.T) {
	gods := GodNodes(semanticGraph(), 10, 0)
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
	g.AddEdge(model.Edge{Source: "na", Target: "c1", Relation: model.Rel("conceptually_related_to"), Confidence: model.ConfInferred, ConfidenceScore: 0.9})
	g.AddEdge(model.Edge{Source: "nb", Target: "c1", Relation: model.Rel("conceptually_related_to"), Confidence: model.ConfInferred, ConfidenceScore: 0.9})
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

func TestGodNodesExcludeNoiseLabels(t *testing.T) {
	g := sampleGraph()
	// A mechanical hub out-degreeing every real entity.
	g.AddNode(model.Node{ID: "dep", Label: "dependencies", SourceFile: "package.json"})
	for _, id := range []string{"a1", "a2", "a3", "b1", "b2", "b3"} {
		g.AddEdge(model.Edge{Source: "dep", Target: id, Relation: model.RelCalls, Confidence: model.ConfExtracted})
	}
	for _, n := range GodNodes(g, 10, 0) {
		if n.Label == "dependencies" {
			t.Errorf("god nodes should exclude noise label %q", n.Label)
		}
	}
	// A JSON key that isn't generic noise still ranks.
	g.AddNode(model.Node{ID: "sched", Label: "scheduler", SourceFile: "package.json"})
	g.AddEdge(model.Edge{Source: "sched", Target: "a1", Relation: model.RelCalls, Confidence: model.ConfExtracted})
	var found bool
	for _, n := range GodNodes(g, 20, 0) {
		if n.Label == "scheduler" {
			found = true
		}
	}
	if !found {
		t.Error("non-noise JSON key should still rank as a god node")
	}
}

// hubGraph is one mega-hub (degree 40), two mid symbols, and a tail of leaves.
func hubGraph() *model.Graph {
	g := model.New()
	g.AddNode(model.Node{ID: "hub", Label: "Registry", SourceFile: "u.go"})
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("leaf%d", i)
		g.AddNode(model.Node{ID: id, Label: id, SourceFile: fmt.Sprintf("l%d.go", i)})
		g.AddEdge(model.Edge{Source: "hub", Target: id, Relation: model.RelCalls, Confidence: model.ConfExtracted})
	}
	for _, m := range []struct {
		name string
		deg  int
	}{{"core", 6}, {"svc", 4}} {
		g.AddNode(model.Node{ID: m.name, Label: m.name, SourceFile: m.name + ".go"})
		for i := 0; i < m.deg; i++ {
			g.AddEdge(model.Edge{Source: m.name, Target: fmt.Sprintf("leaf%d", i), Relation: model.RelCalls, Confidence: model.ConfExtracted})
		}
	}
	return g
}

func TestGodNodesExcludeHubsPercentile(t *testing.T) {
	g := hubGraph()

	// Zero keeps the historical ranking: the hub still wins.
	if got := GodNodes(g, 3, 0); len(got) == 0 || got[0].Label != "Registry" {
		t.Fatalf("GodNodes(topN=3, pct=0) = %+v, want Registry first", got)
	}

	// The threshold must match cluster()'s formula: degrees sorted ascending,
	// idx = max(0, len*pct/100 - 1), anything with a greater degree is a hub.
	degrees := make([]int, 0, len(g.NodeIDs()))
	for _, id := range g.NodeIDs() {
		degrees = append(degrees, g.Degree(id))
	}
	sort.Ints(degrees)
	threshold := degrees[len(degrees)*90/100-1]

	gods := GodNodes(g, 100, 90)
	if len(gods) == 0 {
		t.Fatal("suppressing the hub must not empty the ranking")
	}
	for _, n := range gods {
		if n.Degree > threshold {
			t.Errorf("god node %q has degree %d, above the p90 threshold %d", n.Label, n.Degree, threshold)
		}
	}

	// 100 excludes nothing: the threshold is the maximum degree.
	if got := GodNodes(g, 3, 100); len(got) == 0 || got[0].Label != "Registry" {
		t.Errorf("GodNodes(topN=3, pct=100) = %+v, want Registry first", got)
	}
}

// godNodesBySort is GodNodes as it was: a stable sort of every node by degree.
func godNodesBySort(g *model.Graph, topN, pct int) []GodNode {
	ids := append([]string(nil), g.NodeIDs()...)
	hubThreshold, hasThreshold := 0, false
	if pct > 0 && len(ids) > 0 {
		degrees := make([]int, len(ids))
		for i, id := range ids {
			degrees[i] = g.Degree(id)
		}
		sort.Ints(degrees)
		idx := min(max(len(degrees)*pct/100-1, 0), len(degrees)-1)
		hubThreshold, hasThreshold = degrees[idx], true
	}
	sort.SliceStable(ids, func(i, j int) bool { return g.Degree(ids[i]) > g.Degree(ids[j]) })
	var out []GodNode
	for _, id := range ids {
		if hasThreshold && g.Degree(id) > hubThreshold {
			continue
		}
		if isFileNode(g, id) || isConceptNode(g, id) || isJSONKeyNode(g, id) {
			continue
		}
		out = append(out, GodNode{ID: id, Label: g.Nodes[id].Label, Degree: g.Degree(id)})
		if len(out) >= topN {
			break
		}
	}
	return out
}

func TestGodNodesMatchesStableSort(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for run := 0; run < 300; run++ {
		g := model.New()
		n := 1 + rng.Intn(60)
		for i := 0; i < n; i++ {
			node := model.Node{ID: fmt.Sprintf("n%02d", rng.Intn(1000)*100+i), Label: fmt.Sprintf("L%d", i), SourceFile: "a/x.go"}
			switch rng.Intn(6) {
			case 0:
				node.Label = "x.go" // file hub
			case 1:
				node.SourceFile = "" // concept
			case 2:
				node.Label += "()"
			}
			g.AddNode(node)
		}
		ids := g.NodeIDs()
		for i := rng.Intn(3 * n); i > 0; i-- {
			g.AddEdge(model.Edge{Source: ids[rng.Intn(n)], Target: ids[rng.Intn(n)], Relation: model.RelCalls, Confidence: model.ConfExtracted})
		}
		for _, topN := range []int{0, 1, 3, 10, 100} {
			for _, pct := range []int{0, 50, 90} {
				if got, want := GodNodes(g, topN, pct), godNodesBySort(g, topN, pct); !reflect.DeepEqual(got, want) {
					t.Fatalf("run %d topN=%d pct=%d:\n got %+v\nwant %+v", run, topN, pct, got, want)
				}
			}
		}
	}
}
