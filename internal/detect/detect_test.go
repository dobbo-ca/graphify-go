package detect

import (
	"os"
	"path/filepath"
	"testing"
)

// .terraform holds the provider/module cache a `terraform init` downloads. It is
// always generated, never source, and can hold tens of thousands of vendored
// .tf files. It must be skipped by the built-in rule even when no .gitignore
// covers it — e.g. when graphify builds a subdirectory of a repo whose
// .terraform ignore rule lives in the repo-root .gitignore.
func TestCollectFilesSkipsTerraform(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.tf"), `resource "aws_x" "y" {}`)
	mustWrite(t, filepath.Join(root, ".terraform", "modules", "vendored.tf"), `resource "aws_z" "w" {}`)

	files, err := CollectFiles(root)
	if err != nil {
		t.Fatalf("CollectFiles: %v", err)
	}
	for _, f := range files {
		if filepath.ToSlash(f) != "main.tf" {
			t.Errorf("expected only main.tf, got %q", f)
		}
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d: %v", len(files), files)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
