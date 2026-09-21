package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The post-checkout hook must skip the rebuild when HEAD did not move or when
// only files were checked out; post-commit/post-merge stay unconditional.
func TestPostCheckoutHookGuards(t *testing.T) {
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := hookInstall(repo); err != nil {
		t.Fatalf("hookInstall: %v", err)
	}

	read := func(h string) string {
		data, err := os.ReadFile(filepath.Join(repo, ".git", "hooks", h))
		if err != nil {
			t.Fatalf("read %s: %v", h, err)
		}
		return string(data)
	}
	if got := read("post-checkout"); !strings.Contains(got, postCheckoutGuards) {
		t.Errorf("post-checkout is missing the guards:\n%s", got)
	}
	for _, h := range []string{"post-commit", "post-merge"} {
		if got := read(h); strings.Contains(got, "exit 0") {
			t.Errorf("%s should not be guarded:\n%s", h, got)
		}
	}

	// The generated script must at least be valid shell.
	hook := filepath.Join(repo, ".git", "hooks", "post-checkout")
	if out, err := exec.Command("/bin/sh", "-n", hook).CombinedOutput(); err != nil {
		t.Errorf("post-checkout is not valid shell: %v: %s", err, out)
	}
}
