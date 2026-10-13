package extract

import (
	"slices"
	"sort"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// pyEdges resolves the sources and returns the edges of the given relations
// as sorted "source label -relation-> target label" lines. A file node shows
// as its path.
func pyEdges(srcs map[string]string, rels ...model.Relation) []string {
	ext := resolvePy(srcs)
	label := map[string]string{}
	for _, n := range ext.Nodes {
		label[n.ID] = n.Label
	}
	for f := range srcs {
		label[idutil.MakeID(f)] = f
	}
	var out []string
	for _, e := range ext.Edges {
		if slices.Contains(rels, e.Relation) {
			tgt := label[e.Target]
			if tgt == "" {
				tgt = e.Target
			}
			out = append(out, label[e.Source]+" -"+e.Relation.String()+"-> "+tgt)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// A def inside a function is a node of its own, held by the function around
// it, and a call is credited to the innermost def that holds it.
func TestPythonNestedFunctions(t *testing.T) {
	src := `def leaf(): pass
def other(): pass
def third(): pass

def outer():
    def inner(a=third()):
        def deep():
            leaf()
        other()
        deep()
    @deco(third())
    def wrapped():
        helper()
    def helper(): pass
    lam = lambda: other()
    class Local:
        third()
        def meth(self):
            leaf()
    if lam:
        def cond(): pass
    inner()

class K:
    def m(self):
        def in_m():
            leaf()
        in_m()

def unrelated():
    helper()
    inner()
`
	got := pyEdges(map[string]string{"a.py": src}, model.RelContains, model.RelCalls)
	want := []string{
		"K -contains-> K.m()",
		"K.m() -calls-> in_m()",
		"K.m() -contains-> in_m()",
		"a.py -contains-> K",
		"a.py -contains-> leaf()",
		"a.py -contains-> other()",
		"a.py -contains-> outer()",
		"a.py -contains-> third()",
		"a.py -contains-> unrelated()",
		"deep() -calls-> leaf()",
		"in_m() -calls-> leaf()",
		"inner() -calls-> deep()",
		"inner() -calls-> other()",
		"inner() -contains-> deep()",
		"meth() -calls-> leaf()",
		"outer() -calls-> inner()",
		// the lambda, the decorator and the class body run in outer
		"outer() -calls-> other()",
		"outer() -calls-> third()",
		"outer() -contains-> cond()",
		"outer() -contains-> helper()",
		"outer() -contains-> inner()",
		"outer() -contains-> meth()",
		"outer() -contains-> wrapped()",
		"wrapped() -calls-> helper()",
	}
	if !slices.Equal(got, want) {
		t.Errorf("edges:\n got %q\nwant %q", got, want)
	}
}
