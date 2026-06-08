package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookInstall(t *testing.T) {
	root := t.TempDir()
	hooks := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	// A pre-existing, non-graphify hook must not be clobbered.
	foreign := filepath.Join(hooks, "post-merge")
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\necho mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cmdHook([]string{"install", root}); err != nil {
		t.Fatalf("hook install: %v", err)
	}

	pc := filepath.Join(hooks, "post-commit")
	data, err := os.ReadFile(pc)
	if err != nil {
		t.Fatalf("post-commit not written: %v", err)
	}
	if !strings.Contains(string(data), hookMarker) || !strings.Contains(string(data), "update") {
		t.Errorf("post-commit missing marker or update call:\n%s", data)
	}
	if fi, _ := os.Stat(pc); fi.Mode()&0o111 == 0 {
		t.Error("post-commit is not executable")
	}
	if got, _ := os.ReadFile(foreign); string(got) != "#!/bin/sh\necho mine\n" {
		t.Errorf("foreign post-merge hook was overwritten: %s", got)
	}
}

func TestHookInstallRejectsNonRepo(t *testing.T) {
	if err := cmdHook([]string{"install", t.TempDir()}); err == nil {
		t.Error("expected error installing hooks outside a git repo")
	}
}

func TestWatchTick(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package p\n\nfunc A() {}\n")
	if err := cmdBuild(root); err != nil {
		t.Fatalf("build: %v", err)
	}

	if changed, err := watchTick(root); err != nil || changed {
		t.Errorf("no edit: changed=%v err=%v, want false/nil", changed, err)
	}

	write("a.go", "package p\n\nfunc A() {}\n\nfunc B() {}\n")
	if changed, _ := watchTick(root); !changed {
		t.Error("edited file: want changed=true")
	}

	if err := cmdUpdate(root); err != nil {
		t.Fatal(err)
	}
	write("c.go", "package p\n\nfunc C() {}\n")
	if changed, _ := watchTick(root); !changed {
		t.Error("added file: want changed=true")
	}
}
