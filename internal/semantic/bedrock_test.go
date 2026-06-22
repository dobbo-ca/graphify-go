package semantic

import "testing"

// The Bedrock backend turns the model's structured tool output into graph
// fragments. parseConcepts is the pure core of that translation and is tested
// here without any network call.

func TestParseConceptsBuildsNodesAndEdges(t *testing.T) {
	raw := `{
		"concepts": [
			{"name": "AppConfig", "relation": "conceptually_related_to", "score": 0.9},
			{"name": "deployment bake time", "relation": "cites", "score": 0.7}
		]
	}`
	note := Note{ID: "notes_appconfig_bake", File: "learnings/appconfig-bake.md"}
	nodes, edges, err := parseConcepts(note, []byte(raw))
	if err != nil {
		t.Fatalf("parseConcepts: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want 2 concept nodes, got %d", len(nodes))
	}
	if len(edges) != 2 {
		t.Fatalf("want 2 edges, got %d", len(edges))
	}
	// Concept ids are stable (idutil), edges originate from the note, and the
	// score rides ConfidenceScore.
	if nodes[0].FileType != ConceptType {
		t.Errorf("node 0 file_type = %q, want %q", nodes[0].FileType, ConceptType)
	}
	for _, e := range edges {
		if e.Source != note.ID {
			t.Errorf("edge source = %q, want note id %q", e.Source, note.ID)
		}
		if e.ConfidenceScore == 0 {
			t.Errorf("edge %v lost its score", e)
		}
	}
	// An edge must point at the concept node id that parseConcepts minted.
	want := nodes[0].ID
	found := false
	for _, e := range edges {
		if e.Target == want {
			found = true
		}
	}
	if !found {
		t.Errorf("no edge targets the AppConfig concept node id %q", want)
	}
}

func TestParseConceptsDropsUnknownRelations(t *testing.T) {
	raw := `{"concepts": [{"name": "X", "relation": "is_a", "score": 1.0}]}`
	_, edges, err := parseConcepts(Note{ID: "n"}, []byte(raw))
	if err != nil {
		t.Fatalf("parseConcepts: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("unknown relation should yield no edge, got %d", len(edges))
	}
}

func TestParseConceptsToleratesGarbage(t *testing.T) {
	if _, _, err := parseConcepts(Note{ID: "n"}, []byte("not json")); err == nil {
		t.Errorf("malformed tool output should error so the note is skipped")
	}
}

func TestParseConceptsMarksLowScoreAmbiguous(t *testing.T) {
	raw := `{"concepts": [{"name": "Maybe", "relation": "semantically_similar_to", "score": 0.2}]}`
	_, edges, err := parseConcepts(Note{ID: "n"}, []byte(raw))
	if err != nil {
		t.Fatalf("parseConcepts: %v", err)
	}
	if len(edges) != 1 || edges[0].Confidence != "AMBIGUOUS" {
		t.Errorf("low-score edge should be AMBIGUOUS, got %+v", edges)
	}
}
