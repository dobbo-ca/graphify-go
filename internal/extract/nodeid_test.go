package extract

import "testing"

// resolveSrc extracts each in-memory file and resolves the set.
func resolveSrc(t *testing.T, srcs [][2]string) (map[[2]string]bool, map[[3]string]bool) {
	t.Helper()
	var results []Result
	var files []string
	for _, s := range srcs {
		results = append(results, FileFromBytes(s[0], []byte(s[1])))
		files = append(files, s[0])
	}
	ext := Resolve(results, files)
	nodes := map[[2]string]bool{} // source file, label
	at := map[string][2]string{}
	for _, n := range ext.Nodes {
		k := [2]string{n.SourceFile, n.Label}
		nodes[k] = true
		at[n.ID] = k
	}
	edges := map[[3]string]bool{} // source file/label, relation, target file/label
	for _, e := range ext.Edges {
		s, d := at[e.Source], at[e.Target]
		edges[[3]string{s[0] + ":" + s[1], e.Relation.String(), d[0] + ":" + d[1]}] = true
	}
	return nodes, edges
}

// Files that share a directory name and a stem keep their own symbols.
func TestNodeIDsDoNotCollideAcrossFiles(t *testing.T) {
	py := "class ApiClient:\n    def get(self):\n        pass\n\ndef helper():\n    pass\n\ndef only_py():\n    helper()\n"
	rb := "class ApiClient\n  def get\n  end\nend\n\ndef helper\nend\n"
	nodes, edges := resolveSrc(t, [][2]string{
		{"pkg/sample.py", py},
		{"pkg/sample.rb", rb},
		{"a/x/foo.go", "package x\n\nfunc Helper() {}\n\nfunc OnlyA() { Helper() }\n"},
		{"b/x/foo.go", "package x\n\nfunc Helper() {}\n\nfunc OnlyB() { Helper() }\n"},
	})
	for _, want := range [][2]string{
		{"pkg/sample.py", "ApiClient"}, {"pkg/sample.py", "ApiClient.get()"}, {"pkg/sample.py", "helper()"},
		{"pkg/sample.rb", "ApiClient"}, {"pkg/sample.rb", "ApiClient.get()"}, {"pkg/sample.rb", "helper()"},
		{"a/x/foo.go", "Helper()"}, {"b/x/foo.go", "Helper()"},
	} {
		if !nodes[want] {
			t.Errorf("missing node %v", want)
		}
	}
	for _, want := range [][3]string{
		{"pkg/sample.py:only_py()", "calls", "pkg/sample.py:helper()"},
		{"a/x/foo.go:OnlyA()", "calls", "a/x/foo.go:Helper()"},
		{"b/x/foo.go:OnlyB()", "calls", "b/x/foo.go:Helper()"},
		{"pkg/sample.py:sample.py", "contains", "pkg/sample.py:helper()"},
	} {
		if !edges[want] {
			t.Errorf("missing edge %v", want)
		}
	}
	for e := range edges {
		if e[1] == "contains" && e[0][:len("pkg/sample.py")] == "pkg/sample.py" && e[2][:len("pkg/sample.rb")] == "pkg/sample.rb" {
			t.Errorf("contains edge crosses files: %v", e)
		}
	}
}

// A name that is all punctuation must not take the id of its file.
func TestGoBlankFuncKeepsItsNode(t *testing.T) {
	nodes, edges := resolveSrc(t, [][2]string{
		{"pkg/a.go", "package pkg\n\nfunc _() { Helper() }\n\nfunc Helper() {}\n"},
		{"py/i18n.py", "def _(s):\n    return s\n"},
		{"c/gt.c", "static int _(int s) { return s; }\n"},
		{"rb/x.rb", "def _(s)\nend\n"},
		{"php/t.php", "<?php\nfunction __($s) { return $s; }\n"},
	})
	for _, want := range [][2]string{{"pkg/a.go", "_()"}, {"py/i18n.py", "_()"}, {"c/gt.c", "_()"}, {"rb/x.rb", "_()"}, {"php/t.php", "__()"}} {
		if !nodes[want] {
			t.Errorf("missing %v: %v", want, nodes)
		}
	}
	if !edges[[3]string{"pkg/a.go:_()", "calls", "pkg/a.go:Helper()"}] || edges[[3]string{"pkg/a.go:a.go", "contains", "pkg/a.go:a.go"}] {
		t.Errorf("edges: %v", edges)
	}
}

// A call on an external module keeps its module edge when a definition of
// another language has the same qualified name.
func TestExternalModuleCallSurvivesCrossLanguageName(t *testing.T) {
	_, edges := resolveSrc(t, [][2]string{
		{"py/app.py", "import cv2 as cv\n\ndef f(a):\n    return cv.fitEllipse(a)\n"},
		{"src/shape.cpp", "void cv::fitEllipse(int a) {}\n"},
	})
	found := false
	for e := range edges {
		if e[0] == "py/app.py:f()" && e[1] == "calls" {
			found = true
		}
	}
	if !found {
		t.Errorf("f() lost its call to cv2: %v", edges)
	}
}

// The owner of a method is a type of the same language, its own file first.
func TestSiblingOwnerRules(t *testing.T) {
	_, edges := resolveSrc(t, [][2]string{
		{"p.h", "class Msg;\nclass Msg { public: void run(); };\n"},
		{"p.cc", "void Msg::run() {}\n"},
		{"a/widget.go", "package a\n\ntype Widget struct{}\n\nfunc (w Widget) Run() {}\n"},
		{"a/widget.py", "class Widget:\n    pass\n"},
	})
	if want := [3]string{"p.h:Msg", "contains", "p.cc:Msg.run()"}; !edges[want] {
		t.Errorf("missing edge %v\n%v", want, edges)
	}
	if edges[[3]string{"a/widget.go:Widget", "contains", "a/widget.go:Widget.Run()"}] {
		t.Error("Go method got a type contains edge")
	}
	if edges[[3]string{"a/widget.py:Widget", "contains", "a/widget.go:Widget.Run()"}] {
		t.Error("Go method owned by a Python class")
	}
}

// An out-of-line C++ method stays under its class in the sibling header.
func TestCppOutOfLineMethodOwnerInHeader(t *testing.T) {
	nodes, edges := resolveSrc(t, [][2]string{
		{"lib/foo.h", "class Foo {\npublic:\n  void bar();\n  void baz() {}\n};\n"},
		{"lib/foo.cpp", "#include \"foo.h\"\nvoid Foo::bar() { baz(); }\n"},
	})
	if !nodes[[2]string{"lib/foo.cpp", "Foo.bar()"}] {
		t.Fatalf("missing Foo.bar() in foo.cpp: %v", nodes)
	}
	for _, want := range [][3]string{
		{"lib/foo.h:Foo", "contains", "lib/foo.cpp:Foo.bar()"},
		{"lib/foo.cpp:foo.cpp", "contains", "lib/foo.cpp:Foo.bar()"},
		{"lib/foo.cpp:Foo.bar()", "calls", "lib/foo.h:Foo.baz()"},
	} {
		if !edges[want] {
			t.Errorf("missing edge %v\n%v", want, edges)
		}
	}
}
