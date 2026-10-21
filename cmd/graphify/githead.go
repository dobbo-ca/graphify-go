package main

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitEnv holds the variables that move git's repo discovery or ref storage.
var gitEnv = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_CEILING_DIRECTORIES",
	"GIT_DISCOVERY_ACROSS_FILESYSTEM", "GIT_INDEX_FILE", "GIT_NAMESPACE",
	"GIT_OBJECT_DIRECTORY", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
	"GIT_REFERENCE_BACKEND", "GIT_TEST_ASSUME_DIFFERENT_OWNER",
}

// gitHead returns the current commit SHA of the repo at root, or "" if unavailable.
// It reads the repo files itself and runs git only when unsure.
func gitHead(root string) string {
	if sha, ok := readGitHead(root); ok {
		return sha
	}
	cmd := exec.Command("git", "-C", root, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// readGitHead resolves HEAD like git rev-parse. ok is false when git must
// decide: odd env, other ref storage, foreign owner, or a malformed file.
func readGitHead(root string) (sha string, ok bool) {
	for _, k := range gitEnv {
		if _, set := os.LookupEnv(k); set {
			return "", false
		}
	}
	// Without git, main stamps no commit.
	if _, err := exec.LookPath("git"); err != nil {
		return "", true
	}
	dir, err := physDir(root)
	if err != nil {
		return "", false
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return "", false
	}
	for {
		gitdir, found, ok := gitDirAt(dir)
		if !ok {
			return "", false
		}
		if found {
			return headOf(dir, gitdir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", true
		}
		pfi, err := os.Stat(parent)
		if err != nil || !sameDevice(fi, pfi) {
			// git will not cross filesystems by default.
			return "", false
		}
		dir, fi = parent, pfi
	}
}

// physDir resolves symlinks, since git discovers from the physical cwd.
func physDir(root string) (string, error) {
	p, err := filepath.EvalSymlinks(root)
	if err != nil || filepath.IsAbs(p) {
		return p, err
	}
	wd, err := os.Getwd()
	if err == nil {
		wd, err = filepath.EvalSymlinks(wd)
	}
	return filepath.Join(wd, p), err
}

// gitDirAt finds the git dir that dir is the top of, as git discovery does.
func gitDirAt(dir string) (gitdir string, found, ok bool) {
	dotgit := filepath.Join(dir, ".git")
	fi, err := os.Lstat(dotgit)
	switch {
	case err == nil && fi.IsDir():
		if !looksLikeGitDir(dotgit) {
			return "", false, false
		}
		return dotgit, true, true
	case err == nil && fi.Mode().IsRegular():
		b, err := os.ReadFile(dotgit)
		p, cut := strings.CutPrefix(strings.TrimRight(string(b), "\r\n"), "gitdir: ")
		if err != nil || !cut {
			return "", false, false
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		return p, true, true
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return "", false, false
	case err == nil:
		return "", false, false
	}
	// Bare repos and admin dirs: let git apply its config.
	if _, err := os.Lstat(filepath.Join(dir, "HEAD")); err == nil || !errors.Is(err, fs.ErrNotExist) {
		return "", false, false
	}
	return "", false, true
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func looksLikeGitDir(d string) bool {
	for _, n := range []string{"HEAD", "objects", "refs"} {
		if _, err := os.Stat(filepath.Join(d, n)); err != nil {
			return false
		}
	}
	return true
}

// headOf reads HEAD of gitdir, which top (the work tree or bare dir) holds.
func headOf(top, gitdir string) (string, bool) {
	if !ownedByMe(top) || !ownedByMe(gitdir) {
		return "", false
	}
	common := gitdir
	if b, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
		common = strings.TrimSpace(string(b))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitdir, common)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	// git rejects a gitdir without these.
	if !looksLikeGitDir(gitdir) && !(fileExists(filepath.Join(gitdir, "HEAD")) && looksLikeGitDir(common)) {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(common, "reftable")); err == nil {
		return "", false
	}
	cfg, err := os.ReadFile(filepath.Join(common, "config"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if configNeedsGit(cfg) {
		return "", false
	}
	return resolveRef("HEAD", gitdir, common, 0)
}

// configNeedsGit is true for config keys that change ref or repo layout.
// Branch names are not matched, so a branch called worktree-x is fine.
func configNeedsGit(cfg []byte) bool {
	for _, ln := range strings.Split(strings.ToLower(string(cfg)), "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "[") {
			name := strings.TrimLeft(ln, "[ \t")
			if i := strings.IndexAny(name, " \t].\""); i >= 0 {
				name = name[:i]
			}
			if name == "extensions" || name == "include" || name == "includeif" {
				return true
			}
			// A key may follow the header on the same line.
			_, ln, _ = strings.Cut(ln, "]")
			ln = strings.TrimSpace(ln)
		}
		key, val, _ := strings.Cut(ln, "=")
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "worktree", "refstorage":
			return true
		case "repositoryformatversion":
			if val != "0" {
				return true
			}
		}
	}
	return false
}

// badRef is true for names git's refname rules may reject.
func badRef(name string) bool {
	if strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") || strings.Contains(name, "@{") {
		return true
	}
	for _, c := range name {
		if c < ' ' || c == 0x7f || strings.ContainsRune(" ~^:?*[\\", c) {
			return true
		}
	}
	for _, p := range strings.Split(name, "/") {
		if p == "" || p[0] == '.' || strings.HasSuffix(p, ".lock") {
			return true
		}
	}
	return false
}

func isSha(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// resolveRef follows name to a SHA. An unborn ref gives "" and true.
func resolveRef(name, gitdir, common string, depth int) (string, bool) {
	if depth >= 5 || (name != "HEAD" && (!strings.HasPrefix(name, "refs/") || strings.Contains(name, "..") || badRef(name))) {
		return "", false
	}
	base := common
	if name == "HEAD" || strings.HasPrefix(name, "refs/worktree/") || strings.HasPrefix(name, "refs/bisect/") || strings.HasPrefix(name, "refs/rewritten/") {
		base = gitdir
	}
	path := filepath.Join(base, filepath.FromSlash(name))
	if fi, err := os.Lstat(path); err == nil && !fi.Mode().IsRegular() {
		// Legacy symlink refs: let git follow them.
		return "", false
	}
	b, err := os.ReadFile(path)
	if err == nil {
		s := strings.TrimSuffix(string(b), "\n")
		if isSha(s) {
			return s, true
		}
		if target, cut := strings.CutPrefix(s, "ref: "); cut {
			return resolveRef(target, gitdir, common, depth+1)
		}
		return "", false
	}
	if !errors.Is(err, fs.ErrNotExist) || name == "HEAD" {
		return "", false
	}
	packed, err := os.ReadFile(filepath.Join(common, "packed-refs"))
	if err != nil {
		return "", errors.Is(err, fs.ErrNotExist)
	}
	for _, line := range strings.Split(string(packed), "\n") {
		if line == "" || line[0] == '#' || line[0] == '^' {
			continue
		}
		sha, ref, _ := strings.Cut(line, " ")
		if strings.TrimSpace(ref) == name {
			return sha, isSha(sha)
		}
	}
	return "", true
}
