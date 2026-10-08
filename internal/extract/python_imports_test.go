package extract

import (
	"sort"
	"testing"
)

// pyResolve extracts each Python source and resolves the corpus, returning the
// sorted "src rel tgt[ type]" import edges and the node ids.
func pyResolve(srcs map[string]string) ([]string, map[string]bool) {
	var results []Result
	var files []string
	for f, s := range srcs {
		results = append(results, extractPython(f, []byte(s)))
		files = append(files, f)
	}
	ext := Resolve(results, files)
	var edges []string
	for _, e := range ext.Edges {
		if e.Relation != "imports" && e.Relation != "imports_from" {
			continue
		}
		s := e.Source + " " + e.Relation + " " + e.Target
		if e.TypeOnly {
			s += " type"
		}
		edges = append(edges, s)
	}
	sort.Strings(edges)
	ids := map[string]bool{}
	for _, n := range ext.Nodes {
		ids[n.ID] = true
	}
	return edges, ids
}

func TestPythonImportsResolveToFiles(t *testing.T) {
	cases := []struct {
		name string
		srcs map[string]string
		want []string
	}{
		{"sibling cycle", map[string]string{"pkg/a.py": "import b\n", "pkg/b.py": "import a\n"},
			[]string{"pkg_a_py imports_from pkg_b_py", "pkg_b_py imports_from pkg_a_py"}},
		{"relative module", map[string]string{"pkg/a.py": "from .b import helper\n", "pkg/b.py": ""},
			[]string{"pkg_a_py imports_from pkg_b_py"}},
		{"from dot import module", map[string]string{"pkg/a.py": "from . import b\n", "pkg/b.py": ""},
			[]string{"pkg_a_py imports_from pkg_b_py"}},
		{"from package import submodule", map[string]string{
			"pkg/a.py": "from .sub import mod\n", "pkg/sub/__init__.py": "", "pkg/sub/mod.py": ""},
			[]string{"pkg_a_py imports_from pkg_sub_init_py", "pkg_a_py imports_from pkg_sub_mod_py"}},
		{"parent relative", map[string]string{"pkg/sub/a.py": "from ..b import x\n", "pkg/b.py": ""},
			[]string{"pkg_sub_a_py imports_from pkg_b_py"}},
		{"absolute dotted", map[string]string{
			"main.py": "import pkg.b\nfrom pkg.a import A\n", "pkg/a.py": "", "pkg/b.py": ""},
			[]string{"main_py imports_from pkg_a_py", "main_py imports_from pkg_b_py"}},
		{"ambiguous absolute drops", map[string]string{"src/a.py": "import b\n", "src/b.py": "", "b.py": ""},
			[]string{"src_a_py imports b"}},
		{"sibling of a package stays external", map[string]string{
			"pkg/__init__.py": "", "pkg/a.py": "import json\n", "pkg/json.py": ""},
			[]string{"pkg_a_py imports json"}},
		{"nested blocks", map[string]string{
			"a.py": "try:\n    import b\nexcept ImportError:\n    import c\nwith x:\n    import d\n",
			"b.py": "", "c.py": "", "d.py": ""},
			[]string{"a_py imports_from b_py", "a_py imports_from c_py", "a_py imports_from d_py"}},
		{"type checking", map[string]string{
			"a.py": "import typing\nif typing.TYPE_CHECKING:\n    import b\nelse:\n    import c\nif TYPE_CHECKING:\n    import d\n",
			"b.py": "", "c.py": "", "d.py": ""},
			[]string{"a_py imports typing", "a_py imports_from b_py type", "a_py imports_from c_py", "a_py imports_from d_py type"}},
		{"negated or compound type checking runs", map[string]string{
			"a.py": "import typing\nif not typing.TYPE_CHECKING:\n    import b\nif x or typing.TYPE_CHECKING:\n    import c\n",
			"b.py": "", "c.py": ""},
			[]string{"a_py imports typing", "a_py imports_from b_py", "a_py imports_from c_py"}},
		{"future and unresolved dot", map[string]string{
			"pkg/a.py": "from __future__ import annotations\nfrom . import missing\n"}, nil},
	}
	for _, c := range cases {
		got, ids := pyResolve(c.srcs)
		if len(got) != len(c.want) {
			t.Errorf("%s: edges = %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: edges = %v, want %v", c.name, got, c.want)
				break
			}
		}
		if ids[""] || ids["annotations"] || ids["future"] {
			t.Errorf("%s: phantom node in %v", c.name, ids)
		}
	}
}
