package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// callflowSampleJSON has two communities, each with one internal call, plus a
// cross-community call that must be excluded. A label carries a `<` to exercise
// sanitisation.
const callflowSampleJSON = `{"directed":true,"multigraph":false,"graph":{},
"nodes":[
 {"id":"a","label":"add()","file_type":"code","source_file":"util/math.go","community":0,"norm_label":"add()"},
 {"id":"b","label":"compute()","file_type":"code","source_file":"util/math.go","community":0,"norm_label":"compute()"},
 {"id":"c","label":"Serve()","file_type":"code","source_file":"web/server.go","community":1,"norm_label":"serve()"},
 {"id":"d","label":"handle<T>()","file_type":"code","source_file":"web/server.go","community":1,"norm_label":"handle"}
],
"links":[
 {"source":"b","target":"a","relation":"calls","confidence":"INFERRED","confidence_score":0.5},
 {"source":"c","target":"d","relation":"calls","confidence":"INFERRED","confidence_score":0.5},
 {"source":"a","target":"c","relation":"calls","confidence":"INFERRED","confidence_score":0.5}
],
"hyperedges":[]}`

func TestCallflowFromJSON(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(jsonPath, []byte(callflowSampleJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "graph.callflow.html")
	if err := CallflowFromJSON(jsonPath, out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	s := string(data)

	// Two intra-community calls only; the cross-community a->c is excluded.
	if !strings.Contains(s, "2 call edges") {
		t.Errorf("expected '2 call edges' in header, got:\n%s", firstLines(s, 12))
	}
	if n := strings.Count(s, `<pre class="mermaid">`); n != 2 {
		t.Errorf("expected 2 mermaid diagrams, got %d", n)
	}
	if !strings.Contains(s, "flowchart LR") {
		t.Error("missing mermaid flowchart")
	}
	// Community labels from the dominant directory.
	if !strings.Contains(s, ">util ") || !strings.Contains(s, ">web ") {
		t.Error("expected util and web community labels")
	}
	// The '<' in a node label must be sanitised out of the mermaid source.
	if strings.Contains(s, "handle<T>") {
		t.Error("node label was not sanitised for mermaid/HTML")
	}
	if !strings.Contains(s, "handle(T)()") {
		t.Errorf("expected sanitised label handle(T)(), got:\n%s", s)
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
