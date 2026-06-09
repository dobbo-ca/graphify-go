package query

import (
	"os"
	"path/filepath"
	"testing"
)

func loadJSON(t *testing.T, content string) *Graph {
	t.Helper()
	dir := t.TempDir()
	// Load requires the path to resolve inside a graphify-out directory.
	out := filepath.Join(dir, "graphify-out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(out, "graph.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// graph: util.go defines add(); math.go defines compute() which calls add();
// main.go defines run() which calls compute(). Changing util.go should impact
// compute (caller of add) and run (caller of compute), transitively.
const affectedJSON = `{"directed":true,"multigraph":false,"graph":{},
"nodes":[
 {"id":"add","label":"add()","file_type":"code","source_file":"util.go","source_location":"L1"},
 {"id":"compute","label":"compute()","file_type":"code","source_file":"math.go","source_location":"L1"},
 {"id":"run","label":"run()","file_type":"code","source_file":"main.go","source_location":"L1"},
 {"id":"lonely","label":"lonely()","file_type":"code","source_file":"other.go","source_location":"L1"}
],
"links":[
 {"source":"compute","target":"add","relation":"calls","confidence":"INFERRED"},
 {"source":"run","target":"compute","relation":"calls","confidence":"INFERRED"}
],
"hyperedges":[]}`

func TestAffectedTransitive(t *testing.T) {
	g := loadJSON(t, affectedJSON)
	res := Affected(g, []string{"util.go"})

	if len(res.Changed) != 1 || res.Changed[0].ID != "add" {
		t.Fatalf("changed: got %+v, want [add]", res.Changed)
	}
	got := map[string]bool{}
	for _, n := range res.Impacted {
		got[n.ID] = true
	}
	if !got["compute"] || !got["run"] {
		t.Errorf("impacted should include compute and run (transitive callers), got %v", got)
	}
	if got["lonely"] {
		t.Error("lonely() is unrelated and must not be impacted")
	}
	if got["add"] {
		t.Error("changed nodes must not also appear in impacted")
	}
}

func TestAffectedNoMatch(t *testing.T) {
	g := loadJSON(t, affectedJSON)
	res := Affected(g, []string{"nonexistent.go"})
	if len(res.Changed) != 0 || len(res.Impacted) != 0 {
		t.Errorf("expected empty result for unknown file, got %+v", res)
	}
}
