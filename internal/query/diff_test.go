package query

import (
	"reflect"
	"testing"
)

// oldDiffJSON: util.go defines add(); math.go defines compute() which calls add().
const oldDiffJSON = `{"directed":true,"multigraph":false,"graph":{},
"nodes":[
 {"id":"add","label":"add()","file_type":"code","source_file":"util.go","source_location":"L1"},
 {"id":"compute","label":"compute()","file_type":"code","source_file":"math.go","source_location":"L1"}
],
"links":[
 {"source":"compute","target":"add","relation":"calls","confidence":"INFERRED"}
],
"hyperedges":[]}`

// newDiffJSON: add() removed, sub() added; compute() now calls sub() instead.
const newDiffJSON = `{"directed":true,"multigraph":false,"graph":{},
"nodes":[
 {"id":"sub","label":"sub()","file_type":"code","source_file":"util.go","source_location":"L5"},
 {"id":"compute","label":"compute()","file_type":"code","source_file":"math.go","source_location":"L1"}
],
"links":[
 {"source":"compute","target":"sub","relation":"calls","confidence":"INFERRED"}
],
"hyperedges":[]}`

func TestDiff(t *testing.T) {
	oldG := loadJSON(t, oldDiffJSON)
	newG := loadJSON(t, newDiffJSON)
	res := Diff(oldG, newG)

	if got := res.NewNodes; !reflect.DeepEqual(got, []DiffNode{{ID: "sub", Label: "sub()"}}) {
		t.Errorf("new nodes: got %+v, want [sub]", got)
	}
	if got := res.RemovedNodes; !reflect.DeepEqual(got, []DiffNode{{ID: "add", Label: "add()"}}) {
		t.Errorf("removed nodes: got %+v, want [add]", got)
	}
	if got := res.NewEdges; !reflect.DeepEqual(got, []DiffEdge{{Source: "compute", Target: "sub", Relation: "calls"}}) {
		t.Errorf("new edges: got %+v, want [compute->sub]", got)
	}
	if got := res.RemovedEdges; !reflect.DeepEqual(got, []DiffEdge{{Source: "compute", Target: "add", Relation: "calls"}}) {
		t.Errorf("removed edges: got %+v, want [compute->add]", got)
	}
	if want := "1 new node, 1 new edge, 1 node removed, 1 edge removed"; res.Summary != want {
		t.Errorf("summary: got %q, want %q", res.Summary, want)
	}
}

func TestDiffNoChanges(t *testing.T) {
	g1 := loadJSON(t, oldDiffJSON)
	g2 := loadJSON(t, oldDiffJSON)
	res := Diff(g1, g2)
	if res.Summary != "no changes" {
		t.Errorf("summary: got %q, want %q", res.Summary, "no changes")
	}
	if len(res.NewNodes)+len(res.RemovedNodes)+len(res.NewEdges)+len(res.RemovedEdges) != 0 {
		t.Errorf("expected empty delta, got %+v", res)
	}
}

// TestDiffEdgeDirectionAware: the graph is directed, so reversing an edge's
// endpoints between snapshots registers as one removed (x->y) and one added (y->x).
func TestDiffEdgeDirectionAware(t *testing.T) {
	a := `{"directed":true,"nodes":[
 {"id":"x","label":"x()","source_file":"a.go","source_location":"L1"},
 {"id":"y","label":"y()","source_file":"b.go","source_location":"L1"}
],"links":[{"source":"x","target":"y","relation":"calls","confidence":"INFERRED"}]}`
	b := `{"directed":true,"nodes":[
 {"id":"x","label":"x()","source_file":"a.go","source_location":"L1"},
 {"id":"y","label":"y()","source_file":"b.go","source_location":"L1"}
],"links":[{"source":"y","target":"x","relation":"calls","confidence":"INFERRED"}]}`
	res := Diff(loadJSON(t, a), loadJSON(t, b))
	if got := res.NewEdges; !reflect.DeepEqual(got, []DiffEdge{{Source: "y", Target: "x", Relation: "calls"}}) {
		t.Errorf("new edges: got %+v, want [y->x]", got)
	}
	if got := res.RemovedEdges; !reflect.DeepEqual(got, []DiffEdge{{Source: "x", Target: "y", Relation: "calls"}}) {
		t.Errorf("removed edges: got %+v, want [x->y]", got)
	}
	if want := "1 new edge, 1 edge removed"; res.Summary != want {
		t.Errorf("summary: got %q, want %q", res.Summary, want)
	}
}

// TestDiffMutualRecursionEdgeRemoved: when a node pair has edges in both
// directions (A->B and B->A) and one is removed, the removal must be reported.
// A direction-insensitive key would collapse both edges and hide the removal.
func TestDiffMutualRecursionEdgeRemoved(t *testing.T) {
	a := `{"directed":true,"nodes":[
 {"id":"A","label":"A()","source_file":"a.go","source_location":"L1"},
 {"id":"B","label":"B()","source_file":"b.go","source_location":"L1"}
],"links":[
 {"source":"A","target":"B","relation":"calls","confidence":"INFERRED"},
 {"source":"B","target":"A","relation":"calls","confidence":"INFERRED"}
]}`
	b := `{"directed":true,"nodes":[
 {"id":"A","label":"A()","source_file":"a.go","source_location":"L1"},
 {"id":"B","label":"B()","source_file":"b.go","source_location":"L1"}
],"links":[
 {"source":"A","target":"B","relation":"calls","confidence":"INFERRED"}
]}`
	res := Diff(loadJSON(t, a), loadJSON(t, b))
	if len(res.NewEdges) != 0 {
		t.Errorf("new edges: got %+v, want none", res.NewEdges)
	}
	if got := res.RemovedEdges; !reflect.DeepEqual(got, []DiffEdge{{Source: "B", Target: "A", Relation: "calls"}}) {
		t.Errorf("removed edges: got %+v, want [B->A]", got)
	}
	if want := "1 edge removed"; res.Summary != want {
		t.Errorf("summary: got %q, want %q", res.Summary, want)
	}
}
