package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mergeDriverStatus must report "not registered" before install and "registered"
// after a full hookInstall wires both the config key and the .gitattributes line
// (#1902).
func TestMergeDriverStatus(t *testing.T) {
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	if got := mergeDriverStatus(repo); got != "not registered" {
		t.Errorf("before install: mergeDriverStatus = %q, want %q", got, "not registered")
	}
	if err := hookInstall(repo); err != nil {
		t.Fatalf("hookInstall: %v", err)
	}
	if got := mergeDriverStatus(repo); got != "registered" {
		t.Errorf("after install: mergeDriverStatus = %q, want %q", got, "registered")
	}
}

// Uninstalling the merge driver must strip only the graphify line and preserve
// other pre-existing .gitattributes entries (#1902).
func TestUninstallPreservesOtherAttributes(t *testing.T) {
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	attrPath := filepath.Join(repo, ".gitattributes")
	if err := os.WriteFile(attrPath, []byte("*.md text\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := hookInstall(repo); err != nil {
		t.Fatalf("hookInstall: %v", err)
	}
	if err := hookUninstall(repo); err != nil {
		t.Fatalf("hookUninstall: %v", err)
	}

	data, err := os.ReadFile(attrPath)
	if err != nil {
		t.Fatalf(".gitattributes should still exist after uninstall: %v", err)
	}
	if !strings.Contains(string(data), "*.md text") {
		t.Errorf("pre-existing entry lost after uninstall:\n%s", data)
	}
	if strings.Contains(string(data), mergeAttrLine) {
		t.Errorf("merge line still present after uninstall:\n%s", data)
	}
}

// When the graphify line is the only entry, uninstall must delete the
// .gitattributes file rather than leave an empty one behind (#1902).
func TestUninstallRemovesEmptyGitattributes(t *testing.T) {
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	attrPath := filepath.Join(repo, ".gitattributes")

	if err := hookInstall(repo); err != nil {
		t.Fatalf("hookInstall: %v", err)
	}
	if err := hookUninstall(repo); err != nil {
		t.Fatalf("hookUninstall: %v", err)
	}
	if _, err := os.Stat(attrPath); !os.IsNotExist(err) {
		t.Errorf(".gitattributes should be deleted when the merge line was its only entry, stat err = %v", err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func hookInstalled(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "post-commit"))
	return err == nil && strings.Contains(string(b), hookMarker)
}

// In-repo core.hooksPath is honoured; out-of-repo is refused and flagged inactive.
func TestHookInstallHonoursHooksPath(t *testing.T) {
	repo := t.TempDir()
	gitRun(t, repo, "init")
	gitRun(t, repo, "config", "core.hooksPath", ".husky")
	if err := hookInstall(repo); err != nil {
		t.Fatal(err)
	}
	if !hookInstalled(filepath.Join(repo, ".husky")) || hookInstalled(filepath.Join(repo, ".git", "hooks")) {
		t.Error("in-repo hooksPath not honoured")
	}

	out := t.TempDir()
	repo2 := t.TempDir()
	gitRun(t, repo2, "init")
	gitRun(t, repo2, "config", "core.hooksPath", out)
	if err := hookInstall(repo2); err != nil {
		t.Fatal(err)
	}
	if hookInstalled(out) || !hookInstalled(filepath.Join(repo2, ".git", "hooks")) {
		t.Error("out-of-repo hooksPath not refused")
	}
	_, eff, _ := resolveHooksDir(repo2)
	if eff != filepath.Clean(out) {
		t.Errorf("effective = %q, want %q", eff, out)
	}
}

// A linked worktree has a .git file; install must use the common-dir hooks.
func TestHookInstallLinkedWorktree(t *testing.T) {
	repo := t.TempDir()
	gitRun(t, repo, "init")
	gitRun(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "x")
	wt := filepath.Join(t.TempDir(), "wt")
	gitRun(t, repo, "worktree", "add", wt)
	if err := hookInstall(repo); err != nil {
		t.Fatal(err)
	}
	if err := hookInstall(wt); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(repo, ".git", "hooks")
	if !hookInstalled(hooks) {
		t.Error("worktree install did not write common-dir hooks")
	}
	b, _ := os.ReadFile(filepath.Join(hooks, "post-commit"))
	if strings.Contains(string(b), wt) || strings.Contains(string(b), repo) {
		t.Errorf("hook bakes in an install path: %s", b)
	}
}
