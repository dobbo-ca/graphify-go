package extract

import "testing"

// extractAll runs the real dispatch over in-memory sources and resolves them.
func extractAll(t *testing.T, srcs map[string]string) (edges []struct{ Src, Rel, Tgt string }) {
	t.Helper()
	var results []Result
	var files []string
	for rel, src := range srcs {
		results = append(results, FileFromBytes(rel, []byte(src)))
		files = append(files, rel)
	}
	ext := Resolve(results, files)
	label := map[string]string{}
	for _, n := range ext.Nodes {
		label[n.ID] = n.Label
	}
	for _, e := range ext.Edges {
		edges = append(edges, struct{ Src, Rel, Tgt string }{label[e.Source], e.Relation, label[e.Target]})
	}
	return edges
}

func wantEdges(t *testing.T, srcs map[string]string, want [][3]string, unwanted ...[3]string) {
	t.Helper()
	got := extractAll(t, srcs)
	has := func(w [3]string) bool {
		for _, e := range got {
			if e.Src == w[0] && e.Rel == w[1] && e.Tgt == w[2] {
				return true
			}
		}
		return false
	}
	for _, w := range want {
		if !has(w) {
			t.Errorf("missing edge %s --%s--> %s; got %v", w[0], w[1], w[2], got)
		}
	}
	for _, w := range unwanted {
		if has(w) {
			t.Errorf("unexpected edge %s --%s--> %s", w[0], w[1], w[2])
		}
	}
}

// Acceptance: `class B extends A implements I` yields inherits and implements
// edges that resolve across files.
func TestJavaInheritsImplements(t *testing.T) {
	wantEdges(t, map[string]string{
		"pkg/A.java": "class A { }\ninterface I { }\ninterface J extends I { }\n",
		"pkg/B.java": "import pkg.A;\nclass B extends A implements I, J { }\n",
	}, [][3]string{
		{"B", "inherits", "A"},
		{"B", "implements", "I"},
		{"B", "implements", "J"},
		{"J", "inherits", "I"},
	})
}

// A generic, qualified supertype still binds to the bare type name.
func TestJavaGenericQualifiedBase(t *testing.T) {
	wantEdges(t, map[string]string{
		"Base.java": "class Base { }\n",
		"Sub.java":  "class Sub extends foo.Base<String> { }\n",
	}, [][3]string{{"Sub", "inherits", "Base"}})
}

func TestCSharpBases(t *testing.T) {
	wantEdges(t, map[string]string{
		"lib.cs":  "interface IThing { }\ninterface IOther { }\nclass Base { }\n",
		"impl.cs": "class Impl : Base, IThing, IOther { }\ninterface ISub : IThing { }\n",
	}, [][3]string{
		{"Impl", "inherits", "Base"},
		{"Impl", "implements", "IThing"},
		{"Impl", "implements", "IOther"},
		{"ISub", "inherits", "IThing"},
	}, [3]string{"Impl", "inherits", "IThing"})
}

func TestPythonInherits(t *testing.T) {
	wantEdges(t, map[string]string{
		"base.py": "class Base:\n    pass\n\nclass Meta(type):\n    pass\n",
		"sub.py":  "from base import Base, Meta\n\nclass Sub(Base, metaclass=Meta):\n    pass\n",
	}, [][3]string{
		{"Sub", "inherits", "Base"},
	}, [3]string{"Sub", "inherits", "Meta"})
}

// A base class outside the corpus drops rather than creating a stub node.
func TestUnresolvedBaseDrops(t *testing.T) {
	got := extractAll(t, map[string]string{"sub.py": "class Sub(SomeLibBase):\n    pass\n"})
	for _, e := range got {
		if e.Rel == "inherits" {
			t.Errorf("unexpected inherits edge for out-of-corpus base: %v", e)
		}
	}
}

// A base class with an explicit constructor registers a member named after the
// class, which must not make `extends A` ambiguous — same file, same package,
// and cross-file.
func TestJavaBaseWithConstructor(t *testing.T) {
	wantEdges(t, map[string]string{
		"pkg/A.java":   "class A { A() {} }\n",
		"pkg/B.java":   "class B extends A { B() {} }\n",
		"other/C.java": "import pkg.A;\nclass C extends A { }\n",
		"Same.java":    "class P { P() {} }\nclass Q extends P { }\n",
	}, [][3]string{
		{"B", "inherits", "A"},
		{"C", "inherits", "A"},
		{"Q", "inherits", "P"},
	})
}
