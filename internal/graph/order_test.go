package graph

import (
	"math/rand"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// edgeOrder must reproduce sort.SliceStable on (source, target, relation), the
// order Build used before, including the relative order of equal keys.
func TestEdgeOrderMatchesStableSort(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, n := range []int{0, 1, 2, 3, 50, 1000, 200000} {
		// Few distinct keys, so most edges tie on one, two or all three fields.
		k := 1 + n/200
		edges := make([]model.Edge, n)
		for i := range edges {
			edges[i] = model.Edge{
				Source:         "s" + strconv.Itoa(rng.Intn(k)),
				Target:         "t" + strconv.Itoa(rng.Intn(3)),
				Relation:       model.Rel([]string{"calls", "imports", "contains"}[rng.Intn(3)]),
				SourceLocation: model.LocOf(strconv.Itoa(i)),
			}
		}
		want := append([]model.Edge(nil), edges...)
		sort.SliceStable(want, func(i, j int) bool {
			a, b := want[i], want[j]
			if a.Source != b.Source {
				return a.Source < b.Source
			}
			if a.Target != b.Target {
				return a.Target < b.Target
			}
			return a.Relation.String() < b.Relation.String()
		})
		ptrs := make([]*model.Edge, n)
		for i := range edges {
			ptrs[i] = &edges[i]
		}
		order := edgeOrder(ptrs)
		if len(order) != n {
			t.Fatalf("n=%d: got %d indexes", n, len(order))
		}
		for pos, i := range order {
			if edges[i] != want[pos] {
				t.Fatalf("n=%d: position %d is edge %s, stable sort has %s", n, pos, edges[i].SourceLocation, want[pos].SourceLocation)
			}
		}
	}
}

// BuildParts must leave the graph Build leaves for the joined extraction,
// wherever the parts are cut, and must not write to them.
func TestBuildPartsMatchesBuild(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for round := 0; round < 100; round++ {
		var ext model.Extraction
		for i, n := 0, 1+rng.Intn(30); i < n; i++ {
			// Few ids, some spelled two ways: repeated nodes, edges that tie
			// on all three keys, endpoints found by their normalized form.
			id := []string{"n", "N.", "m"}[rng.Intn(3)] + strconv.Itoa(rng.Intn(4))
			ext.Nodes = append(ext.Nodes, model.Node{ID: id, Label: strconv.Itoa(i), SourceFile: []string{"", "a.go", "b.py"}[rng.Intn(3)]})
		}
		for i, n := 0, rng.Intn(80); i < n; i++ {
			ext.Edges = append(ext.Edges, model.Edge{
				Source:     []string{"n", "N.", "m", "x"}[rng.Intn(4)] + strconv.Itoa(rng.Intn(4)),
				Target:     []string{"n", "n.", "m"}[rng.Intn(3)] + strconv.Itoa(rng.Intn(4)),
				Relation:   model.Rel([]string{"calls", "imports", "contains"}[rng.Intn(3)]),
				Confidence: model.Conf([]string{"INFERRED", "EXTRACTED"}[rng.Intn(2)]),
				Weight:     float64(i),
			})
		}
		var nodes [][]model.Node
		var edges [][]model.Edge
		for lo := 0; lo < len(ext.Nodes); {
			hi := lo + rng.Intn(len(ext.Nodes)-lo+1)
			nodes, lo = append(nodes, ext.Nodes[lo:hi]), hi
		}
		for lo := 0; lo < len(ext.Edges); {
			hi := lo + rng.Intn(len(ext.Edges)-lo+1)
			edges, lo = append(edges, ext.Edges[lo:hi]), hi
		}
		before := append([]model.Edge(nil), ext.Edges...)
		want, got := Build(model.Extraction{Nodes: ext.Nodes, Edges: before}), BuildParts(nodes, edges)
		if !slices.Equal(ext.Edges, before) {
			t.Fatalf("round %d: BuildParts wrote to its parts", round)
		}
		if !slices.Equal(got.NodeIDs(), want.NodeIDs()) {
			t.Fatalf("round %d: nodes %v, want %v", round, got.NodeIDs(), want.NodeIDs())
		}
		for _, id := range want.NodeIDs() {
			if !reflect.DeepEqual(got.Nodes[id], want.Nodes[id]) {
				t.Fatalf("round %d: node %s = %+v, want %+v", round, id, *got.Nodes[id], *want.Nodes[id])
			}
			if !slices.Equal(got.NeighborIndexes(id), want.NeighborIndexes(id)) {
				t.Fatalf("round %d: neighbours of %s differ", round, id)
			}
		}
		if got.NumEdges() != want.NumEdges() {
			t.Fatalf("round %d: %d edges, want %d", round, got.NumEdges(), want.NumEdges())
		}
		for i, e := range want.Edges() {
			if *got.Edges()[i] != *e {
				t.Fatalf("round %d: edge %d = %+v, want %+v", round, i, *got.Edges()[i], *e)
			}
		}
	}
}
