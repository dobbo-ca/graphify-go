package detect

import (
	"path/filepath"
	"testing"
)

func collected(t *testing.T, root string) map[string]bool {
	t.Helper()
	files, err := CollectFiles(root)
	if err != nil {
		t.Fatalf("CollectFiles: %v", err)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[filepath.ToSlash(f)] = true
	}
	return got
}

// A directory excluded only via .git/info/exclude (not .gitignore) must be
// pruned from the scan, not fully indexed (#1810).
func TestInfoExcludeHonored(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".git", "info", "exclude"), "buildcache/\n")
	mustWrite(t, filepath.Join(root, "buildcache", "x.py"), "x = 1")
	mustWrite(t, filepath.Join(root, "app.py"), "def go(): pass")

	got := collected(t, root)
	if !got["app.py"] {
		t.Errorf("app.py should be collected, got %v", got)
	}
	if got["buildcache/x.py"] {
		t.Errorf("buildcache/x.py is excluded by .git/info/exclude and must be pruned, got %v", got)
	}
}

// When <root>/.git is a FILE (a linked worktree), info/exclude is resolved
// through gitdir: -> commondir to the shared git dir (#1809).
func TestInfoExcludeWorktreeGitFile(t *testing.T) {
	tmp := t.TempDir()
	main := filepath.Join(tmp, "main")
	wt := filepath.Join(tmp, "wt")

	// Shared git dir with the exclude, plus the worktree gitdir + commondir link.
	mustWrite(t, filepath.Join(main, ".git", "info", "exclude"), "excluded/\n")
	mustWrite(t, filepath.Join(main, ".git", "worktrees", "foo", "commondir"), "../..\n")

	// The worktree checkout: .git is a FILE pointing at the worktree gitdir.
	gitdir := filepath.Join(main, ".git", "worktrees", "foo")
	mustWrite(t, filepath.Join(wt, ".git"), "gitdir: "+gitdir+"\n")
	mustWrite(t, filepath.Join(wt, "excluded", "y.py"), "y = 1")
	mustWrite(t, filepath.Join(wt, "keep.py"), "def go(): pass")

	got := collected(t, wt)
	if !got["keep.py"] {
		t.Errorf("keep.py should be collected, got %v", got)
	}
	if got["excluded/y.py"] {
		t.Errorf("excluded/y.py (via shared .git/info/exclude) must be pruned, got %v", got)
	}
}

// info/exclude is lowest precedence: a nearer .gitignore negation re-includes a
// path it would otherwise exclude.
func TestInfoExcludePrecedence(t *testing.T) {
	root := t.TempDir()
	// Both dirs are excluded by info/exclude; only buildcache is re-included by
	// .gitignore. secretcache proves info/exclude is genuinely in effect (this
	// assertion fails if info/exclude support is removed); buildcache proves a
	// nearer .gitignore negation still overrides it.
	mustWrite(t, filepath.Join(root, ".git", "info", "exclude"), "buildcache/\nsecretcache/\n")
	mustWrite(t, filepath.Join(root, ".gitignore"), "!buildcache/\n")
	mustWrite(t, filepath.Join(root, "buildcache", "x.py"), "x = 1")
	mustWrite(t, filepath.Join(root, "secretcache", "y.py"), "y = 1")

	got := collected(t, root)
	if !got["buildcache/x.py"] {
		t.Errorf("a .gitignore negation must override .git/info/exclude, got %v", got)
	}
	if got["secretcache/y.py"] {
		t.Errorf("secretcache is excluded by info/exclude (not negated) and must be pruned, got %v", got)
	}
}
