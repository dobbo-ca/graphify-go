package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCmdAskNegativeBudget ensures a non-positive --budget is rejected at parse
// time rather than panicking the engine on a negative slice bound.
func TestCmdAskNegativeBudget(t *testing.T) {
	for _, args := range [][]string{
		{"q", "--budget", "-1"},
		{"q", "--budget=-1"},
		{"q", "--budget", "0"},
		{"q", "--budget=0"},
	} {
		err := cmdAsk(args)
		if err == nil || !strings.Contains(err.Error(), "positive integer") {
			t.Errorf("cmdAsk(%v) = %v, want positive-integer error", args, err)
		}
	}
}

// TestCmdAskGraphTraversal ensures a user-supplied --graph path cannot escape the
// graphify-out directory to read arbitrary on-disk JSON.
func TestCmdAskGraphTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "graphify-out"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "secrets.json")
	if err := os.WriteFile(secret, []byte(`{"nodes":[],"links":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	// Use the canonical cwd so containment checks match across symlinked tmpdirs.
	cwd, _ := os.Getwd()

	for _, p := range []string{secret, "../secrets.json", filepath.Join(cwd, "secrets.json")} {
		if err := cmdAsk([]string{"q", "--graph", p}); err == nil {
			t.Errorf("cmdAsk --graph %q = nil, want containment error", p)
		}
	}

	// A path inside graphify-out must clear the containment guard.
	inside := filepath.Join(cwd, "graphify-out", "alt.json")
	if err := os.WriteFile(inside, []byte(`{"nodes":[],"links":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdAsk([]string{"q", "--graph", inside}); err != nil {
		t.Errorf("cmdAsk --graph %q (inside graphify-out) = %v, want success", inside, err)
	}
}
