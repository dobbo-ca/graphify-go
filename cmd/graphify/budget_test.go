package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/ts"
)

// stallJulia makes the julia parser retry one byte without end.
const stallJulia = "@testset \"t0\" begin\n    @testset \"t1\" begin\n        l2"

const (
	budgetWarning = "  warning: skipped s.jl (parser made no progress, GRAPHIFY_PARSE_BUDGET=65536; 0 turns the limit off)\n"
	budgetLine    = "- 1 file(s) skipped: parser made no progress (GRAPHIFY_PARSE_BUDGET)\n"
)

// runOut runs fn and returns stdout, stderr and the two output files.
func runOut(t *testing.T, root string, fn func([]string) error, args ...string) (stdout, stderr, graph, report string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	stdout = captureStdout(t, func() {
		if err := fn(append([]string{root}, args...)); err != nil {
			t.Errorf("run: %v", err)
		}
	})
	os.Stderr = saved
	w.Close()
	b, _ := io.ReadAll(r)
	g, err := os.ReadFile(filepath.Join(root, "graphify-out", "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(filepath.Join(root, "graphify-out", "GRAPH_REPORT.md"))
	if err != nil {
		t.Fatal(err)
	}
	return stdout, string(b), string(g), string(md)
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParseBudgetEndToEnd(t *testing.T) {
	defer func(b uint32) { ts.Budget = b }(ts.Budget)
	ts.Budget = 65536
	root := t.TempDir()
	writeTree(t, root, map[string]string{"s.jl": stallJulia, "a.go": "package p\n\nfunc A() { B() }\n\nfunc B() {}\n"})

	_, stderr, graph, report := runOut(t, root, cmdBuild)
	if stderr != budgetWarning {
		t.Errorf("build: stderr = %q, want %q", stderr, budgetWarning)
	}
	if !strings.Contains(report, "communities\n- ") || !strings.Contains(report, budgetLine) {
		t.Errorf("build: report lacks %q:\n%s", budgetLine, report)
	}
	for _, want := range []string{`"label": "A()"`, `"label": "B()"`, `"label": "s.jl"`} {
		if !strings.Contains(graph, want) {
			t.Errorf("build: graph.json lacks %s", want)
		}
	}

	stdout, stderr, graph2, report2 := runOut(t, root, cmdUpdate)
	if !strings.Contains(stdout, "(0 reparsed, 2 reused") || stderr != budgetWarning {
		t.Errorf("update: stdout = %q, stderr = %q, want all reused and the warning", stdout, stderr)
	}
	if graph2 != graph || report2 != report {
		t.Error("update changed the outputs of an unchanged tree")
	}

	// Another limit is another cache.
	ts.Budget = 70000
	stdout, stderr, graph2, _ = runOut(t, root, cmdUpdate)
	if !strings.Contains(stdout, "(2 reparsed, 0 reused") || !strings.Contains(stderr, "GRAPHIFY_PARSE_BUDGET=70000") || !strings.Contains(stderr, "rebuilding it once") {
		t.Errorf("changed budget: stdout = %q, stderr = %q, want a full reparse with a note", stdout, stderr)
	}
	if graph2 != graph {
		t.Error("changed budget: graph.json differs")
	}
	ts.Budget = 65536
	runOut(t, root, cmdUpdate)

	// A fixed file is parsed and no longer reported.
	writeTree(t, root, map[string]string{"s.jl": "function f()\nend\n"})
	stdout, stderr, graph2, report2 = runOut(t, root, cmdUpdate)
	if !strings.Contains(stdout, "(1 reparsed, 1 reused") || stderr != "" {
		t.Errorf("fixed file: stdout = %q, stderr = %q, want one reparse and no warning", stdout, stderr)
	}
	if strings.Contains(report2, "skipped") || !strings.Contains(graph2, `"label": "f()"`) {
		t.Errorf("fixed file: report still has the line or the graph lacks f():\n%s", report2)
	}

	// A good file that turns into a stopped one loses nodes without --force.
	writeTree(t, root, map[string]string{"s.jl": stallJulia})
	stdout, stderr, graph2, report2 = runOut(t, root, cmdUpdate)
	if !strings.Contains(stdout, "(1 reparsed, 1 reused") || stderr != budgetWarning {
		t.Errorf("file turned bad: stdout = %q, stderr = %q", stdout, stderr)
	}
	if graph2 != graph || report2 != report {
		t.Error("file turned bad: outputs differ from the first build")
	}
}

func TestParseBudgetSameAtAnyGOMAXPROCS(t *testing.T) {
	defer func(b uint32) { ts.Budget = b }(ts.Budget)
	ts.Budget = 65536
	var want [2]string
	for _, procs := range []int{1, 8} {
		defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(procs))
		root := t.TempDir()
		files := map[string]string{"s.jl": stallJulia, "t.jl": stallJulia + "\n", "a.go": "package p\n\nfunc A() { B() }\n", "b.go": "package p\n\nfunc B() {}\n"}
		writeTree(t, root, files)
		_, stderr, graph, report := runOut(t, root, cmdBuild)
		if strings.Count(stderr, "warning: skipped") != 2 || strings.Index(stderr, "s.jl") > strings.Index(stderr, "t.jl") {
			t.Errorf("GOMAXPROCS %d: stderr = %q, want s.jl then t.jl", procs, stderr)
		}
		got := [2]string{strings.ReplaceAll(graph, root, ""), strings.ReplaceAll(report, root, "")}
		if want[0] == "" {
			want = got
		}
		if got != want {
			t.Errorf("GOMAXPROCS %d: outputs differ from GOMAXPROCS 1", procs)
		}
	}
}
