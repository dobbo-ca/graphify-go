package cluster

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

type louvainCase struct {
	name string
	g    *model.Graph
}

// build makes a graph of n nodes named by index, with the given index pairs as edges.
func build(n int, pairs ...[2]int) *model.Graph {
	g := model.New()
	for i := 0; i < n; i++ {
		g.AddNode(model.Node{ID: fmt.Sprintf("n%03d", i)})
	}
	ids := g.NodeIDs()
	for _, p := range pairs {
		g.AddEdge(model.Edge{Source: ids[p[0]], Target: ids[p[1]], Relation: model.RelCalls})
	}
	return g
}

func clique(from, to int) [][2]int {
	var out [][2]int
	for i := from; i < to; i++ {
		for j := i + 1; j < to; j++ {
			out = append(out, [2]int{i, j})
		}
	}
	return out
}

func louvainCases() []louvainCase {
	cases := []louvainCase{
		{"single", build(1)},
		{"isolated", build(5)},
		{"pair", build(2, [2]int{0, 1})},
		{"two_triangles", build(6, append(append(clique(0, 3), clique(3, 6)...), [2]int{0, 3})...)},
		// Every move ties: the lowest community index must win.
		{"tie_cycle4", build(4, [2]int{0, 1}, [2]int{1, 2}, [2]int{2, 3}, [2]int{3, 0})},
		{"tie_star", build(7, [2]int{0, 1}, [2]int{0, 2}, [2]int{0, 3}, [2]int{0, 4}, [2]int{0, 5}, [2]int{0, 6})},
		{"tie_path", build(9, [2]int{0, 1}, [2]int{1, 2}, [2]int{2, 3}, [2]int{3, 4}, [2]int{4, 5}, [2]int{5, 6}, [2]int{6, 7}, [2]int{7, 8})},
		{"self_loops", build(5, [2]int{0, 0}, [2]int{1, 1}, [2]int{4, 4}, [2]int{0, 1}, [2]int{1, 2}, [2]int{2, 0})},
		{"duplicate_edges", build(4, [2]int{0, 1}, [2]int{1, 0}, [2]int{0, 1}, [2]int{2, 3}, [2]int{3, 2}, [2]int{1, 2})},
		{"disconnected", build(14, append(append(clique(0, 4), clique(4, 8)...), [2]int{8, 9}, [2]int{9, 10}, [2]int{11, 12})...)},
		// A dense clique raises the total weight so the ring of small cliques
		// merges into one oversized community, which splitOversized breaks up.
		{"oversized", build(60, append(append(append(append(append(clique(0, 40), clique(40, 45)...), clique(45, 50)...), clique(50, 55)...), clique(55, 60)...),
			[2]int{40, 45}, [2]int{46, 50}, [2]int{51, 55}, [2]int{56, 41})...)},
	}
	for trial := 0; trial < 40; trial++ {
		r := rand.New(rand.NewSource(int64(trial)))
		n := 2 + r.Intn(250)
		blocks := 1 + r.Intn(8)
		var pairs [][2]int
		for i := r.Intn(n * (1 + r.Intn(6))); i > 0; i-- {
			a, b := r.Intn(n), r.Intn(n)
			if r.Intn(4) > 0 { // planted structure
				b = (a/blocks*blocks + r.Intn(blocks)) % n
			}
			pairs = append(pairs, [2]int{a, b})
		}
		if trial%2 == 1 { // heavy core, so sparse groups merge and get split again
			pairs = append(pairs, clique(0, n/5)...)
		}
		cases = append(cases, louvainCase{fmt.Sprintf("random_%02d", trial), build(n, pairs...)})
	}
	return cases
}

// formatGroups renders groups as node indices, keeping group and member order.
func formatGroups(groups [][]string) string {
	parts := make([]string, len(groups))
	for i, group := range groups {
		idx := make([]string, len(group))
		for j, id := range group {
			n, _ := strconv.Atoi(id[1:])
			idx[j] = strconv.Itoa(n)
		}
		parts[i] = strings.Join(idx, " ")
	}
	return strings.Join(parts, "|")
}

func clustered(g *model.Graph) [][]string {
	communities := Cluster(g)
	cids := make([]int, 0, len(communities))
	for cid := range communities {
		cids = append(cids, cid)
	}
	sort.Ints(cids)
	out := make([][]string, len(cids))
	for i, cid := range cids {
		out[i] = communities[cid]
	}
	return out
}

// The golden was recorded from gonum's community.Modularize before it was
// replaced. The raw order matters too: splitOversized feeds it back in.
func TestLouvainMatchesGonumGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/louvain_gonum.golden")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		f := strings.Split(line, "\t")
		want[f[0]] = f[1:]
	}
	cases := louvainCases()
	if len(want) != len(cases) {
		t.Fatalf("golden has %d cases, want %d", len(want), len(cases))
	}
	for _, c := range cases {
		for run := 0; run < 2; run++ { // same answer every run
			if got := formatGroups(louvain(c.g, c.g.NodeIDs())); got != want[c.name][0] {
				t.Errorf("%s: raw groups\n got %s\nwant %s", c.name, got, want[c.name][0])
			}
			if got := formatGroups(clustered(c.g)); got != want[c.name][1] {
				t.Errorf("%s: Cluster\n got %s\nwant %s", c.name, got, want[c.name][1])
			}
		}
	}
}

// louvain feeds modularize each node's neighbours in map order, so the result
// must not depend on that order, group and member order included.
func TestModularizeIgnoresNeighbourOrder(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, n := range []int{2, 9, 60, 400, 3000} {
		// Sparse random edges plus a few dense blocks, so ties and real moves both occur.
		set := map[[2]int]bool{}
		for i := 0; i < 3*n; i++ {
			a, b := rng.Intn(n), rng.Intn(n)
			if a != b {
				set[[2]int{min(a, b), max(a, b)}] = true
			}
		}
		for b := 0; b+6 <= n && b < 60; b += 12 {
			for _, p := range clique(b, b+6) {
				set[p] = true
			}
		}
		adj := make([][]int, n)
		for p := range set {
			adj[p[0]] = append(adj[p[0]], p[1])
			adj[p[1]] = append(adj[p[1]], p[0])
		}
		for i := range adj {
			sort.Ints(adj[i])
		}
		want := fmt.Sprint(modularize(adj))
		for run := 0; run < 5; run++ {
			shuffled := make([][]int, n)
			for i := range adj {
				shuffled[i] = append([]int(nil), adj[i]...)
				rng.Shuffle(len(shuffled[i]), func(a, b int) { shuffled[i][a], shuffled[i][b] = shuffled[i][b], shuffled[i][a] })
			}
			if got := fmt.Sprint(modularize(shuffled)); got != want {
				t.Fatalf("n=%d: result changed with neighbour order", n)
			}
		}
	}
}

// The skip in localMove must give what looking at every node every sweep gives.
func TestSkipMatchesLookingAtAll(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for round := 0; round < 400; round++ {
		// Blocks of uneven size with noise between them: near ties and long
		// tails of sweeps with a few moves each.
		n := 2 + rng.Intn(1500)
		block := 2 + rng.Intn(40)
		set := map[[2]int]bool{}
		for i := 0; i < n*(1+rng.Intn(6)); i++ {
			a := rng.Intn(n)
			b := rng.Intn(n)
			if rng.Intn(4) > 0 {
				b = a/block*block + rng.Intn(block)
			}
			if a != b && b < n {
				set[[2]int{min(a, b), max(a, b)}] = true
			}
		}
		adj := make([][]int, n)
		for p := range set {
			adj[p[0]] = append(adj[p[0]], p[1])
			adj[p[1]] = append(adj[p[1]], p[0])
		}
		lookAtAll = true
		want := fmt.Sprint(modularize(adj))
		lookAtAll = false
		if got := fmt.Sprint(modularize(adj)); got != want {
			t.Fatalf("round %d, n=%d: the skip changed the result", round, n)
		}
	}
}
