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
		{"pkg/b.py", `from .m import Real as Base
from .m import traced as Late
from .a import Later as Late


class Real(Base):
    def clone(self, other: Base, l: Late) -> Base:
        return other
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
		{"pkg/b.py:Real", "inherits", "pkg/m.py:Real"}, // not the class of the same name here
		{"pkg/b.py:Real.clone()", "references", "pkg/m.py:Real"},
		{"pkg/b.py:Real.clone()", "references", "pkg/a.py:Later"}, // the last import of Late
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

// Upstream fixture r3o: the import statement of an alias names its module.
func TestPythonAliasBindsItsOwnModule(t *testing.T) {
	_, edges := resolveSrc(t, [][2]string{
		{"a/models.py", "class User:\n    pass\n"},
		{"b/models.py", "class User:\n    pass\n"},
		{"a/views.py", "from a.models import User as AUser\nfrom b.models import User as BUser\n\n\nclass Remote(BUser):\n    pass\n\n\ndef handle(u: BUser, v: AUser):\n    pass\n"},
	})
	for _, w := range [][3]string{
		{"a/views.py:Remote", "inherits", "b/models.py:User"},
		{"a/views.py:handle()", "references", "b/models.py:User"},
		{"a/views.py:handle()", "references", "a/models.py:User"},
	} {
		if !edges[w] {
			t.Errorf("missing edge %v", w)
		}
	}
}

// Upstream fixtures r4p, r4o, r4q: a fallback alias from outside the corpus
// leaves the plain import in force; two imports on one line keep their own
// module; `from . import Name as N` is an alias too.
func TestPythonAliasImportOrder(t *testing.T) {
	_, edges := resolveSrc(t, [][2]string{
		{"pkg/__init__.py", "class Name:\n    pass\n"},
		{"pkg/fast.py", "class Foo:\n    pass\n\n\nclass X:\n    pass\n"},
		{"pkg/c.py", "class X:\n    pass\n"},
		{"pkg/app.py", `try:
    from .fast import Foo
except ImportError:
    from slowext import SlowFoo as Foo
from .fast import X as P; from .c import X as R
from . import Name as N


class K(Foo):
    def m(self, p: P, r: R, n: N):
        pass
`},
	})
	for _, w := range [][3]string{
		{"pkg/app.py:K", "inherits", "pkg/fast.py:Foo"},
		{"pkg/app.py:K.m()", "references", "pkg/fast.py:X"},
		{"pkg/app.py:K.m()", "references", "pkg/c.py:X"},
		{"pkg/app.py:K.m()", "references", "pkg/__init__.py:Name"},
	} {
		if !edges[w] {
			t.Errorf("missing edge %v", w)
		}
	}
}

// Upstream fixtures r6g, r6e: a plain import binds in its own module although
// another module has the name; a qualified name is not the file's alias.
func TestPythonPlainImportAndQualifiedName(t *testing.T) {
	_, edges := resolveSrc(t, [][2]string{
		{"p1.py", "class Config:\n    pass\n"},
		{"p2.py", "class Config:\n    pass\n\n\nclass R:\n    pass\n\n\ndef load():\n    pass\n"},
		{"lib.py", "class Real:\n    pass\n"},
		{"user.py", "import p2\nfrom p1 import Config\nfrom p2 import load\nfrom lib import Real as R\n\n\nclass Mine(Config):\n    pass\n\n\nclass KQ(p2.R):\n    pass\n\n\nclass KS(p2.R[int]):\n    pass\n"},
	})
	for _, w := range [][3]string{
		{"user.py:Mine", "inherits", "p1.py:Config"},
		{"user.py:KQ", "inherits", "p2.py:R"},
		{"user.py:KS", "inherits", "p2.py:R"},
	} {
		if !edges[w] {
			t.Errorf("missing edge %v", w)
		}
	}
}

// Upstream 0.9.88 on fixture gox: signature and struct field types give
// references; pkg.T binds through the import path or gets a stub labelled
// with it. Not copied: the same-package Dup left on a stub, no edge for a
// variadic parameter.
func TestGoReferences(t *testing.T) {
	nodes, edges := resolveSrc(t, [][2]string{
		{"go.mod", "example.com/m"},
		{"lib/t.go", "package lib\n\ntype Thing struct{}\n\ntype Dup struct{}\n"},
		{"app/b.go", "package app\n\nfunc G(l Local, d Dup) Inner { return 0 }\n"},
		{"app/a.go", `package app

import (
	"context"
	"io"

	lb "example.com/m/lib"
)

type Dup struct{}

type Inner int

type Gen[T any] struct{ V T }

type Num interface{ ~int | Inner }

type Tilde interface{ ~[]Dup }

type Emb struct {
	Gen[lb.Thing]
}

type Local struct {
	A lb.Thing
	B Gen[io.Reader]
	C struct{ In Inner }
	D func(ctx context.Context) (*Local, error)
	E []*lb.Dup
	F Dup
	G unknownpkg.T
	Inner
}

func (g *Gen[T]) Get(d Dup, x T) lb.Dup { return lb.Dup{} }

func F[U any](l *Local, u U, vs ...Inner) (n int, err error) { return }
`},
	})
	for _, w := range [][3]string{
		{"app/a.go:Local", "references", "lib/t.go:Thing"},
		{"app/a.go:Local", "references", "app/a.go:Gen"},
		{"app/a.go:Local", "references", ":io.Reader"},
		{"app/a.go:Local", "references", "app/a.go:Inner"}, // field C; the embedded Inner is no reference
		{"app/a.go:Local", "references", ":context.Context"},
		{"app/a.go:Local", "references", "lib/t.go:Dup"},
		{"app/a.go:Local", "references", "app/a.go:Dup"},
		{"app/a.go:Local", "references", ":unknownpkg.T"},
		{"app/a.go:Num", "references", "app/a.go:Inner"},
		{"app/a.go:Tilde", "references", "app/a.go:Dup"},
		{"app/a.go:Emb", "references", "lib/t.go:Thing"},
		{"app/a.go:Gen.Get()", "references", "app/a.go:Dup"},
		{"app/a.go:Gen.Get()", "references", "lib/t.go:Dup"},
		{"app/a.go:F()", "references", "app/a.go:Local"},
		{"app/a.go:F()", "references", "app/a.go:Inner"},
		{"app/b.go:G()", "references", "app/a.go:Dup"},
		{"app/b.go:G()", "references", "app/a.go:Local"},
	} {
		if !edges[w] {
			t.Errorf("missing edge %v", w)
		}
	}
	for _, label := range []string{"T", "U", "int", "error", "Dup", "Reader", "Context"} {
		if nodes[[2]string{"", label}] {
			t.Errorf("unexpected stub %q", label)
		}
	}
}
