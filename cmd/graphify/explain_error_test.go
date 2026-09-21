package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/query"
)

// A full 10-candidate ambiguous error must survive explainError intact: every
// candidate, the "(and N more)" tail, and the disambiguation hint.
func TestExplainErrorKeepsAllCandidates(t *testing.T) {
	amb := &query.AmbiguousError{Query: "Handler", More: 7}
	for i := 0; i < 10; i++ {
		amb.Candidates = append(amb.Candidates,
			fmt.Sprintf("Handler() (internal/extract/some/deeply/nested/package%d/handler.go:%d)", i, 120+i))
	}
	got := explainError("Handler", amb)
	for _, c := range amb.Candidates {
		if !strings.Contains(got, c) {
			t.Errorf("explainError dropped candidate %q\ngot: %s", c, got)
		}
	}
	if !strings.Contains(got, "(and 7 more)") {
		t.Errorf("explainError dropped the More count\ngot: %s", got)
	}
	if !strings.Contains(got, "disambiguate with path/to/file::Symbol") {
		t.Errorf("explainError dropped the disambiguation hint\ngot: %s", got)
	}
}

// Control characters in a node label are still stripped, now at the point
// query.ambiguous builds the *AmbiguousError rather than when explainError
// renders it.
func TestExplainErrorSanitizesCandidates(t *testing.T) {
	graphJSON := `{
  "directed": false, "multigraph": false, "graph": {},
  "nodes": [
    {"id":"a_x","label":"BadLabel\u0000","file_type":"code","source_file":"a.go","source_location":"L1","norm_label":"BadLabel\u0000"},
    {"id":"b_x","label":"BadLabel\u0000","file_type":"code","source_file":"b.go","source_location":"L1","norm_label":"BadLabel\u0000"}
  ],
  "links": []
}`
	p := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(p, []byte(graphJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := query.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	_, err = query.Explain(g, "BadLabel")
	var amb *query.AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("err = %v, want *AmbiguousError", err)
	}
	got := explainError("BadLabel", amb)
	if strings.ContainsRune(got, 0) {
		t.Errorf("explainError left a control character in %q", got)
	}
}
