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
	// A label of two files shows with its file.
	files, dup := map[string]string{}, map[string]bool{}
	for _, n := range ext.Nodes {
		if f, ok := files[n.Label]; ok && f != n.SourceFile {
			dup[n.Label] = true
		}
		files[n.Label] = n.SourceFile
	}
	for _, n := range ext.Nodes {
		if dup[n.Label] {
			label[n.ID] = n.SourceFile + ":" + n.Label
		}
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

// A call binds through the import that names its callee or its receiver: a
// module alias, an import inside the function, and an aliased name. other/mod.py
// makes each name one the name alone cannot bind.
func TestPythonCallsThroughImports(t *testing.T) {
	srcs := map[string]string{
		"pkg/__init__.py": "from .mod import g as reexp\n",
		"pkg/mod.py": `def f(): pass
def g(): pass
class Cls:
    def f(self): pass
`,
		"other/mod.py": "def f(): pass\ndef g(): pass\n",
		"main.py": `import pkg.mod as m
from pkg import mod as mm
from pkg import reexp
from pkg.mod import g as galias

def alias():
    m.f()
def from_mod():
    mm.g()
def from_alias():
    galias()
def via_init():
    reexp()
def local_from():
    from pkg.mod import f
    f()
def local_alias():
    from pkg.mod import f as ff
    ff()
def local_mod():
    from pkg import mod as lm
    lm.g()
def local_import():
    import pkg.mod as pm
    def inner():
        pm.f()
def no_leak():
    ff()
    lm.g()
def shadowed(m, galias):
    m.f()
    galias()
def rebound():
    mm = 1
    mm.g()
class K:
    def meth(self):
        m.g()
`,
	}
	got := pyEdges(srcs, model.RelCalls)
	want := []string{
		"K.meth() -calls-> pkg/mod.py:g()",
		"alias() -calls-> pkg/mod.py:f()",
		"from_alias() -calls-> pkg/mod.py:g()",
		"from_mod() -calls-> pkg/mod.py:g()",
		"inner() -calls-> pkg/mod.py:f()",
		"local_alias() -calls-> pkg/mod.py:f()",
		"local_from() -calls-> pkg/mod.py:f()",
		"local_mod() -calls-> pkg/mod.py:g()",
		"via_init() -calls-> pkg/mod.py:g()",
	}
	if !slices.Equal(got, want) {
		t.Errorf("calls:\n got %q\nwant %q", got, want)
	}
	ext := resolvePy(srcs)
	for _, e := range ext.Edges {
		if e.Relation == model.RelCalls && e.Confidence != model.ConfExtracted {
			t.Errorf("call %s -> %s is %s, want EXTRACTED", e.Source, e.Target, e.Confidence)
		}
	}
}

// A file imports each top-level definition a from-import names, at any depth
// and through a package __init__.py; a submodule, a constant, a method and a
// nested def get no such edge.
func TestPythonImportedSymbolEdges(t *testing.T) {
	srcs := map[string]string{
		"pkg/__init__.py": "from .core import reexp, renamed_src as renamed\n",
		"pkg/core.py": `CONST = 1
def func_a(): pass
def func_b(): pass
def reexp(): pass
def renamed_src(): pass
class Klass:
    def meth_only(self): pass
def outer():
    def nested_fn(): pass
`,
		"pkg/mod.py":   "def in_mod(): pass\ndef starred(): pass\n",
		"pkg/plain.py": "from .core import func_b\n",
		"main.py": `from typing import TYPE_CHECKING
from pkg.core import func_a, func_b as fb
from pkg import mod
from pkg import reexp, renamed
from pkg.core import CONST, missing, meth_only, nested_fn
from pkg.plain import func_b as via_plain
from pkg.mod import *
from main import lazy
if TYPE_CHECKING:
    from pkg.core import Klass
def lazy():
    from pkg.mod import in_mod
    from pkg.core import func_a
`,
	}
	got := pyEdges(srcs, model.RelImports)
	want := []string{
		"main.py -imports-> Klass",
		"main.py -imports-> func_a()",
		"main.py -imports-> func_b()",
		"main.py -imports-> in_mod()",
		// an import of the file itself stays the external it was
		"main.py -imports-> main",
		"main.py -imports-> reexp()",
		"main.py -imports-> renamed_src()",
		"main.py -imports-> typing",
		"pkg/__init__.py -imports-> reexp()",
		"pkg/__init__.py -imports-> renamed_src()",
		"pkg/plain.py -imports-> func_b()",
	}
	if !slices.Equal(got, want) {
		t.Errorf("imports:\n got %q\nwant %q", got, want)
	}
	// The module edges stay as they are: none for the import inside lazy.
	got = pyEdges(srcs, model.RelImportsFrom)
	want = []string{
		"main.py -imports_from-> pkg/__init__.py",
		"main.py -imports_from-> pkg/core.py",
		"main.py -imports_from-> pkg/mod.py",
		"main.py -imports_from-> pkg/plain.py",
		"pkg/__init__.py -imports_from-> pkg/core.py",
		"pkg/plain.py -imports_from-> pkg/core.py",
	}
	if !slices.Equal(got, want) {
		t.Errorf("imports_from:\n got %q\nwant %q", got, want)
	}
}
