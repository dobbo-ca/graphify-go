package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveResultWritesMemoryDoc covers the frontmatter, body sections and the
// unique filename of a saved Q&A result.
func TestSaveResultWritesMemoryDoc(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	path, err := saveResult(dir, `Where is "Load" defined?`, "In internal/query.", "query", "useful", "it was query.Load", []string{"Load()", "query.go"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{
		`question: "Where is \"Load\" defined?"`,
		`type: "query"`,
		`contributor: "graphify"`,
		`outcome: "useful"`,
		`correction: "it was query.Load"`,
		`source_nodes: ["Load()", "query.go"]`,
		"## Answer\n\nIn internal/query.",
		"- Signal: useful",
		"- Correction: it was query.Load",
		"## Source Nodes\n\n- Load()\n- query.go",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("saved doc missing %q; got:\n%s", want, got)
		}
	}
	if name := filepath.Base(path); !strings.HasPrefix(name, "query_") || !strings.HasSuffix(name, "_where_is_load_defined.md") {
		t.Errorf("filename = %q, want query_<ts>_<hex>_<slug>.md", name)
	}

	// A second save of the same question in the same second is its own file.
	other, err := saveResult(dir, `Where is "Load" defined?`, "again", "query", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if other == path {
		t.Fatalf("concurrent saves collided on %s", path)
	}
}

// TestSaveResultOmitsOptionalSections keeps a minimal save minimal.
func TestSaveResultOmitsOptionalSections(t *testing.T) {
	path, err := saveResult(t.TempDir(), "q?", "a", "query", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	for _, unwanted := range []string{"outcome:", "correction:", "source_nodes:", "## Outcome", "## Source Nodes"} {
		if strings.Contains(string(body), unwanted) {
			t.Errorf("minimal save contains %q:\n%s", unwanted, body)
		}
	}
}

// TestCmdSaveResultFlags covers argument parsing, including the variadic
// --nodes list and the required/validated flags.
func TestCmdSaveResultFlags(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	defer os.Chdir(cwd)

	if err := cmdSaveResult([]string{"--question", "How?", "--answer=Like this", "--nodes", "a()", "b()", "--outcome", "dead_end"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(memoryDirPath)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ReadDir(%s) = %v, %v; want 1 file", memoryDirPath, entries, err)
	}
	body, _ := os.ReadFile(filepath.Join(memoryDirPath, entries[0].Name()))
	if !strings.Contains(string(body), `source_nodes: ["a()", "b()"]`) || !strings.Contains(string(body), "Like this") {
		t.Errorf("unexpected doc:\n%s", body)
	}

	for _, args := range [][]string{
		{"--answer", "a"},
		{"--question", "q"},
		{"--question", "q", "--answer", "a", "--outcome", "maybe"},
		{"--question", "q", "--answer", "a", "--bogus"},
		{"--question"},
	} {
		if err := cmdSaveResult(args); err == nil {
			t.Errorf("cmdSaveResult(%v) = nil, want error", args)
		}
	}
}
