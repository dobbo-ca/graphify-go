package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGraphJSON(t *testing.T, content string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "graphify-out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(out, "graph.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestValidateClean(t *testing.T) {
	p := writeGraphJSON(t, affectedJSON)
	issues, nodes, links, err := Validate(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
	if nodes != 4 || links != 2 {
		t.Errorf("counts: got %d nodes / %d links, want 4 / 2", nodes, links)
	}
}

const brokenJSON = `{"directed":true,"multigraph":false,"graph":{},
"nodes":[
 {"id":"a","label":"a()","file_type":"code","source_file":"x.go"},
 {"id":"a","label":"a-dup()","file_type":"code","source_file":"y.go"},
 {"id":"","label":"empty","file_type":"code","source_file":"z.go"}
],
"links":[
 {"source":"a","target":"ghost","relation":"calls","confidence":"INFERRED"}
],
"hyperedges":[]}`

func TestValidateFindsProblems(t *testing.T) {
	p := writeGraphJSON(t, brokenJSON)
	issues, _, _, err := Validate(p)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(issues, "\n")
	for _, want := range []string{"duplicate node id", "empty id", "target node missing"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected an issue mentioning %q, got:\n%s", want, joined)
		}
	}
}
