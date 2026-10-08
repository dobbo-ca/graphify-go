package extract

import (
	"reflect"
	"sort"
	"testing"
)

// importEdges extracts each source by extension, resolves the corpus with the
// given go.mod dir -> module path table, and returns the sorted import edges.
func importEdges(srcs map[string]string, goMods map[string]string) []string {
	results := []Result{{GoMods: goMods}}
	var files []string
	for f, s := range srcs {
		results = append(results, FileFromBytes(f, []byte(s)))
		files = append(files, f)
	}
	sort.Strings(files)
	var edges []string
	for _, e := range Resolve(results, files).Edges {
		if e.Relation == "imports" || e.Relation == "imports_from" {
			edges = append(edges, e.Source+" "+e.Relation+" "+e.Target)
		}
	}
	sort.Strings(edges)
	return edges
}

func TestLocalImportsResolveToFiles(t *testing.T) {
	root := map[string]string{".": "example.com/m"}
	cases := []struct {
		name   string
		srcs   map[string]string
		goMods map[string]string
		want   []string
	}{
		{"go package links every non-test file", map[string]string{
			"main.go":        "package main\nimport (\n\t\"fmt\"\n\t\"example.com/m/util\"\n)\n",
			"util/a.go":      "package util\n",
			"util/b_test.go": "package util\n",
		}, root, []string{"main_go imports fmt", "main_go imports_from util_a_go"}},
		// No edge into a_test.go, so no cycle with b.
		{"go external test importing its importer", map[string]string{
			"a/a.go": "package a\n", "a/a_test.go": "package a_test\nimport \"example.com/m/b\"\n",
			"b/b.go": "package b\nimport \"example.com/m/a\"\n",
		}, root, []string{"a_a_test_go imports_from b_b_go", "b_b_go imports_from a_a_go"}},
		{"go nested module wins", map[string]string{
			"main.go":       "package main\nimport \"example.com/m/sub/util\"\nimport \"example.com/sub/util\"\n",
			"sub/util/a.go": "package util\n",
		}, map[string]string{".": "example.com/m", "sub": "example.com/sub"},
			[]string{"main_go imports example_com_m_sub_util", "main_go imports_from sub_util_a_go"}},
		{"go external test package skips itself", map[string]string{
			"util/a.go": "package util\n", "util/a_test.go": "package util_test\nimport \"example.com/m/util\"\n",
		}, root, []string{"util_a_test_go imports_from util_a_go"}},
		{"go module under a dot directory", map[string]string{
			".tools/gen/main.go":   "package main\nimport \"example.com/dot/util\"\n",
			".tools/gen/util/u.go": "package util\n",
		}, map[string]string{".tools/gen": "example.com/dot"},
			[]string{"tools_gen_main_go imports_from tools_gen_util_u_go"}},
		{"go without go.mod stays external", map[string]string{
			"main.go": "package main\nimport \"example.com/m/util\"\n", "util/a.go": "package util\n",
		}, nil, []string{"main_go imports example_com_m_util"}},
		{"c include beside the file", map[string]string{
			"src/main.c": "#include \"h.h\"\n#include <stdio.h>\n#include \"gone.h\"\n", "src/h.h": "", "src/stdio.h": "",
		}, nil, []string{"src_main_c imports gone_h", "src_main_c imports stdio_h", "src_main_c imports_from src_h_h"}},
		{"cpp include beside the file", map[string]string{
			"src/w.cpp": "#include \"w.hpp\"\n#include <vector>\n", "src/w.hpp": "",
		}, nil, []string{"src_w_cpp imports vector", "src_w_cpp imports_from src_w_hpp"}},
		{"rust mod and crate paths", map[string]string{
			"src/main.rs":      "mod helper;\nmod deep;\nmod gone;\nuse crate::helper::assist;\nuse crate::deep::leaf;\nuse std::fmt;\n",
			"src/helper.rs":    "pub fn assist() {}\n",
			"src/deep/mod.rs":  "pub mod leaf;\nuse super::helper;\nuse self::leaf::Leaf;\n",
			"src/deep/leaf.rs": "use crate::{helper, deep::Other};\n",
		}, nil, []string{
			"src_deep_leaf_rs imports_from src_deep_mod_rs",
			"src_deep_leaf_rs imports_from src_helper_rs",
			"src_deep_mod_rs imports_from src_deep_leaf_rs",
			"src_deep_mod_rs imports_from src_helper_rs",
			"src_main_rs imports self_gone",
			"src_main_rs imports std",
			"src_main_rs imports_from src_deep_leaf_rs",
			"src_main_rs imports_from src_deep_mod_rs",
			"src_main_rs imports_from src_helper_rs",
		}},
		{"rust mod under a non-mod.rs file", map[string]string{
			"src/lib.rs": "mod a;\n", "src/a.rs": "mod b;\n", "src/a/b.rs": "use super::super::a;\n",
		}, nil, []string{"src_a_b_rs imports_from src_a_rs", "src_a_rs imports_from src_a_b_rs", "src_lib_rs imports_from src_a_rs"}},
		{"ruby require_relative", map[string]string{
			"app/main.rb":  "require_relative 'lib/h'\nrequire_relative './lib/h'\nrequire 'json'\nrequire_relative 'gone'\n",
			"app/lib/h.rb": "", "app/json.rb": "",
		}, nil, []string{"app_main_rb imports gone", "app_main_rb imports json", "app_main_rb imports_from app_lib_h_rb"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := importEdges(tc.srcs, tc.goMods); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}
