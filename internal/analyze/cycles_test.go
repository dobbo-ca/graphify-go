package analyze

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// importCyclesOracle is the pre-SCC ImportCycles, kept verbatim as the
// reference the pruned search must match cycle for cycle.
func importCyclesOracle(g *model.Graph, maxLen, topN int) []Cycle {
	adj := map[string][]string{}
	for _, e := range g.Edges() {
		if e.Relation != "imports_from" || e.TypeOnly {
			continue
		}
		uf, vf := g.Nodes[e.Source].SourceFile, g.Nodes[e.Target].SourceFile
		if uf == "" || vf == "" || uf == vf {
			continue
		}
		adj[uf] = append(adj[uf], vf)
	}
	seen := map[string]bool{}
	var cycles []Cycle
	starts := make([]string, 0, len(adj))
	for s := range adj {
		starts = append(starts, s)
	}
	sort.Strings(starts)

	var dfs func(start, cur string, path []string, visited map[string]bool)
	dfs = func(start, cur string, path []string, visited map[string]bool) {
		if len(path) > maxLen || len(cycles) >= topN*10 {
			return
		}
		nbrs := append([]string(nil), adj[cur]...)
		sort.Strings(nbrs)
		for _, nb := range nbrs {
			if nb == start && len(path) >= 2 {
				if key := rotateKey(path); !seen[key] {
					seen[key] = true
					cycles = append(cycles, Cycle{Files: append([]string(nil), path...)})
				}
				continue
			}
			if nb > start || visited[nb] {
				continue
			}
			visited[nb] = true
			dfs(start, nb, append(path, nb), visited)
			visited[nb] = false
		}
	}
	for _, s := range starts {
		dfs(s, s, []string{s}, map[string]bool{s: true})
	}
	sort.SliceStable(cycles, func(i, j int) bool { return len(cycles[i].Files) < len(cycles[j].Files) })
	if len(cycles) > topN {
		cycles = cycles[:topN]
	}
	return cycles
}

func rotateKey(path []string) string {
	min, idx := path[0], 0
	for i, p := range path {
		if p < min {
			min, idx = p, i
		}
	}
	rot := append(append([]string(nil), path[idx:]...), path[:idx]...)
	return strings.Join(rot, "\x00")
}

// importGraph builds one node per file and an imports_from edge per "a>b" pair.
func importGraph(edges ...string) *model.Graph {
	g := model.New()
	for _, e := range edges {
		a, b, _ := strings.Cut(e, ">")
		g.AddNode(model.Node{ID: a, SourceFile: a})
		g.AddNode(model.Node{ID: b, SourceFile: b})
		g.AddEdge(model.Edge{Source: a, Target: b, Relation: "imports_from"})
	}
	return g
}

func cycleStrings(cs []Cycle) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, strings.Join(c.Files, ","))
	}
	return out
}

// Pins what the report has always shown on small graphs.
func TestImportCyclesPinned(t *testing.T) {
	for _, tc := range []struct {
		name         string
		edges        []string
		maxLen, topN int
		want         []string
	}{
		{"self loop is not a cycle", []string{"a>a", "a>b"}, 5, 20, []string{}},
		{"isolated and acyclic nodes", []string{"a>b", "b>c", "x>y"}, 5, 20, []string{}},
		// A cycle starts at its largest file and walks the import direction.
		{"triangle", []string{"a>b", "b>c", "c>a"}, 5, 20, []string{"c,a,b"}},
		{"disconnected components in start order", []string{"y>x", "x>y", "b>a", "a>b"}, 5, 20, []string{"b,a", "y,x"}},
		// Equal lengths keep discovery order: start ascending, then neighbour ascending.
		{"ties", []string{"a>b", "b>a", "a>c", "c>a", "b>c", "c>b"}, 5, 20,
			[]string{"b,a", "c,a", "c,b", "c,a,b", "c,b,a"}},
		{"shortest first", []string{"a>b", "b>c", "c>a", "y>z", "z>y"}, 5, 20, []string{"z,y", "c,a,b"}},
		{"length bound", []string{"a>b", "b>c", "c>d", "d>a"}, 3, 20, []string{}},
		{"length bound inclusive", []string{"a>b", "b>c", "c>d", "d>a"}, 4, 20, []string{"d,a,b,c"}},
		{"topN cap", []string{"a>b", "b>a", "a>c", "c>a", "b>c", "c>b"}, 5, 2, []string{"b,a", "c,a"}},
	} {
		g := importGraph(tc.edges...)
		got := cycleStrings(ImportCycles(g, tc.maxLen, tc.topN))
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
		if old := cycleStrings(importCyclesOracle(g, tc.maxLen, tc.topN)); !reflect.DeepEqual(old, tc.want) {
			t.Errorf("%s: oracle got %v, want %v", tc.name, old, tc.want)
		}
	}
}

// Random import graphs, sparse to complete, including several nodes per file,
// duplicate edges, non-import and type-only edges, and caps small enough to
// trip the topN*10 early exit.
func TestImportCyclesMatchesOracle(t *testing.T) {
	nonEmpty, capped := 0, 0
	for trial := 0; trial < 4000; trial++ {
		r := rand.New(rand.NewSource(int64(trial)))
		n := 2 + r.Intn(14)
		g := model.New()
		for i := 0; i < n; i++ {
			g.AddNode(model.Node{ID: fmt.Sprint("n", i), SourceFile: fmt.Sprintf("f%d.go", r.Intn(n))})
		}
		// Density cycles from sparse up to several edges per ordered pair.
		for i, m := 0, r.Intn(n*(1+trial%12)); i < m; i++ {
			g.AddEdge(model.Edge{Source: fmt.Sprint("n", r.Intn(n)), Target: fmt.Sprint("n", r.Intn(n)),
				Relation: []string{"imports_from", "imports_from", "calls"}[r.Intn(3)], TypeOnly: r.Intn(8) == 0})
		}
		maxLen, topN := 1+r.Intn(6), 1+r.Intn(4)
		want, got := importCyclesOracle(g, maxLen, topN), ImportCycles(g, maxLen, topN)
		if len(want) > 0 {
			nonEmpty++
		}
		if len(want) == topN {
			capped++
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("trial %d maxLen=%d topN=%d:\nwant %v\ngot  %v", trial, maxLen, topN, want, got)
		}
	}
	if nonEmpty < 1000 || capped < 500 {
		t.Errorf("weak coverage: nonEmpty=%d capped=%d", nonEmpty, capped)
	}
}

// A dense acyclic import graph has width^maxLen paths and no cycle; the search
// must not walk them.
func TestImportCyclesDenseAcyclicIsFast(t *testing.T) {
	const layers, width = 7, 40
	g := model.New()
	id := func(l, i int) string { return fmt.Sprintf("l%d/f%02d.go", l, i) }
	for l := 0; l < layers; l++ {
		for i := 0; i < width; i++ {
			g.AddNode(model.Node{ID: id(l, i), SourceFile: id(l, i)})
		}
	}
	for l := 1; l < layers; l++ {
		for i := 0; i < width; i++ {
			for j := 0; j < width; j++ {
				g.AddEdge(model.Edge{Source: id(l, i), Target: id(l-1, j), Relation: "imports_from"})
			}
		}
	}
	done := make(chan []Cycle, 1)
	go func() { done <- ImportCycles(g, 5, 20) }()
	select {
	case cycles := <-done:
		if len(cycles) != 0 {
			t.Errorf("expected no cycles, got %v", cycles)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ImportCycles still running after 10s on a dense acyclic graph")
	}
}
