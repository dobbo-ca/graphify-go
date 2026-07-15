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
