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
		g.AddEdge(model.Edge{Source: ids[p[0]], Target: ids[p[1]], Relation: "calls"})
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
