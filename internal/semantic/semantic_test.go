package semantic

import (
	"context"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/cache"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// fakeBackend records the notes it is asked to extract from and returns a
// canned set of nodes/edges per note, so the enrichment plumbing can be tested
// without a network call.
type fakeBackend struct {
	calls   []string // note IDs passed to Extract, in order
	result  func(n Note) ([]model.Node, []model.Edge)
	failOn  map[string]bool
	callCnt int
}

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Extract(_ context.Context, n Note) ([]model.Node, []model.Edge, error) {
	f.calls = append(f.calls, n.ID)
	f.callCnt++
	if f.failOn[n.ID] {
		return nil, nil, errContext("boom")
	}
	if f.result != nil {
		ns, es := f.result(n)
		return ns, es, nil
	}
	return nil, nil, nil
}

type errContext string

func (e errContext) Error() string { return string(e) }

// baseExtraction is a tiny deterministic core: one note file and one concept it
// links to, plus an isolated note with no edges.
func baseExtraction() model.Extraction {
	return model.Extraction{
		Nodes: []model.Node{
			{ID: "notes_appconfig_bake", Label: "AppConfig bake", FileType: "document", SourceFile: "learnings/appconfig-bake.md", SourceLocation: "L1"},
			{ID: "appconfig", Label: "AppConfig", FileType: "concept"},
			{ID: "notes_other", Label: "Other note", FileType: "document", SourceFile: "learnings/other.md", SourceLocation: "L1"},
		},
		Edges: []model.Edge{
			{Source: "notes_other", Target: "appconfig", Relation: "references", Confidence: "EXTRACTED"},
		},
	}
}

func notes() []Note {
	return []Note{
		{ID: "notes_appconfig_bake", File: "learnings/appconfig-bake.md", Content: "AppConfig managed strategies bake for 10 minutes."},
	}
}

func TestEnrichIsAdditive(t *testing.T) {
	base := baseExtraction()
	be := &fakeBackend{result: func(n Note) ([]model.Node, []model.Edge) {
		return []model.Node{{ID: "appconfig", Label: "AppConfig", FileType: "concept"}},
			[]model.Edge{{Source: n.ID, Target: "appconfig", Relation: "conceptually_related_to", Confidence: "INFERRED", ConfidenceScore: 0.8}}
	}}

	out, _, err := Enrich(context.Background(), Config{Backend: be}, base, notes(), nil)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	// Every deterministic node and edge must survive unchanged.
	if len(out.Nodes) < len(base.Nodes) {
		t.Fatalf("nodes shrank: got %d, base had %d", len(out.Nodes), len(base.Nodes))
	}
	for _, want := range base.Edges {
		found := false
		for _, got := range out.Edges {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("deterministic edge %+v was mutated or dropped", want)
		}
	}

	// The new semantic edge must be present.
	var sem int
	for _, e := range out.Edges {
		if e.Relation == "conceptually_related_to" {
			sem++
		}
	}
	if sem != 1 {
		t.Errorf("want 1 semantic edge, got %d", sem)
	}
}

func TestEnrichNeverDuplicatesAConceptNode(t *testing.T) {
	base := baseExtraction() // already has the appconfig concept node
	be := &fakeBackend{result: func(n Note) ([]model.Node, []model.Edge) {
		// Backend re-emits an existing node id with a different label; the merge
		// must not overwrite or duplicate the deterministic node.
		return []model.Node{{ID: "appconfig", Label: "DIFFERENT", FileType: "concept"}}, nil
	}}
	out, _, err := Enrich(context.Background(), Config{Backend: be}, base, notes(), nil)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	count := 0
	for _, n := range out.Nodes {
		if n.ID == "appconfig" {
			count++
			if n.Label != "AppConfig" {
				t.Errorf("deterministic node label overwritten: got %q", n.Label)
			}
		}
	}
	if count != 1 {
		t.Errorf("appconfig node count = %d, want 1 (no duplicate)", count)
	}
}

func TestEnrichCacheSkipsUnchangedNotes(t *testing.T) {
	base := baseExtraction()
	be := &fakeBackend{result: func(n Note) ([]model.Node, []model.Edge) {
		return nil, []model.Edge{{Source: n.ID, Target: "appconfig", Relation: "cites", Confidence: "INFERRED"}}
	}}

	// First run: cold cache, backend is called once and a cache is returned.
	_, c1, err := Enrich(context.Background(), Config{Backend: be}, base, notes(), nil)
	if err != nil {
		t.Fatalf("first Enrich: %v", err)
	}
	if be.callCnt != 1 {
		t.Fatalf("cold run: backend called %d times, want 1", be.callCnt)
	}

	// Second run with the same content + warm cache: backend not called again.
	out2, _, err := Enrich(context.Background(), Config{Backend: be}, base, notes(), c1)
	if err != nil {
		t.Fatalf("second Enrich: %v", err)
	}
	if be.callCnt != 1 {
		t.Errorf("warm run re-paid tokens: backend called %d times total, want 1", be.callCnt)
	}
	// The cached edge must still appear in the output.
	got := 0
	for _, e := range out2.Edges {
		if e.Relation == "cites" {
			got++
		}
	}
	if got != 1 {
		t.Errorf("cached semantic edge missing: got %d cites edges", got)
	}
}

func TestEnrichCacheRepaysChangedNote(t *testing.T) {
	base := baseExtraction()
	be := &fakeBackend{result: func(n Note) ([]model.Node, []model.Edge) { return nil, nil }}
	_, c1, _ := Enrich(context.Background(), Config{Backend: be}, base, notes(), nil)

	changed := notes()
	changed[0].Content = "completely different prose now"
	_, _, err := Enrich(context.Background(), Config{Backend: be}, base, changed, c1)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if be.callCnt != 2 {
		t.Errorf("changed note should re-pay tokens: backend called %d times, want 2", be.callCnt)
	}
}

func TestEnrichSkipsSensitiveAndOversizedNotes(t *testing.T) {
	base := baseExtraction()
	be := &fakeBackend{result: func(n Note) ([]model.Node, []model.Edge) { return nil, nil }}
	ns := []Note{
		{ID: "secret", File: "secrets/creds.md", Content: "token=abc"},
		{ID: "huge", File: "learnings/huge.md", Content: strings.Repeat("x", MaxNoteBytes+1)},
		{ID: "ok", File: "learnings/ok.md", Content: "fine"},
	}
	_, _, err := Enrich(context.Background(), Config{Backend: be}, base, ns, nil)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if len(be.calls) != 1 || be.calls[0] != "ok" {
		t.Errorf("sensitive/oversized notes were sent to the backend: calls=%v", be.calls)
	}
}

func TestEnrichDropsEdgesWithUnknownEndpoints(t *testing.T) {
	base := baseExtraction()
	be := &fakeBackend{result: func(n Note) ([]model.Node, []model.Edge) {
		// Edge to a target that is neither a deterministic node nor an emitted
		// concept node — must be dropped so it can't dangle.
		return nil, []model.Edge{{Source: n.ID, Target: "ghost_node", Relation: "cites", Confidence: "INFERRED"}}
	}}
	out, _, err := Enrich(context.Background(), Config{Backend: be}, base, notes(), nil)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	for _, e := range out.Edges {
		if e.Target == "ghost_node" {
			t.Errorf("dangling edge to ghost_node was kept")
		}
	}
}

func TestEnrichForcesInferredConfidence(t *testing.T) {
	base := baseExtraction()
	be := &fakeBackend{result: func(n Note) ([]model.Node, []model.Edge) {
		// A backend that wrongly claims EXTRACTED must be downgraded — semantic
		// edges are never EXTRACTED.
		return nil, []model.Edge{{Source: n.ID, Target: "appconfig", Relation: "cites", Confidence: "EXTRACTED"}}
	}}
	out, _, err := Enrich(context.Background(), Config{Backend: be}, base, notes(), nil)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	for _, e := range out.Edges {
		if e.Relation == "cites" && e.Confidence == "EXTRACTED" {
			t.Errorf("semantic edge kept EXTRACTED confidence; must be INFERRED or AMBIGUOUS")
		}
	}
}

func TestBackendErrorIsTolerated(t *testing.T) {
	base := baseExtraction()
	be := &fakeBackend{failOn: map[string]bool{"notes_appconfig_bake": true}}
	// A backend failure on one note must not abort the whole enrichment or
	// corrupt the deterministic core.
	out, _, err := Enrich(context.Background(), Config{Backend: be}, base, notes(), nil)
	if err != nil {
		t.Fatalf("a per-note backend error must not fail Enrich: %v", err)
	}
	if len(out.Nodes) != len(base.Nodes) {
		t.Errorf("node count changed after backend error: got %d, want %d", len(out.Nodes), len(base.Nodes))
	}
}

// compile-time assertions that the cache type round-trips through the shared
// cache package's hashing helper (the same content-hash pattern).
var _ = cache.HashBytes
