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

// TestDiffEdgeDirectionInsensitive: the same edge with endpoints swapped between
// snapshots must not register as removed+added — the canonical key sorts them.
func TestDiffEdgeDirectionInsensitive(t *testing.T) {
	a := `{"directed":true,"nodes":[
 {"id":"x","label":"x()","source_file":"a.go","source_location":"L1"},
 {"id":"y","label":"y()","source_file":"b.go","source_location":"L1"}
],"links":[{"source":"x","target":"y","relation":"calls","confidence":"INFERRED"}]}`
	b := `{"directed":true,"nodes":[
 {"id":"x","label":"x()","source_file":"a.go","source_location":"L1"},
 {"id":"y","label":"y()","source_file":"b.go","source_location":"L1"}
],"links":[{"source":"y","target":"x","relation":"calls","confidence":"INFERRED"}]}`
	res := Diff(loadJSON(t, a), loadJSON(t, b))
	if len(res.NewEdges) != 0 || len(res.RemovedEdges) != 0 {
		t.Errorf("swapped endpoints should be unchanged, got new=%+v removed=%+v", res.NewEdges, res.RemovedEdges)
	}
	if res.Summary != "no changes" {
		t.Errorf("summary: got %q, want %q", res.Summary, "no changes")
	}
}
