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

// Round 1 of the review of the nested defs and the import-bound calls.
func TestPythonNestedAndImportEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		srcs map[string]string
		rels []model.Relation
		want []string
	}{
		{"self in a def nested in a method is the self of the method", map[string]string{"a.py": `class K:
    def save(self): pass
    def m(self):
        def in_m():
            self.save()
            def deeper():
                self.save()
`}, []model.Relation{model.RelCalls}, []string{"deeper() -calls-> K.save()", "in_m() -calls-> K.save()"}},
		{"self in a method of a local class is not", map[string]string{"a.py": `class K:
    def save(self): pass
    def m(self):
        class Local:
            def go(self):
                self.save()
                def inner():
                    self.save()
`}, []model.Relation{model.RelCalls}, nil},
		{"a nested def keeps off the id of a module-level def", map[string]string{"a.py": `def alpha(): pass
def beta(): pass
def load():
    def config():
        beta()
    return config()
def load_config():
    alpha()
`}, []model.Relation{model.RelCalls, model.RelContains}, []string{
			"a.py -contains-> alpha()", "a.py -contains-> beta()", "a.py -contains-> load()", "a.py -contains-> load_config()",
			"config() -calls-> beta()", "load() -calls-> config()", "load() -contains-> config()", "load_config() -calls-> alpha()",
		}},
		{"and off the id of a method", map[string]string{"a.py": `def other(): pass
class K:
    def m_helper(self): pass
    def m(self):
        def helper():
            other()
`}, []model.Relation{model.RelCalls, model.RelContains}, []string{
			"K -contains-> K.m()", "K -contains-> K.m_helper()", "K.m() -contains-> helper()",
			"a.py -contains-> K", "a.py -contains-> other()", "helper() -calls-> other()",
		}},
		{"a default and an annotation of a nested def run in the function around it", map[string]string{"a.py": `def make(): pass
def ann(): pass
def outer(x=make()):
    def inner(x=make()) -> ann():
        pass
`}, []model.Relation{model.RelCalls}, []string{"outer() -calls-> ann()", "outer() -calls-> make()"}},
		{"an import in the function hides the import of the module", map[string]string{
			"utils.py": "def run(): pass\n", "shim.py": "def run(): pass\n",
			"app.py": `from utils import run
def build():
    run()
def deploy():
    from shim import run
    run()
`}, []model.Relation{model.RelCalls}, []string{"build() -calls-> utils.py:run()", "deploy() -calls-> shim.py:run()"}},
		{"an import in a nested def does not reach the function around it", map[string]string{
			"m.py": "def run(): pass\n", "n.py": "def run(): pass\n",
			"nest.py": `import m as x
def outer():
    def inner():
        import n as x
        x.run()
    def sibling():
        x.run()
    x.run()
`}, []model.Relation{model.RelCalls}, []string{"inner() -calls-> n.py:run()", "outer() -calls-> m.py:run()", "sibling() -calls-> m.py:run()"}},
		{"a from-imported name is no external module", map[string]string{"pkg/a.py": `from sqlalchemy import orm
from sqlalchemy.orm import Session
from . import x
from .x import y
def f():
    orm.sessionmaker()
    x.go()
`}, []model.Relation{model.RelCalls}, nil},
		{"a from-import in a class body", map[string]string{
			"mod.py": "def top(): pass\n",
			"cb.py":  "class Cls:\n    from mod import top\n",
		}, []model.Relation{model.RelImports}, []string{"cb.py -imports-> top()"}},
		// Round 2.
		{"two nested defs of one id are one node that both names reach", map[string]string{
			"other.py": "def helper(): pass\n",
			"m.py": `def under():
    def _helper(): pass
    def helper():
        return _helper()
    return helper()
`}, []model.Relation{model.RelCalls}, []string{"under() -calls-> _helper()"}},
		{"an import in the body of a local class is no name of its methods", map[string]string{
			"a.py": "def dumps(): pass\n", "b.py": "def dumps(): pass\n",
			"m.py": `from a import dumps
def local_cls():
    class C:
        from b import dumps
        def m(self):
            return dumps()
    dumps()
`}, []model.Relation{model.RelCalls}, []string{"local_cls() -calls-> a.py:dumps()", "m() -calls-> a.py:dumps()"}},
		{"a name the function imports twice or as a module binds nothing", map[string]string{
			"a.py": "def f(): pass\n", "b.py": "def f(): pass\n", "c.py": "def f(): pass\n",
			"app.py": `from a import f
def cond(x):
    if x:
        from b import f
    else:
        from c import f
    f()
def plain():
    import b as f
    f()
def ext():
    from extlib import f
    f()
`}, []model.Relation{model.RelCalls}, nil},
		{"the path of the import binds before the stem of its module", map[string]string{
			"a/util.py": "def fmt(): pass\n",
			"b/core.py": "def fmt(): pass\n",
			"b/util.py": "from .core import fmt\n",
			"main.py":   "from b.util import fmt\ndef main():\n    fmt()\n",
		}, []model.Relation{model.RelCalls}, []string{"main() -calls-> b/core.py:fmt()"}},
		{"a name the package binds is not its submodule", map[string]string{
			"pkg/__init__.py": "from .config import config\n",
			"pkg/config.py":   "class Config:\n    def load(self): pass\ndef load(): pass\ndef config(): pass\n",
			"main.py":         "from pkg import config\ndef main():\n    config.load()\n",
		}, []model.Relation{model.RelCalls}, nil},
		{"a from-import of a bare relative module", map[string]string{
			"pkg/__init__.py":     "def top(): pass\n",
			"pkg/a.py":            "from . import top\n",
			"pkg/sub/__init__.py": "from .. import top\n",
			"far.py":              "from pkg.sub import top\n",
		}, []model.Relation{model.RelImports}, []string{
			"far.py -imports-> top()", "pkg/a.py -imports-> top()", "pkg/sub/__init__.py -imports-> top()",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pyEdges(c.srcs, c.rels...); !slices.Equal(got, c.want) {
				t.Errorf("edges:\n got %q\nwant %q", got, c.want)
			}
		})
	}
}
