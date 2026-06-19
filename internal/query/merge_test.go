package query

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "g.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// loadMerged reads back the JSON Merge wrote to current.
func loadMerged(t *testing.T, path string) *Graph {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var g Graph
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("merged output is not valid graph.json: %v", err)
	}
	return &g
}

func TestMergeUnionsNodesAndEdges(t *testing.T) {
	current := writeFile(t, `{"directed":true,"multigraph":false,"graph":{},
"nodes":[
 {"id":"a","label":"a()"},
 {"id":"b","label":"b()"}
],
"links":[
 {"source":"a","target":"b","relation":"calls","confidence":"INFERRED"}
]}`)
	other := writeFile(t, `{"directed":true,"multigraph":false,"graph":{},
"nodes":[
 {"id":"b","label":"b()"},
 {"id":"c","label":"c()"}
],
"links":[
 {"source":"b","target":"c","relation":"calls","confidence":"INFERRED"}
]}`)

	if err := Merge(current, other); err != nil {
		t.Fatal(err)
	}
	g := loadMerged(t, current)
	if len(g.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3 (a, b deduped, c)", len(g.Nodes))
	}
	if len(g.Links) != 2 {
		t.Errorf("links = %d, want 2", len(g.Links))
	}
}

func TestMergeKeepsBothDirectionsAndRelations(t *testing.T) {
	// a->b calls and b->a calls are distinct edges; a->b calls and a->b imports
	// are also distinct. None should collapse.
	current := writeFile(t, `{"directed":true,"multigraph":false,"graph":{},
"nodes":[{"id":"a","label":"a()"},{"id":"b","label":"b()"}],
"links":[{"source":"a","target":"b","relation":"calls","confidence":"INFERRED"}]}`)
	other := writeFile(t, `{"directed":true,"multigraph":false,"graph":{},
"nodes":[{"id":"a","label":"a()"},{"id":"b","label":"b()"}],
"links":[
 {"source":"b","target":"a","relation":"calls","confidence":"INFERRED"},
 {"source":"a","target":"b","relation":"imports","confidence":"INFERRED"},
 {"source":"a","target":"b","relation":"calls","confidence":"INFERRED"}
]}`)

	if err := Merge(current, other); err != nil {
		t.Fatal(err)
	}
	g := loadMerged(t, current)
	// a->b calls (deduped), b->a calls, a->b imports = 3.
	if len(g.Links) != 3 {
		t.Errorf("links = %d, want 3 (direction- and relation-aware dedup)", len(g.Links))
	}
}

func TestMergePreservesNodeAttributes(t *testing.T) {
	current := writeFile(t, `{"directed":true,"multigraph":false,"graph":{},
"nodes":[{"id":"a","label":"a()","community":2,"confidence_score":0.5}],
"links":[]}`)
	other := writeFile(t, `{"directed":true,"multigraph":false,"graph":{},"nodes":[],"links":[]}`)
	if err := Merge(current, other); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"community": 2`, `"confidence_score": 0.5`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("merged output lost attribute %q:\n%s", want, data)
		}
	}
}

func TestMergeRejectsCorruptInput(t *testing.T) {
	current := writeFile(t, `{"directed":true,"nodes":[],"links":[]}`)
	other := writeFile(t, `{not json`)
	if err := Merge(current, other); err == nil {
		t.Fatal("expected error on corrupt other graph, got nil")
	}
}

func TestMergeMissingFile(t *testing.T) {
	current := writeFile(t, `{"directed":true,"nodes":[],"links":[]}`)
	if err := Merge(current, filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error on missing other graph, got nil")
	}
}

func TestMergeRejectsTooManyNodes(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"directed":true,"multigraph":false,"graph":{},"nodes":[`)
	for i := 0; i <= mergeMaxNodes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"n%d","label":"n"}`, i)
	}
	b.WriteString(`],"links":[]}`)
	current := writeFile(t, b.String())
	other := writeFile(t, `{"directed":true,"nodes":[],"links":[]}`)
	err := Merge(current, other)
	if err == nil || !strings.Contains(err.Error(), "node cap") {
		t.Fatalf("expected node-cap error, got %v", err)
	}
}
