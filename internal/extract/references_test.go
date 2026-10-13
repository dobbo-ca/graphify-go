package extract

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// refResult is a hand-built file: its defs (a label ending in "()" is a
// function) and the names it references from the function f().
func refResult(file string, defs []string, refs ...TypeRef) Result {
	fid := idutil.MakeID(file)
	r := Result{File: file, Nodes: []model.Node{{ID: fid, Label: file, FileType: "code", SourceFile: file}}}
	for _, label := range append([]string{"f()"}, defs...) {
		name := label
		if n := len(label); n > 2 && label[n-2:] == "()" {
			name = label[:n-2]
		}
		id := idutil.MakeID(file, name)
		r.Nodes = append(r.Nodes, model.Node{ID: id, Label: label, FileType: "code", SourceFile: file})
		r.Defs = append(r.Defs, Def{ID: id, Name: name, File: file})
	}
	for _, t := range refs {
		t.FromID, t.File, t.Stub = idutil.MakeID(file, "f"), file, true
		if t.Relation == "" {
			t.Relation = "references"
		}
		r.TypeRefs = append(r.TypeRefs, t)
	}
	return r
}

func TestReferencesBindOrStub(t *testing.T) {
	results := []Result{
		refResult("a/x.py", []string{"Local", "Dup", "Early"},
			TypeRef{Name: "Local"},                      // same file
			TypeRef{Name: "Uniq"},                       // one definition in the corpus
			TypeRef{Name: "Dup"},                        // same file wins over b/y.py
			TypeRef{Name: "Gone"},                       // no definition: stub
			TypeRef{Name: "Gone"},                       // one stub per file and name
			TypeRef{Name: "helper"},                     // a function is not a type
			TypeRef{Name: "helper", Deco: true},         // but it is a decorator
			TypeRef{Name: "earlY"},                      // stub id folds onto Early
			TypeRef{Name: "f", Deco: true},              // self reference
			TypeRef{Name: "Base", Relation: "inherits"}, // unresolved base
		),
		refResult("b/y.py", []string{"Uniq", "Dup", "helper()"},
			TypeRef{Name: "Gone"},
			TypeRef{Name: "Amb"},
		),
		refResult("c/z.py", []string{"Amb"}),
		refResult("d/z.py", []string{"Amb"}),
	}
	ext := Resolve(results, []string{"a/x.py", "b/y.py", "c/z.py", "d/z.py"})

	label := map[string][2]string{}
	stubs := map[string]int{}
	for _, n := range ext.Nodes {
		label[n.ID] = [2]string{n.SourceFile, n.Label}
		if n.SourceFile == "" {
			if n.FileType != "code" {
				t.Errorf("stub %s has file type %q, want code", n.ID, n.FileType)
			}
			stubs[n.ID]++
		}
	}
	wantStubs := []string{
		idutil.MakeID("a/x.py", "Gone"), idutil.MakeID("a/x.py", "helper"), idutil.MakeID("a/x.py", "Base"),
		idutil.MakeID("b/y.py", "Gone"), idutil.MakeID("b/y.py", "Amb"),
	}
	for _, id := range wantStubs {
		if stubs[id] != 1 {
			t.Errorf("stub %s: got %d nodes, want 1", id, stubs[id])
		}
	}
	if len(stubs) != len(wantStubs) {
		t.Errorf("got stubs %v, want %v", stubs, wantStubs)
	}

	got := map[[3]string]bool{}
	for _, e := range ext.Edges {
		if e.Relation == model.RelContains {
			continue
		}
		tgt := label[e.Target]
		got[[3]string{label[e.Source][0], e.Relation.String(), tgt[0] + ":" + tgt[1]}] = true
		if e.Relation == model.RelReferences && e.Confidence != model.ConfExtracted {
			t.Errorf("references edge to %v is not EXTRACTED", tgt)
		}
	}
	want := [][3]string{
		{"a/x.py", "references", "a/x.py:Local"},
		{"a/x.py", "references", "b/y.py:Uniq"},
		{"a/x.py", "references", "a/x.py:Dup"},
		{"a/x.py", "references", ":Gone"},
		{"a/x.py", "references", ":helper"},
		{"a/x.py", "references", "b/y.py:helper()"},
		{"a/x.py", "inherits", ":Base"},
		{"b/y.py", "references", ":Gone"},
		{"b/y.py", "references", ":Amb"},
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing edge %v", w)
		}
		delete(got, w)
	}
	for g := range got {
		t.Errorf("unexpected edge %v", g)
	}
}

// An inherits TypeRef without Stub drops when its base is outside the corpus.
func TestUnresolvedBaseWithoutStubDrops(t *testing.T) {
	r := refResult("a/x.py", nil, TypeRef{Name: "Base", Relation: "inherits"})
	r.TypeRefs[0].Stub = false
	ext := Resolve([]Result{r}, []string{"a/x.py"})
	for _, n := range ext.Nodes {
		if n.SourceFile == "" {
			t.Errorf("unexpected stub %+v", n)
		}
	}
}

// Upstream 0.9.88 on the same two files (fixtures pyx, py1): parameter and
// return annotations and decorators give references; a plain unresolved base
// and an unresolved name give a sourceless stub.
func TestPythonReferences(t *testing.T) {
	nodes, edges := resolveSrc(t, [][2]string{
		{"pkg/m.py", "class Real:\n    pass\n\n\ndef traced(f):\n    return f\n"},
		{"pkg/a.py", `from typing import Optional, TypeVar
from .m import Real, traced
from .m import Real as R

T = TypeVar("T")


class K(Exception, Generic[T], em.Base):
    @staticmethod
    @traced
    def run(self, x: T, y: "Real", *a: Va, k: Optional[Real] = None) -> Later:
        return None

    @property
    def p(self) -> int:
        return 1

    @p.setter
    def p(self, v: em.Thing):
        pass


class Later:
    pass


@other.deco(1)
@functools.wraps(K)
async def f(a: list[Real], b: traced) -> dict[str, Gone]:
    return {}


class S(R):
    def m(self, x: Annotated[R, Field(gt=0)], y: _):
        pass
`},
	})
	for _, w := range [][3]string{
		{"pkg/a.py:K", "inherits", ":Exception"},
		{"pkg/a.py:K.run()", "references", "pkg/m.py:traced()"},
		{"pkg/a.py:K.run()", "references", ":Va"},
		{"pkg/a.py:K.run()", "references", "pkg/m.py:Real"},
		{"pkg/a.py:K.run()", "references", "pkg/a.py:Later"},
		{"pkg/a.py:K.p()", "references", ":Thing"},
		{"pkg/a.py:f()", "references", ":deco"},
		{"pkg/a.py:f()", "references", "pkg/m.py:Real"},
		{"pkg/a.py:f()", "references", ":Gone"},
		{"pkg/a.py:f()", "references", ":traced"},   // a function is not a type
		{"pkg/a.py:S", "inherits", "pkg/m.py:Real"}, // through the import alias
		{"pkg/a.py:S.m()", "references", "pkg/m.py:Real"},
		{"pkg/a.py:S.m()", "references", ":Field"},
	} {
		if !edges[w] {
			t.Errorf("missing edge %v", w)
		}
	}
	if edges[[3]string{"pkg/a.py:S.m()", "references", "pkg/a.py:a.py"}] {
		t.Error("the name _ folds onto the file id and must give no edge")
	}
	for _, label := range []string{"T", "Generic", "Base", "Optional", "int", "str", "dict", "list", "staticmethod", "property", "setter", "wraps", "p", "R", "gt", "_"} {
		if nodes[[2]string{"", label}] {
			t.Errorf("unexpected stub %q", label)
		}
	}
}
