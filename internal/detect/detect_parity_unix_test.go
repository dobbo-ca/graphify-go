//go:build !windows

package detect

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"
)

// within fails fast instead of hanging on a FIFO open.
func within(t *testing.T, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("collect hung, a non-regular file was opened")
	}
}

func TestNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	put(t, root, "ok.go", "package a\n")
	for _, n := range []string{"fifo.go", "fifo", "go.mod"} {
		if err := syscall.Mkfifo(filepath.Join(root, n), 0o644); err != nil {
			t.Skip(err)
		}
	}
	os.Symlink(filepath.Join(root, "fifo"), filepath.Join(root, "linkfifo"))
	os.Symlink(filepath.Join(root, "fifo.go"), filepath.Join(root, "linkfifo.go"))
	var files, mans []string
	within(t, func() {
		files, _ = CollectFiles(root)
		mans, _ = CollectManifests(root)
	})
	if len(files) != 1 || files[0] != "ok.go" {
		t.Fatalf("files %v", files)
	}
	if len(mans) != 0 {
		t.Fatalf("manifests %v", mans)
	}
}

func TestBuildOutputProbeAbortsOnStatError(t *testing.T) {
	root := resolved(t)
	put(t, root, "out/Z.class", "")
	if err := os.Symlink("a", filepath.Join(root, "out", "a")); err != nil {
		t.Fatal(err)
	}
	if hasBuildOutputMarkers(filepath.Join(root, "out")) {
		t.Error("ELOOP entry must abort the probe")
	}
}

func TestSymlinkedCwdParity(t *testing.T) {
	base := resolved(t)
	put(t, base, "repo/.gitignore", "*.py\n")
	mkGit(t, filepath.Join(base, "repo"))
	put(t, base, "repo/deep/sub/a.go", "package a\n")
	put(t, base, "repo/deep/sub/h.py", "x = 1\n")
	if err := os.Symlink("a.go", filepath.Join(base, "repo/deep/sub/l.go")); err != nil {
		t.Fatal(err)
	}
	put(t, base, "plain/x", "")
	if err := os.Symlink("../repo/deep/sub", filepath.Join(base, "plain/lnk")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(base, "plain/lnk"))
	if got := collect(t, ".."); !slices.Equal(got, []string{"sub/a.go", "sub/l.go"}) {
		t.Errorf("root .. from symlinked cwd: %v", got)
	}

	put(t, base, "t4/__tests__/c.go", "package a\n")
	put(t, base, "t4/__tests__/snapshots/x.go", "package a\n")
	if err := os.Symlink("__tests__", filepath.Join(base, "t4/t")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(base, "t4/t"))
	if got := collect(t, "."); !slices.Equal(got, []string{"c.go"}) {
		t.Errorf("snapshots via symlinked cwd: %v", got)
	}
}
