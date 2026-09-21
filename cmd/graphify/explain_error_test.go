package main

import (
	"fmt"
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

// Control characters in candidates are still stripped.
func TestExplainErrorSanitizesCandidates(t *testing.T) {
	amb := &query.AmbiguousError{Query: "X", Candidates: []string{"Bad\x00Label (a.go:1)"}}
	got := explainError("X", amb)
	if strings.ContainsRune(got, 0) {
		t.Errorf("explainError left a control character in %q", got)
	}
}
