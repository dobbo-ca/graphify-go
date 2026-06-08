package detect

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestCollectFilesGitignore verifies CollectFiles honours .gitignore the way
// git does. The expected sets were cross-checked against `git check-ignore`.
func TestCollectFilesGitignore(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(".gitignore", "build/\n/rootonly.go\ngen_*.go\nvendored/**\n!vendored/keep.go\n")
	write("sub/.gitignore", "*.py\n")

	for _, f := range []string{
		"keep.go", "rootonly.go", "gen_a.go", "build/b.go",
		"sub/rootonly.go", "sub/c.go", "sub/d.py",
		"vendored/v.go", "vendored/keep.go",
	} {
		write(f, "package x\n")
	}

	got, err := CollectFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	gotSet := map[string]bool{}
	for _, f := range got {
		gotSet[filepath.ToSlash(f)] = true
	}

	want := []string{"keep.go", "sub/rootonly.go", "sub/c.go", "vendored/keep.go"}
	ignored := []string{"rootonly.go", "gen_a.go", "build/b.go", "sub/d.py", "vendored/v.go"}

	for _, w := range want {
		if !gotSet[w] {
			t.Errorf("expected %q to be collected, but it was excluded", w)
		}
	}
	for _, ig := range ignored {
		if gotSet[ig] {
			t.Errorf("expected %q to be gitignored, but it was collected", ig)
		}
	}
	if len(got) != len(want) {
		keys := make([]string, 0, len(gotSet))
		for k := range gotSet {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Errorf("collected %d files, want %d: %s", len(got), len(want), strings.Join(keys, ", "))
	}
}
