package detect

import (
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// walkTree must report what filepath.WalkDir reports, in the same order.
func TestWalkTreeMatchesWalkDir(t *testing.T) {
	root := t.TempDir()
	rng := rand.New(rand.NewSource(1))
	dirs := []string{root}
	for i := 0; i < 400; i++ {
		d := filepath.Join(dirs[rng.Intn(len(dirs))], fmt.Sprintf("d%d", rng.Intn(50)))
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, d)
	}
	for i := 0; i < 3000; i++ {
		// Names sort before, between and after the dN directories.
		name := fmt.Sprintf("%c%d.x", "Cdz"[rng.Intn(3)], i)
		if err := os.WriteFile(filepath.Join(dirs[rng.Intn(len(dirs))], name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)

	fn := func(rep *WalkReport, path string, d fs.DirEntry, err error) error {
		if err != nil {
			rep.WalkErrors = append(rep.WalkErrors, path)
			return nil
		}
		if d.IsDir() {
			if strings.HasSuffix(path, "7") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), "z") {
			rep.Skipped++
			if rep.SkippedExts == nil {
				rep.SkippedExts = map[string]int{}
			}
			rep.SkippedExts[d.Name()[:2]]++
			return nil
		}
		rep.Files = append(rep.Files, path)
		return nil
	}
	var want WalkReport
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		return fn(&want, path, d, err)
	}); err != nil {
		t.Fatal(err)
	}
	if len(want.Files) < 500 || want.Skipped == 0 || len(want.WalkErrors) != 1 {
		t.Fatalf("weak tree: %d files, %d skipped, errors %v", len(want.Files), want.Skipped, want.WalkErrors)
	}
	for i := 0; i < 20; i++ {
		if got := walkTree(root, fn); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: walkTree differs from WalkDir (%d files against %d, errors %v against %v)",
				i, len(got.Files), len(want.Files), got.WalkErrors, want.WalkErrors)
		}
	}
	missing := filepath.Join(root, "missing")
	var wantMissing WalkReport
	_ = filepath.WalkDir(missing, func(path string, d fs.DirEntry, err error) error {
		return fn(&wantMissing, path, d, err)
	})
	if got := walkTree(missing, fn); !reflect.DeepEqual(got, wantMissing) {
		t.Fatalf("missing root: %+v against %+v", got, wantMissing)
	}
}
