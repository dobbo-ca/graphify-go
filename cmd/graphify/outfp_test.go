package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dobbo-ca/graphify-go/internal/detect"
	"github.com/dobbo-ca/graphify-go/internal/ts"
)

func fpRoot(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	t.Setenv("GRAPHIFY_MTIME_GRANULARITY_MS", "0")
	root := t.TempDir()
	gitRun(t, root, "init", "-q")
	writeTree(t, root, map[string]string{
		"a.go": "package p\n\nfunc A() { B() }\n",
		"b.go": "package p\n\nfunc B() {}\n",
	})
	gitRun(t, root, "add", ".")
	gitRun(t, root, "-c", "commit.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "x")
	return root
}

func outMtime(t *testing.T, root, name string) time.Time {
	t.Helper()
	fi, err := os.Stat(filepath.Join(root, "graphify-out", name))
	if err != nil {
		t.Fatal(err)
	}
	return fi.ModTime()
}

func TestNoChangeUpdateSkipsWrites(t *testing.T) {
	root := fpRoot(t)
	_, _, g0, r0 := runOut(t, root, func(a []string) error { return cmdBuild(a) })
	gt, rt := outMtime(t, root, "graph.json"), outMtime(t, root, "GRAPH_REPORT.md")
	out1, _, g1, r1 := runOut(t, root, cmdUpdate)
	if g1 != g0 || r1 != r0 || !outMtime(t, root, "graph.json").Equal(gt) || !outMtime(t, root, "GRAPH_REPORT.md").Equal(rt) {
		t.Fatal("no-change update rewrote the outputs")
	}
	// A rewrite on the full path prints the same line.
	f := filepath.Join(root, "graphify-out", "GRAPH_REPORT.md")
	if err := os.WriteFile(f, []byte(r0+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out2, _, g2, r2 := runOut(t, root, cmdUpdate)
	if outMtime(t, root, "graph.json").Equal(gt) {
		t.Fatal("edited report did not take the full path")
	}
	if out2 != out1 || g2 != g0 || r2 != r0 {
		t.Errorf("full path differs:\n%s%s", out1, out2)
	}
}

func TestChangedInputsTakeFullPath(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, root string)
		args   []string
	}{
		{"edit", func(t *testing.T, root string) {
			writeTree(t, root, map[string]string{"b.go": "package p\n\nfunc B() { A() }\n\nfunc C() {}\n"})
		}, nil},
		{"add", func(t *testing.T, root string) {
			writeTree(t, root, map[string]string{"c.go": "package p\n\nfunc C() { A() }\n"})
		}, nil},
		{"remove", func(t *testing.T, root string) {
			os.Remove(filepath.Join(root, "b.go"))
		}, nil},
		{"unclassified", func(t *testing.T, root string) {
			writeTree(t, root, map[string]string{"x.zzz": "?"})
		}, nil},
		{"commit", func(t *testing.T, root string) {
			gitRun(t, root, "-c", "commit.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "y")
		}, nil},
		{"removed graph", func(t *testing.T, root string) {
			os.Remove(filepath.Join(root, "graphify-out", "graph.json"))
		}, nil},
		{"edited graph", func(t *testing.T, root string) {
			f := filepath.Join(root, "graphify-out", "graph.json")
			b, _ := os.ReadFile(f)
			os.WriteFile(f, append(b, '\n'), 0o644)
		}, nil},
		{"no-cluster", func(t *testing.T, root string) {}, []string{"--no-cluster"}},
		{"no-manifests", func(t *testing.T, root string) {}, []string{"--no-manifests"}},
		{"edited manifest", func(t *testing.T, root string) {
			writeTree(t, root, map[string]string{"go.mod": "module example.com/p\n\ngo 1.21\n"})
		}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := fpRoot(t)
			if err := cmdBuild([]string{root}); err != nil {
				t.Fatal(err)
			}
			old := outMtime(t, root, "GRAPH_REPORT.md")
			tc.mutate(t, root)
			_, _, g, r := runOut(t, root, cmdUpdate, tc.args...)
			if outMtime(t, root, "GRAPH_REPORT.md").Equal(old) {
				t.Fatal("update took the early path")
			}
			_, _, cg, cr := runOut(t, root, func(a []string) error { return cmdBuild(a) }, tc.args...)
			if g != cg || r != cr {
				t.Error("update differs from a cold build")
			}
		})
	}
}

func TestOutFPChangesWithBinary(t *testing.T) {
	root := fpRoot(t)
	old := binID
	defer func() { binID = old }()
	a := outFP(root, detect.WalkReport{}, "c", "")
	binID = "other"
	if outFP(root, detect.WalkReport{}, "c", "") == a {
		t.Fatal("fingerprint ignores binary identity")
	}
}

func TestSigOfSeesReplacedFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "g")
	os.WriteFile(p, []byte("a"), 0o644)
	s := sigOf(p)
	os.WriteFile(p, []byte("bb"), 0o644)
	if sigOf(p) == s || sigOf(p+"x") != "" {
		t.Fatal("sig did not change")
	}
}

func TestOverBudgetUpdateRepeatsWarning(t *testing.T) {
	defer func(b uint32) { ts.Budget = b }(ts.Budget)
	ts.Budget = 65536
	root := fpRoot(t)
	writeTree(t, root, map[string]string{"s.jl": stallJulia})
	runOut(t, root, cmdBuild)
	for i := 0; i < 2; i++ {
		if _, stderr, _, _ := runOut(t, root, cmdUpdate); !strings.Contains(stderr, "parser made no progress") {
			t.Fatalf("update %d: stderr = %q, want the warning", i, stderr)
		}
	}
}
