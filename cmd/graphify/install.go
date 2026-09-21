package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dobbo-ca/graphify-go/skills"
)

// skillPath is where Claude Code looks for a user-level skill.
func skillPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "skills", "graphify", "SKILL.md"), nil
}

// cmdInstall handles `graphify install [--uninstall]`, copying the graphify
// skill into Claude Code's user skills directory (or removing it again) so the
// agent reaches for the graph before grep.
func cmdInstall(args []string) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "--uninstall") {
		return fmt.Errorf("usage: graphify install [--uninstall]")
	}
	path, err := skillPath()
	if err != nil {
		return err
	}
	if len(args) == 1 {
		if err := os.RemoveAll(filepath.Dir(path)); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", filepath.Dir(path))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(skills.Graphify), 0o644); err != nil {
		return err
	}
	fmt.Printf("installed skill to %s\n", path)
	return nil
}
