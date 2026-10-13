package extract

import (
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/ts"
)

// stallJulia makes the julia parser retry one byte without end.
const stallJulia = "@testset \"t0\" begin\n    @testset \"t1\" begin\n        l2"

func TestOverBudgetKeepsFileNode(t *testing.T) {
	r := FileFromBytes("a.jl", []byte(stallJulia))
	if !r.OverBudget || len(r.Nodes) != 1 || r.Nodes[0].SourceFile != "a.jl" || len(r.Edges) != 0 {
		t.Fatalf("OverBudget %v, nodes %+v, edges %d; want a stopped parse with the file node only", r.OverBudget, r.Nodes, len(r.Edges))
	}
	// The pool still gives working parsers.
	if r := FileFromBytes("a.go", []byte("package p\n\nfunc A() {}\n")); r.OverBudget || len(r.Nodes) != 2 {
		t.Errorf("after a stopped parse: OverBudget %v, %d nodes, want 2", r.OverBudget, len(r.Nodes))
	}
}

func TestBudgetSetting(t *testing.T) {
	defer func(b uint32) { ts.Budget = b }(ts.Budget)
	// The if with an init clause takes three steps at one byte.
	src := []byte("package p\n\nfunc A(m map[string]int) {\n\tif v, ok := m[\"a\"]; ok {\n\t\tB(v)\n\t}\n}\n\nfunc B(int) {}\n")
	ts.Budget = 2
	if r := FileFromBytes("a.go", src); !r.OverBudget || len(r.Nodes) != 1 {
		t.Errorf("budget 2: OverBudget %v, %d nodes, want a skip with 1", r.OverBudget, len(r.Nodes))
	}
	ts.Budget = 0
	if r := FileFromBytes("a.go", src); r.OverBudget || len(r.Nodes) != 3 {
		t.Errorf("budget 0: OverBudget %v, %d nodes, want 3", r.OverBudget, len(r.Nodes))
	}
}

func TestBudgetScalesWithSize(t *testing.T) {
	defer func(b uint32) { ts.Budget = b }(ts.Budget)
	ts.Budget = 65536
	// About 98,000 steps in a row at the last byte, and no endless retry.
	kt := []byte(strings.Repeat("<", 12000))
	if r := FileFromBytes("a.kt", kt); r.OverBudget {
		t.Error("12000 bytes of kotlin \"<\": stopped, want a parse")
	}
	ts.Budget = 16384
	if r := FileFromBytes("a.kt", kt); !r.OverBudget {
		t.Error("12000 bytes of kotlin \"<\" at a quarter of the limit: parsed, want a stop")
	}
	// A file of 64 KB or more gets the plain limit.
	ts.Budget = 65536
	big := strings.Repeat("x = 1\n", 1<<16) + stallJulia
	if r := FileFromBytes("a.jl", []byte(big)); !r.OverBudget || len(r.Nodes) != 1 {
		t.Errorf("stall after 384 KB: OverBudget %v, %d nodes, want a stopped parse", r.OverBudget, len(r.Nodes))
	}
}
