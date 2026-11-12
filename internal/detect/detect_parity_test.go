package detect

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func put(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func collect(t *testing.T, root string) []string {
	t.Helper()
	files, err := CollectFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	return files
}

func TestSymlinkContainment(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	put(t, root, "real.go", "package a\n")
	put(t, root, "sub/in.go", "package a\n")
	put(t, outside, "out.go", "package a\n")
	put(t, outside, "dir/x.go", "package a\n")
	links := map[string]string{
		"inlink.go":  filepath.Join(root, "sub", "in.go"),
		"outlink.go": filepath.Join(outside, "out.go"),
		"broken.go":  filepath.Join(base, "missing.go"),
		"dirlink":    filepath.Join(root, "sub"),
		"outdirlink": filepath.Join(outside, "dir"),
		"chain1.go":  filepath.Join(root, "chain2.go"),
		"..foo.go":   filepath.Join(root, "real.go"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Skip("symlinks unsupported:", err)
		}
	}
	os.Symlink(filepath.Join(outside, "out.go"), filepath.Join(root, "chain2.go"))
	got := collect(t, root)
	want := []string{"..foo.go", "inlink.go", "real.go", "sub/in.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNoiseDirs(t *testing.T) {
	cases := []struct {
		dir, evidence string // evidence "" means the name alone prunes
		pruned        bool
	}{
		{"venv", "", true}, {".venv", "", true}, {"__pycache__", "", true},
		{"lcov-report", "", true}, {"visual-tests", "", true}, {"visual-test", "", true},
		{"storybook-static", "", true}, {"dist-protected", "", true}, {".angular", "", true},
		{".parcel-cache", "", true}, {".serverless", "", true}, {".obsidian", "", true},
		{".smart-env", "", true}, {"my_venv", "", true},
		{"env", "pyvenv.cfg", true}, {".env", "bin/activate", true}, {"x_env", "lib/python3.12/a", true},
		{"env", "conda-meta/a", true},
		{"env", "", false}, {".env", "", false}, {"x_env", "", false},
		{"coverage", "lcov.info", true}, {"coverage", "lcov-report/a", true}, {"coverage", "", false},
		{"out", "production/a", true}, {"out", "_next/a", true}, {"out", "A.CLASS", true},
		{"out", "sub/a.class", true}, {"out", "sub/a.js.map", true}, {"out", "", false},
		{"out", "sub/Repo.java", false}, {".vscode", "", false},
		{"snapshots", "a.snap", true}, {"snapshots", "", false},
	}
	for _, c := range cases {
		root := t.TempDir()
		put(t, root, filepath.Join("p", c.dir, "keep.ts"), "x\n")
		if c.evidence != "" {
			put(t, root, filepath.Join("p", c.dir, c.evidence), "x\n")
		}
		got := slices.Contains(collect(t, root), filepath.Join("p", c.dir, "keep.ts"))
		if got == c.pruned {
			t.Errorf("%s evidence %q: pruned=%v want %v", c.dir, c.evidence, !got, c.pruned)
		}
	}
	root := t.TempDir()
	put(t, root, "__tests__/snapshots/keep.ts", "x\n")
	put(t, root, "adapter/out/Repo.java", "class A {}\n")
	got := collect(t, root)
	if slices.Contains(got, "__tests__/snapshots/keep.ts") || !slices.Contains(got, "adapter/out/Repo.java") {
		t.Fatalf("got %v", got)
	}
}

func TestRelativeRootSymlinkAndSnapshots(t *testing.T) {
	root := resolved(t)
	put(t, root, "b.go", "package a\n")
	put(t, root, "sub/d.go", "package a\n")
	if err := os.Symlink("b.go", filepath.Join(root, "a.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../b.go", filepath.Join(root, "sub", "c.go")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	got := collect(t, ".")
	if !slices.Equal(got, []string{"a.go", "b.go", "sub/c.go", "sub/d.go"}) {
		t.Errorf("relative root: %v", got)
	}

	tests := filepath.Join(resolved(t), "__tests__")
	put(t, tests, "c.go", "package a\n")
	put(t, tests, "snapshots/x.go", "package a\n")
	t.Chdir(tests)
	if got := collect(t, "."); !slices.Equal(got, []string{"c.go"}) {
		t.Errorf("snapshots under relative root: %v", got)
	}
}

func TestGraphifyOutBeatsNoiseGate(t *testing.T) {
	root := resolved(t)
	put(t, root, "control.go", "package a\n")
	put(t, root, "out/cache/x.go", "package a\n")
	t.Setenv("GRAPHIFY_OUT", "out")
	if got := collect(t, root); !slices.Equal(got, []string{filepath.Join(root, "control.go")}) && !slices.Equal(got, []string{"control.go"}) {
		t.Errorf("GRAPHIFY_OUT=out: %v", got)
	}
}

func TestBuildOutputProbeUsesDirOrder(t *testing.T) {
	// ReadDir(-1) on the dir itself must not sort; just check it still detects.
	root := resolved(t)
	put(t, root, "out/yy/X.class", "")
	put(t, root, "out/m.go", "package a\n")
	if !hasBuildOutputMarkers(filepath.Join(root, "out")) {
		t.Error("expected build output markers")
	}
}

func TestSymlinkedRootWithDotDot(t *testing.T) {
	base := resolved(t)
	put(t, base, "deep/inner/x", "")
	put(t, base, "deep/proj/a.go", "package a\n")
	if err := os.Symlink("a.go", filepath.Join(base, "deep/proj/l.go")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	os.Symlink(filepath.Join(base, "deep/inner"), filepath.Join(base, "lnk"))
	got := collect(t, filepath.Join(base, "lnk")+"/../proj")
	if want := []string{"a.go", "l.go"}; !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
