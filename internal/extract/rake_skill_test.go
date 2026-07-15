package extract

import (
	"os"
	"path/filepath"
	"testing"
)

func labelSet(r Result) map[string]bool {
	m := map[string]bool{}
	for _, n := range r.Nodes {
		m[n.Label] = true
	}
	return m
}

// A .rake file is plain Ruby and must route through the Ruby extractor, emitting
// the same symbol nodes as the identical content in a .rb file (#1784).
func TestExtractRakeUsesRubyExtractor(t *testing.T) {
	src := "def run\n  greet\nend\n\ndef greet\nend\n"
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.rake"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "build.rb"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	rake, err := File(root, "build.rake")
	if err != nil {
		t.Fatalf("File(build.rake): %v", err)
	}
	rb, err := File(root, "build.rb")
	if err != nil {
		t.Fatalf("File(build.rb): %v", err)
	}

	rakeLabels := labelSet(rake)
	if !rakeLabels["run()"] || !rakeLabels["greet()"] {
		t.Errorf(".rake did not extract ruby defs, got labels %v", rakeLabels)
	}
	// Every symbol the .rb equivalent produces (bar the differently-named file
	// node) must also come out of the .rake — proving identical routing.
	for l := range labelSet(rb) {
		if l == "build.rb" {
			continue
		}
		if !rakeLabels[l] {
			t.Errorf(".rake missing symbol %q that the .rb equivalent produced", l)
		}
	}
}

// A .skill file is Markdown (+YAML frontmatter) and must route through the
// markdown extractor, emitting heading nodes (#1901).
func TestExtractSkillUsesMarkdown(t *testing.T) {
	src := "---\nname: orch\n---\n# Orchestrator\n\n## Step One\n"
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orch.skill"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := File(root, "orch.skill")
	if err != nil {
		t.Fatalf("File(orch.skill): %v", err)
	}
	labels := labelSet(r)
	for _, want := range []string{"Orchestrator", "Step One"} {
		if !labels[want] {
			t.Errorf(".skill did not extract markdown heading %q, got %v", want, labels)
		}
	}
}
