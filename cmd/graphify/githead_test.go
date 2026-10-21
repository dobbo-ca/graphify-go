package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadGitHeadMatchesGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	for _, k := range gitEnv {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	commit := func(dir string) {
		gitRun(t, dir, "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "x")
	}
	newRepo := func() string {
		d := t.TempDir()
		gitRun(t, d, "init", "-q", "-b", "main")
		commit(d)
		return d
	}
	tests := []struct {
		name  string
		setup func() string
	}{
		{"branch", newRepo},
		{"packed refs", func() string {
			d := newRepo()
			gitRun(t, d, "pack-refs", "--all")
			os.Remove(filepath.Join(d, ".git", "refs", "heads", "main"))
			return d
		}},
		{"detached", func() string {
			d := newRepo()
			gitRun(t, d, "checkout", "-q", "--detach")
			return d
		}},
		{"worktree branch", func() string {
			d := newRepo()
			w := filepath.Join(t.TempDir(), "w")
			gitRun(t, d, "worktree", "add", "-q", "-b", "other", w)
			commit(w)
			return w
		}},
		{"worktree detached", func() string {
			d := newRepo()
			w := filepath.Join(t.TempDir(), "w")
			gitRun(t, d, "worktree", "add", "-q", "--detach", w)
			return w
		}},
		{"subdirectory", func() string {
			d := newRepo()
			sub := filepath.Join(d, "a", "b")
			os.MkdirAll(sub, 0o755)
			return sub
		}},
		{"unborn", func() string {
			d := t.TempDir()
			gitRun(t, d, "init", "-q", "-b", "main")
			return d
		}},
		{"symbolic ref chain", func() string {
			d := newRepo()
			gitRun(t, d, "symbolic-ref", "refs/heads/alias", "refs/heads/main")
			gitRun(t, d, "symbolic-ref", "HEAD", "refs/heads/alias")
			return d
		}},
		{"no repo", func() string { return t.TempDir() }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.setup()
			want := ""
			if out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output(); err == nil {
				want = strings.TrimSpace(string(out))
			}
			got, ok := readGitHead(dir)
			if !ok {
				t.Fatalf("reader fell back, want in-process answer %q", want)
			}
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// Odd layouts may fall back to git, but must never disagree with it.
func TestReadGitHeadNeverDisagrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	for _, k := range gitEnv {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	newRepo := func() string {
		d := t.TempDir()
		d, _ = filepath.EvalSymlinks(d)
		gitRun(t, d, "init", "-q", "-b", "main")
		gitRun(t, d, "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "x")
		return d
	}
	tests := []struct {
		name  string
		setup func() string
	}{
		{"symlink into other repo", func() string {
			a, b := newRepo(), newRepo()
			os.Mkdir(filepath.Join(b, "sub"), 0o755)
			os.Symlink(filepath.Join(b, "sub"), filepath.Join(a, "link"))
			return filepath.Join(a, "link")
		}},
		{"symlink out of repo", func() string {
			a := newRepo()
			os.Mkdir(filepath.Join(a, "sub"), 0o755)
			l := filepath.Join(t.TempDir(), "link")
			os.Symlink(filepath.Join(a, "sub"), l)
			return l
		}},
		{"nested dotgit without objects", func() string {
			d := newRepo()
			os.MkdirAll(filepath.Join(d, "sub", ".git", "refs"), 0o755)
			os.WriteFile(filepath.Join(d, "sub", ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
			return filepath.Join(d, "sub")
		}},
		{"format version 2", func() string {
			d := newRepo()
			gitRun(t, d, "config", "core.repositoryformatversion", "2")
			return d
		}},
		{"unknown extension", func() string {
			d := newRepo()
			gitRun(t, d, "config", "core.repositoryformatversion", "1")
			gitRun(t, d, "config", "extensions.bogusext", "true")
			return d
		}},
		{"ref chain too deep", func() string {
			d := newRepo()
			gitRun(t, d, "update-ref", "refs/heads/e", "HEAD")
			prev := "e"
			for _, n := range []string{"d", "c", "b", "a"} {
				os.WriteFile(filepath.Join(d, ".git", "refs", "heads", n), []byte("ref: refs/heads/"+prev+"\n"), 0o644)
				prev = n
			}
			os.WriteFile(filepath.Join(d, ".git", "HEAD"), []byte("ref: refs/heads/a\n"), 0o644)
			return d
		}},
		{"worktree admin dir", func() string {
			d := newRepo()
			w := filepath.Join(t.TempDir(), "w")
			gitRun(t, d, "worktree", "add", "-q", "-b", "other", w)
			return filepath.Join(d, ".git", "worktrees", "w")
		}},
		{"64 hex head", func() string {
			d := newRepo()
			h, _ := os.ReadFile(filepath.Join(d, ".git", "refs", "heads", "main"))
			os.WriteFile(filepath.Join(d, ".git", "HEAD"), []byte(strings.TrimSpace(string(h))+strings.Repeat("0", 24)+"\n"), 0o644)
			return d
		}},
		{"leading space head in nested repo", func() string {
			d := newRepo()
			in := filepath.Join(d, "in")
			os.Mkdir(in, 0o755)
			gitRun(t, in, "init", "-q", "-b", "main")
			gitRun(t, in, "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "y")
			os.WriteFile(filepath.Join(in, ".git", "HEAD"), []byte(" ref: refs/heads/main\n"), 0o644)
			return in
		}},
		{"gitfile without space", func() string {
			a := newRepo()
			f := t.TempDir()
			os.WriteFile(filepath.Join(f, ".git"), []byte("gitdir:"+filepath.Join(a, ".git")+"\n"), 0o644)
			return f
		}},
		{"format version 09", func() string {
			d := newRepo()
			gitRun(t, d, "config", "core.repositoryformatversion", "09")
			return d
		}},
		{"bad refname", func() string {
			d := newRepo()
			os.WriteFile(filepath.Join(d, ".git", "HEAD"), []byte("ref: refs/heads//main\n"), 0o644)
			return d
		}},
		{"symlink ref", func() string {
			d := newRepo()
			gitRun(t, d, "branch", "b")
			gitRun(t, d, "pack-refs", "--all")
			os.Symlink("refs/heads/b", filepath.Join(d, ".git", "refs", "heads", "a"))
			os.WriteFile(filepath.Join(d, ".git", "HEAD"), []byte("ref: refs/heads/a\n"), 0o644)
			return d
		}},
		{"gitfile to non-repo", func() string {
			d := t.TempDir()
			fake := filepath.Join(d, "fake")
			os.Mkdir(fake, 0o755)
			os.WriteFile(filepath.Join(fake, "HEAD"), []byte("0123456789abcdef0123456789abcdef01234567\n"), 0o644)
			w := filepath.Join(d, "w")
			os.Mkdir(w, 0o755)
			os.WriteFile(filepath.Join(w, ".git"), []byte("gitdir: "+fake+"\n"), 0o644)
			return w
		}},
		{"bare repo", func() string {
			d := newRepo()
			b := filepath.Join(t.TempDir(), "b.git")
			gitRun(t, d, "clone", "-q", "--bare", d, b)
			return b
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.setup()
			want := ""
			if out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output(); err == nil {
				want = strings.TrimSpace(string(out))
			}
			if got := gitHead(dir); got != want {
				t.Errorf("gitHead %q, want %q", got, want)
			}
			if got, ok := readGitHead(dir); ok && got != want {
				t.Errorf("reader %q, want %q", got, want)
			}
		})
	}
}

func TestReadGitHeadFallsBackOnRefBackendEnv(t *testing.T) {
	t.Setenv("GIT_REFERENCE_BACKEND", "reftable")
	if _, ok := readGitHead(t.TempDir()); ok {
		t.Fatal("reader ignored GIT_REFERENCE_BACKEND")
	}
}

func TestConfigNeedsGit(t *testing.T) {
	for cfg, want := range map[string]bool{
		"[core]\n\trepositoryformatversion = 0\n\tbare = false\n":    false,
		"[branch \"worktree-x\"]\n\tmerge = refs/heads/worktree-x\n": false,
		"[remote \"vscode-extensions\"]\n\turl = x/include\n":        false,
		"[core]\n\tworktree = ../w\n":                                true,
		"[core] worktree = ../w\n":                                   true,
		"[extensions]\n\tworktreeConfig = true\n":                    true,
		"[includeIf \"gitdir:x\"]\n\tpath = y\n":                     true,
		"[include]\n\tpath = y\n":                                    true,
		"[core]\n\trepositoryformatversion = 1\n":                    true,
		"[core]\n\trepositoryFormatVersion=0\n":                      false,
	} {
		if got := configNeedsGit([]byte(cfg)); got != want {
			t.Errorf("configNeedsGit(%q) = %v, want %v", cfg, got, want)
		}
	}
}
