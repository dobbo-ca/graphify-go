package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkillIdempotentAndUninstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dest := filepath.Join(home, ".claude", "skills", "graphify", "SKILL.md")

	for i := 0; i < 2; i++ { // re-running is a no-op
		if err := cmdInstall(nil); err != nil {
			t.Fatalf("install: %v", err)
		}
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("read %s: %v", dest, err)
		}
		if !strings.Contains(string(got), "name: graphify") {
			t.Fatalf("installed skill missing frontmatter: %q", got[:min(80, len(got))])
		}
	}

	if err := cmdInstall([]string{"--uninstall"}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(dest)); !os.IsNotExist(err) {
		t.Fatalf("skill dir still present after uninstall: %v", err)
	}
	// uninstall removes only the graphify skill, leaving ~/.claude/skills intact
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills")); err != nil {
		t.Fatalf("skills dir removed: %v", err)
	}
}

func TestInstallRejectsUnknownArgs(t *testing.T) {
	if err := cmdInstall([]string{"--nope"}); err == nil {
		t.Fatal("expected error for unknown arg")
	}
}
