package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hook install must register the graph.json union merge driver (config key +
// .gitattributes line), do so idempotently, and hook uninstall must remove both
// (#1902).
func TestHookRegistersMergeDriver(t *testing.T) {
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	attrPath := filepath.Join(repo, ".gitattributes")

	if err := hookInstall(repo); err != nil {
		t.Fatalf("hookInstall: %v", err)
	}

	// merge.graphify.driver config points at graphify's merge-driver command.
	out, err := exec.Command("git", "-C", repo, "config", "--get", "merge.graphify.driver").Output()
	if err != nil || !strings.Contains(string(out), "merge-driver") {
		t.Errorf("merge.graphify.driver not registered: out=%q err=%v", out, err)
	}
	// .gitattributes carries the graph.json line exactly once.
	data, err := os.ReadFile(attrPath)
	if err != nil {
		t.Fatalf("read .gitattributes: %v", err)
	}
	if c := strings.Count(string(data), mergeAttrLine); c != 1 {
		t.Errorf(".gitattributes has %d copies of the merge line, want 1:\n%s", c, data)
	}

	// Idempotent: reinstall must not duplicate the .gitattributes line.
	if err := hookInstall(repo); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	data, _ = os.ReadFile(attrPath)
	if c := strings.Count(string(data), mergeAttrLine); c != 1 {
		t.Errorf("after reinstall .gitattributes has %d copies, want 1:\n%s", c, data)
	}

	// Uninstall removes both the config key and the .gitattributes line.
	if err := hookUninstall(repo); err != nil {
		t.Fatalf("hookUninstall: %v", err)
	}
	if err := exec.Command("git", "-C", repo, "config", "--get", "merge.graphify.driver").Run(); err == nil {
		t.Error("merge.graphify.driver still set after uninstall")
	}
	data, _ = os.ReadFile(attrPath)
	if strings.Contains(string(data), mergeAttrLine) {
		t.Errorf(".gitattributes still has the merge line after uninstall:\n%s", data)
	}
}

// Registering the merge driver must preserve pre-existing .gitattributes entries.
func TestMergeDriverPreservesExistingAttributes(t *testing.T) {
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
	data, _ := os.ReadFile(attrPath)
	if !strings.Contains(string(data), "*.md text") {
		t.Errorf("existing .gitattributes entry was lost:\n%s", data)
	}
	if !strings.Contains(string(data), mergeAttrLine) {
		t.Errorf("merge line not appended:\n%s", data)
	}
}
