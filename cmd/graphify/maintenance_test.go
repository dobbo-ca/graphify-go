package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what it
// printed, so tests can assert on the machine-checkable hook status output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

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

func TestHookUninstall(t *testing.T) {
	root := t.TempDir()
	hooks := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := cmdHook([]string{"install", root}); err != nil {
		t.Fatalf("hook install: %v", err)
	}
	// A foreign hook (no marker) must survive uninstall.
	foreign := filepath.Join(hooks, "post-merge")
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\necho mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cmdHook([]string{"uninstall", root}); err != nil {
		t.Fatalf("hook uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hooks, "post-commit")); !os.IsNotExist(err) {
		t.Error("post-commit hook was not removed")
	}
	if got, _ := os.ReadFile(foreign); string(got) != "#!/bin/sh\necho mine\n" {
		t.Errorf("foreign post-merge hook was removed: %s", got)
	}

	// Uninstall is idempotent: a second run on an already-clean repo succeeds.
	if err := cmdHook([]string{"uninstall", root}); err != nil {
		t.Errorf("second uninstall: %v", err)
	}
}

func TestHookStatus(t *testing.T) {
	root := t.TempDir()
	hooks := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := cmdHook([]string{"status", root}); err != nil {
			t.Fatalf("hook status: %v", err)
		}
	})
	if !strings.Contains(out, "post-commit: not installed") {
		t.Errorf("before install, want post-commit not installed:\n%s", out)
	}

	if err := cmdHook([]string{"install", root}); err != nil {
		t.Fatalf("hook install: %v", err)
	}
	out = captureStdout(t, func() {
		if err := cmdHook([]string{"status", root}); err != nil {
			t.Fatalf("hook status: %v", err)
		}
	})
	if !strings.Contains(out, "post-commit: installed") {
		t.Errorf("after install, want post-commit installed:\n%s", out)
	}
}

func TestHookUnknownSubcommand(t *testing.T) {
	if err := cmdHook([]string{"bogus", t.TempDir()}); err == nil {
		t.Error("expected error for unknown hook subcommand")
	}
	if err := cmdHook(nil); err == nil {
		t.Error("expected error for missing hook subcommand")
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
	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build: %v", err)
	}

	if changed, err := watchTick(root); err != nil || changed {
		t.Errorf("no edit: changed=%v err=%v, want false/nil", changed, err)
	}

	write("a.go", "package p\n\nfunc A() {}\n\nfunc B() {}\n")
	if changed, _ := watchTick(root); !changed {
		t.Error("edited file: want changed=true")
	}

	if err := cmdUpdate([]string{root}); err != nil {
		t.Fatal(err)
	}
	write("c.go", "package p\n\nfunc C() {}\n")
	if changed, _ := watchTick(root); !changed {
		t.Error("added file: want changed=true")
	}
}
