package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capture runs fn with os.Stdout redirected to a pipe and returns what it wrote.
func capture(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	runErr := fn()
	os.Stdout = old
	w.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	r.Close()
	return sb.String(), runErr
}

// godTestGraph: hub is connected to three leaves, spoke to one, so hub outranks
// spoke. file.go is a file node and must be excluded from the ranking.
const godTestGraph = `{"nodes":[
  {"id":"hub","label":"hub","file_type":"go","source_file":"file.go","source_location":"L1"},
  {"id":"spoke","label":"spoke","file_type":"go","source_file":"file.go","source_location":"L2"},
  {"id":"a","label":"a","file_type":"go","source_file":"file.go","source_location":"L3"},
  {"id":"b","label":"b","file_type":"go","source_file":"file.go","source_location":"L4"},
  {"id":"file.go","label":"file.go","file_type":"go","source_file":"file.go","source_location":"file"}],
 "links":[
  {"source":"hub","target":"spoke","relation":"calls"},
  {"source":"hub","target":"a","relation":"calls"},
  {"source":"hub","target":"b","relation":"calls"},
  {"source":"file.go","target":"hub","relation":"contains"},
  {"source":"file.go","target":"spoke","relation":"contains"},
  {"source":"file.go","target":"a","relation":"contains"},
  {"source":"file.go","target":"b","relation":"contains"}]}`

func TestCmdGodNodes(t *testing.T) {
	dir := t.TempDir()
	writeTestGraph(t, filepath.Join(dir, "graphify-out", "graph.json"), godTestGraph)
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	out, err := capture(t, func() error { return cmdGodNodes(nil) })
	if err != nil {
		t.Fatalf("cmdGodNodes() = %v, want success", err)
	}
	if !strings.Contains(out, "hub") {
		t.Errorf("cmdGodNodes() output = %q, want the hub node", out)
	}
	if strings.Contains(out, "file.go") {
		t.Errorf("cmdGodNodes() output = %q, want file hub excluded", out)
	}

	// --json emits a parseable array, and --top caps it.
	out, err = capture(t, func() error { return cmdGodNodes([]string{"--json", "--top", "1"}) })
	if err != nil {
		t.Fatalf("cmdGodNodes(--json --top 1) = %v, want success", err)
	}
	var gods []struct {
		ID     string `json:"id"`
		Label  string `json:"label"`
		Degree int    `json:"degree"`
	}
	if err := json.Unmarshal([]byte(out), &gods); err != nil {
		t.Fatalf("cmdGodNodes(--json) output %q: %v", out, err)
	}
	if len(gods) != 1 || gods[0].ID != "hub" || gods[0].Degree != 4 {
		t.Errorf("cmdGodNodes(--json --top 1) = %+v, want one hub node with degree 4", gods)
	}

	// --top=N form is equivalent; bad values and stray args are rejected.
	if _, err := capture(t, func() error { return cmdGodNodes([]string{"--top=2", "--json"}) }); err != nil {
		t.Errorf("cmdGodNodes(--top=2) = %v, want success", err)
	}
	for _, args := range [][]string{{"--top", "0"}, {"--top", "x"}, {"bogus"}} {
		if _, err := capture(t, func() error { return cmdGodNodes(args) }); err == nil {
			t.Errorf("cmdGodNodes(%v) = nil, want error", args)
		}
	}
}

// TestCmdGodNodesGraphOverride checks --graph selects an alternate graph and
// stays contained to graphify-out.
func TestCmdGodNodesGraphOverride(t *testing.T) {
	dir := t.TempDir()
	writeTestGraph(t, filepath.Join(dir, "graphify-out", "graph.json"), `{"nodes":[],"links":[]}`)
	writeTestGraph(t, filepath.Join(dir, "graphify-out", "alt.json"), godTestGraph)
	if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"nodes":[],"links":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	cwd, _ := os.Getwd()

	out, err := capture(t, func() error {
		return cmdGodNodes([]string{"--graph", filepath.Join(cwd, "graphify-out", "alt.json")})
	})
	if err != nil || !strings.Contains(out, "hub") {
		t.Errorf("cmdGodNodes(--graph alt) = %q, %v; want the alt graph's hub", out, err)
	}
	if _, err := capture(t, func() error { return cmdGodNodes([]string{"--graph", "../secrets.json"}) }); err == nil {
		t.Error("cmdGodNodes(--graph ../secrets.json) = nil, want containment error")
	}
}
