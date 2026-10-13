package extract

import (
	"fmt"
	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
	"runtime"
	"slices"
	"testing"
	"time"
)

// hasCall scans resolved edges for a src->tgt calls edge.
func hasCall(edges []edge, src, tgt string) bool {
	for _, e := range edges {
		if e.Relation == "calls" && e.Source == src && e.Target == tgt {
			return true
		}
	}
	return false
}

type edge = struct {
	Source, Target, Relation string
}

// TestResolveAmbiguousByImport checks that a call to a name defined in two files
// resolves to the one the caller actually imports, instead of being skipped.
func TestResolveAmbiguousByImport(t *testing.T) {
	results := []Result{
		{Defs: []Def{{ID: "A", Name: "helper", File: "a/util.js"}}},
		{Defs: []Def{{ID: "B", Name: "helper", File: "b/util.js"}}},
		{
			Defs:  []Def{{ID: "MAIN", Name: "run", File: "main.js"}},
			Calls: []Call{{CallerID: 1, Callee: 2, Loc: model.Line(2)}}, CallText: []string{"", "MAIN", "helper"}, File: "main.js",
			Imps: []Imp{{FileID: "mainfile", File: "main.js", Spec: "./a/util", Loc: model.Line(1)}},
		},
	}
	files := []string{"a/util.js", "b/util.js", "main.js"}
	ext := Resolve(results, files)

	var es []edge
	for _, e := range ext.Edges {
		es = append(es, edge{e.Source, e.Target, e.Relation.String()})
	}
	if !hasCall(es, "MAIN", "A") {
		t.Error("expected MAIN --calls--> A (the imported helper)")
	}
	if hasCall(es, "MAIN", "B") {
		t.Error("did not expect MAIN --calls--> B (not imported)")
	}
}

// TestResolveNoCrossFamilyCall checks that a call whose name is uniquely defined
// only in a different language family never binds: a Python call to `process`
// must not resolve to a Java `process` method (a phantom name collision), while
// a call resolvable within its own family still binds.
func TestResolveNoCrossFamilyCall(t *testing.T) {
	results := []Result{
		{Defs: []Def{{ID: "JAVA", Name: "process", File: "Svc.java"}}},
		{Defs: []Def{{ID: "PYHELPER", Name: "helper", File: "util.py"}}},
		{
			Defs: []Def{{ID: "PYMAIN", Name: "run", File: "main.py"}},
			File: "main.py",
			Calls: []Call{
				{CallerID: 1, Callee: 2, Loc: model.Line(2)}, // cross-family, must not bind
				{CallerID: 1, Callee: 3, Loc: model.Line(3)}, // same-family, must bind
			}, CallText: []string{"", "PYMAIN", "process", "helper"},
		},
	}
	files := []string{"Svc.java", "util.py", "main.py"}
	ext := Resolve(results, files)

	var es []edge
	for _, e := range ext.Edges {
		es = append(es, edge{e.Source, e.Target, e.Relation.String()})
	}
	if hasCall(es, "PYMAIN", "JAVA") {
		t.Error("did not expect PYMAIN --calls--> JAVA (cross language family)")
	}
	if !hasCall(es, "PYMAIN", "PYHELPER") {
		t.Error("expected PYMAIN --calls--> PYHELPER (same language family)")
	}
}

// TestResolveAmbiguousBySameDir checks the same-package (same directory)
// tiebreaker: an ambiguous name with one definition in the caller's directory
// resolves there, mirroring same-package calls in Go.
func TestResolveAmbiguousBySameDir(t *testing.T) {
	results := []Result{
		{Defs: []Def{{ID: "LOCAL", Name: "doIt", File: "pkg/a.go"}}},
		{Defs: []Def{{ID: "FAR", Name: "doIt", File: "other/b.go"}}},
		{
			Defs:  []Def{{ID: "CALLER", Name: "run", File: "pkg/c.go"}},
			Calls: []Call{{CallerID: 1, Callee: 2, Loc: model.Line(3)}}, CallText: []string{"", "CALLER", "doIt"}, File: "pkg/c.go",
		},
	}
	files := []string{"pkg/a.go", "other/b.go", "pkg/c.go"}
	ext := Resolve(results, files)

	var es []edge
	for _, e := range ext.Edges {
		es = append(es, edge{e.Source, e.Target, e.Relation.String()})
	}
	if !hasCall(es, "CALLER", "LOCAL") {
		t.Error("expected CALLER --calls--> LOCAL (same directory)")
	}
	if hasCall(es, "CALLER", "FAR") {
		t.Error("did not expect CALLER --calls--> FAR (different directory)")
	}
}

// TestResolveSameNameMethodsInOneFile checks that when one file declares two
// types each owning a method of the same name, an unqualified call to that name
// binds to neither (drop-on-ambiguity) instead of the last one parsed, while an
// unambiguous same-file name still resolves.
func TestResolveSameNameMethodsInOneFile(t *testing.T) {
	results := []Result{{
		Defs: []Def{
			{ID: "x_a_build", Name: "Build", File: "x.cs"},
			{ID: "x_a_helper", Name: "Helper", File: "x.cs"},
			{ID: "x_b_helper", Name: "Helper", File: "x.cs"},
			{ID: "x_b_only", Name: "Only", File: "x.cs"},
		},
		File: "x.cs",
		Calls: []Call{
			{CallerID: 1, Callee: 2, Loc: model.Line(3)},
			{CallerID: 1, Callee: 3, Loc: model.Line(4)},
		}, CallText: []string{"", "x_a_build", "Helper", "Only"},
	}}
	ext := Resolve(results, []string{"x.cs"})

	var es []edge
	for _, e := range ext.Edges {
		es = append(es, edge{e.Source, e.Target, e.Relation.String()})
	}
	if hasCall(es, "x_a_build", "x_b_helper") {
		t.Error("did not expect x_a_build --calls--> x_b_helper (ambiguous same-file name)")
	}
	if hasCall(es, "x_a_build", "x_a_helper") {
		t.Error("did not expect a guessed edge to x_a_helper either; the name is ambiguous")
	}
	if !hasCall(es, "x_a_build", "x_b_only") {
		t.Error("expected x_a_build --calls--> x_b_only (unambiguous same-file name)")
	}
}

// TestResolveReceiverCalls checks that a member or qualified call binds on its
// receiver, not on the bare method name.
func TestResolveReceiverCalls(t *testing.T) {
	c := func(src, tgt string) [3]string { return [3]string{src, "calls", tgt} }
	for _, tc := range []struct {
		name           string
		srcs           map[string]string
		want, unwanted [][3]string
	}{
		{name: "go stdlib package", srcs: map[string]string{
			"a.go": "package a\nimport \"errors\"\nfunc New() int { return 1 }\nfunc f() error { return errors.New(\"x\") }\n",
		}, unwanted: [][3]string{c("f()", "New()")}},
		{name: "rust foreign type", srcs: map[string]string{
			"a.rs": "struct Config;\nimpl Config { fn new() -> Config { Config } }\nfn a() { let _m = HashMap::new(); }\nfn b() { let _c = Config::new(); }\n",
		}, want: [][3]string{c("b()", "Config.new()")}, unwanted: [][3]string{c("a()", "Config.new()")}},
		{name: "ts local variable", srcs: map[string]string{
			"a.ts": "function push(x: number) {}\nfunction f(arr: number[]) { arr.push(1) }\n",
		}, unwanted: [][3]string{c("f()", "push()")}},
		{name: "js other file", srcs: map[string]string{
			"lib.js": "export function format() {}\nexport function get() {}\n",
			"app.js": "function h(res) { res.format({}); axios.get() }\n",
		}, unwanted: [][3]string{c("h()", "format()"), c("h()", "get()")}},
		{name: "python attribute chain", srcs: map[string]string{
			"pkg/b.py": "def get():\n    pass\n",
			"a.py":     "class C:\n    def run(self):\n        self.session.get()\n",
		}, unwanted: [][3]string{c("C.run()", "get()")}},
		{name: "js this in unrelated class", srcs: map[string]string{
			"a.js": "class A { run() { this.m() } }\nclass B { m() {} }\n",
		}, unwanted: [][3]string{c("A.run()", "B.m()")}},
		{name: "python self in unrelated class", srcs: map[string]string{
			"a.py": "class A:\n    def run(self):\n        self.save()\nclass B:\n    def save(self):\n        pass\n",
		}, unwanted: [][3]string{c("A.run()", "B.save()")}},
		{name: "go corpus package", srcs: map[string]string{
			"util/join.go": "package util\nfunc Join() {}\n",
			"main.go":      "package main\nfunc main() { util.Join() }\n",
		}, want: [][3]string{c("main()", "Join()")}},
		{name: "go receiver", srcs: map[string]string{
			"a.go": "package a\ntype S struct{}\ntype T struct{}\nfunc (s *S) a() { s.b() }\nfunc (s *S) b() {}\nfunc (t *T) b() {}\n",
		}, want: [][3]string{c("S.a()", "S.b()")}, unwanted: [][3]string{c("S.a()", "T.b()")}},
		{name: "java super", srcs: map[string]string{
			"A.java": "class Base { void log() {} }\nclass A extends Base { void go() { super.log(); } }\n",
		}, want: [][3]string{c("A.go()", "Base.log()")}},
		{name: "python inherited self", srcs: map[string]string{
			"a.py": "class B:\n    def save(self):\n        pass\nclass A(B):\n    def run(self):\n        self.save()\n",
		}, want: [][3]string{c("A.run()", "B.save()")}},
		{name: "python unknown ancestor", srcs: map[string]string{
			"a.py": "class B:\n    def save(self):\n        pass\nclass A(Ext, B):\n    def run(self):\n        self.save()\n",
		}, unwanted: [][3]string{c("A.run()", "B.save()")}},
		{name: "lua self", srcs: map[string]string{
			"a.lua": "local A = {}\nlocal B = {}\nfunction A:step() end\nfunction B:step() end\nfunction A:run() self:step() end\n",
		}, want: [][3]string{c("A.run()", "A.step()")}, unwanted: [][3]string{c("A.run()", "B.step()")}},
		{name: "python typed parameter", srcs: map[string]string{
			"a.py": "class Client:\n    def fetch(self):\n        pass\nclass Other:\n    def fetch(self):\n        pass\ndef use(c: Client):\n    return c.fetch()\n",
		}, want: [][3]string{c("use()", "Client.fetch()")}, unwanted: [][3]string{c("use()", "Other.fetch()")}},
		{name: "python constructor binding", srcs: map[string]string{
			"a.py": "class Client:\n    def fetch(self):\n        pass\nclass Other:\n    def fetch(self):\n        pass\ndef use():\n    c = Client()\n    c.fetch()\n",
		}, want: [][3]string{c("use()", "Client.fetch()")}, unwanted: [][3]string{c("use()", "Other.fetch()")}},
		{name: "python rebound local", srcs: map[string]string{
			"a.py": "class Client:\n    def fetch(self):\n        pass\nclass Other:\n    def fetch(self):\n        pass\ndef use(x):\n    c = Client()\n    c = x\n    c.fetch()\n",
		}, unwanted: [][3]string{c("use()", "Client.fetch()"), c("use()", "Other.fetch()")}},
		{name: "go typed parameter", srcs: map[string]string{
			"a.go": "package a\ntype S struct{}\ntype T struct{}\nfunc (s *S) b() {}\nfunc (t *T) b() {}\nfunc use(x *S) { x.b() }\n",
		}, want: [][3]string{c("use()", "S.b()")}, unwanted: [][3]string{c("use()", "T.b()")}},
		{name: "go package-qualified parameter", srcs: map[string]string{
			"m/m.go": "package m\ntype G struct{}\ntype H struct{}\nfunc (g *G) Deg() {}\nfunc (h *H) Deg() {}\n",
			"a/a.go": "package a\nfunc use(g *m.G) { g.Deg() }\n",
		}, want: [][3]string{c("use()", "G.Deg()")}, unwanted: [][3]string{c("use()", "H.Deg()")}},
		{name: "go var declaration", srcs: map[string]string{
			"a.go": "package a\ntype S struct{}\ntype T struct{}\nfunc (s *S) b() {}\nfunc (t *T) b() {}\nfunc use() { var x S; x.b() }\n",
		}, want: [][3]string{c("use()", "S.b()")}, unwanted: [][3]string{c("use()", "T.b()")}},
		{name: "go same-file constructor", srcs: map[string]string{
			"a.go": "package a\ntype S struct{}\ntype T struct{}\nfunc (s *S) b() {}\nfunc (t *T) b() {}\nfunc newS() *S { return &S{} }\nfunc use() { x := newS(); x.b() }\n",
		}, want: [][3]string{c("use()", "S.b()")}, unwanted: [][3]string{c("use()", "T.b()")}},
		{name: "go composite literal", srcs: map[string]string{
			"a.go": "package a\ntype S struct{}\ntype T struct{}\nfunc (s *S) b() {}\nfunc (t T) b() {}\nfunc use() { x := &S{}; y := T{}; x.b(); y.b() }\n",
		}, want: [][3]string{c("use()", "S.b()"), c("use()", "T.b()")}},
		{name: "python for target shadows typed local", srcs: map[string]string{
			"a.py": "class Client:\n    def fetch(self):\n        pass\ndef looped(c: Client, items):\n    for c in items:\n        c.fetch()\n",
		}, unwanted: [][3]string{c("looped()", "Client.fetch()")}},
		{name: "python comprehension target shadows typed local", srcs: map[string]string{
			"a.py": "class Client:\n    def fetch(self):\n        pass\ndef comp(c: Client, items):\n    return [c.fetch() for c in items]\n",
		}, unwanted: [][3]string{c("comp()", "Client.fetch()")}},
		{name: "python with target shadows typed local", srcs: map[string]string{
			"a.py": "class Client:\n    def fetch(self):\n        pass\ndef withed(c: Client):\n    with opener() as c:\n        c.fetch()\n",
		}, unwanted: [][3]string{c("withed()", "Client.fetch()")}},
		{name: "python tuple target shadows typed local", srcs: map[string]string{
			"a.py": "class Client:\n    def fetch(self):\n        pass\ndef tupled(c: Client):\n    c, d = pair()\n    c.fetch()\n",
		}, unwanted: [][3]string{c("tupled()", "Client.fetch()")}},
		{name: "python walrus shadows typed local", srcs: map[string]string{
			"a.py": "class Client:\n    def fetch(self):\n        pass\ndef walrus(c: Client):\n    if (c := pick()):\n        c.fetch()\n",
		}, unwanted: [][3]string{c("walrus()", "Client.fetch()")}},
		{name: "python attribute assignment keeps typed local", srcs: map[string]string{
			"a.py": "class Client:\n    def fetch(self):\n        pass\nclass Other:\n    def fetch(self):\n        pass\ndef use(c: Client):\n    c.x, c.y = 1, 2\n    c.fetch()\n",
		}, want: [][3]string{c("use()", "Client.fetch()")}},
		{name: "go range shadows typed local", srcs: map[string]string{
			"a.go": "package a\ntype S struct{}\ntype T struct{}\nfunc (s *S) b() {}\nfunc (t *T) b() {}\nfunc ranged(s S, ts []T) { for _, s := range ts { s.b() } }\n",
		}, unwanted: [][3]string{c("ranged()", "S.b()")}},
		{name: "go var shadows typed local", srcs: map[string]string{
			"a.go": "package a\ntype S struct{}\ntype T struct{}\nfunc (s *S) b() {}\nfunc (t *T) b() {}\nfunc shadowVar(s S) { { var s T; s.b() } }\n",
		}, unwanted: [][3]string{c("shadowVar()", "S.b()")}},
		{name: "go type switch shadows typed local", srcs: map[string]string{
			"a.go": "package a\ntype S struct{}\ntype T struct{}\nfunc (s *S) b() {}\nfunc (t *T) b() {}\nfunc tswitch(s *S, v interface{}) { switch s := v.(type) { case T: s.b() } }\n",
		}, unwanted: [][3]string{c("tswitch()", "S.b()")}},
		{name: "ruby bare self-send stays in its class", srcs: map[string]string{
			"a.rb": "class A\n  def helper\n  end\nend\nclass B\n  def run\n    helper\n  end\nend\n",
		}, unwanted: [][3]string{c("B.run()", "A.helper()")}},
		{name: "ruby bare self-send picks its own class", srcs: map[string]string{
			"a.rb": "class A\n  def dup_name\n  end\nend\nclass B\n  def dup_name\n  end\n  def run\n    dup_name\n  end\nend\n",
		}, want: [][3]string{c("B.run()", "B.dup_name()")}, unwanted: [][3]string{c("B.run()", "A.dup_name()")}},
		{name: "ruby bare self-send inherited", srcs: map[string]string{
			"a.rb": "class A\n  def helper\n  end\nend\nclass B < A\n  def run\n    helper\n  end\nend\n",
		}, want: [][3]string{c("B.run()", "A.helper()")}},
		{name: "ruby bare call from a top-level method", srcs: map[string]string{
			"a.rb": "def helper\nend\ndef run\n  helper\nend\n",
		}, want: [][3]string{c("run()", "helper()")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantEdges(t, tc.srcs, tc.want, tc.unwanted...)
		})
	}
}

// TestSelfTargetCyclicInherits checks that a cyclic inherits graph is walked
// once per class instead of fanning out at every level.
func TestSelfTargetCyclicInherits(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e", "f"}
	supers := map[string][]string{}
	for _, c := range ids {
		for _, o := range ids {
			if o != c {
				supers[c] = append(supers[c], o)
			}
		}
	}
	done := make(chan string, 1)
	go func() { done <- selfTarget("a", "missing", false, nil, supers, nil) }()
	select {
	case got := <-done:
		if got != "" {
			t.Errorf("selfTarget = %q, want no target", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("selfTarget did not finish on a 6-class inherits cycle")
	}
}

// TestResolveImportEdgesAcrossBatches checks the import edges of a corpus that
// spans several target batches and several edge parts: order, and a TypeOnly
// that a later value import of the same target clears in an earlier part.
func TestResolveImportEdgesAcrossBatches(t *testing.T) {
	const n = 3*impPart/2 + 7
	files := []string{"t.ts"}
	results := []Result{{}}
	type imp struct {
		src, tgt, rel string
		typeOnly      bool
	}
	var want []imp
	for i := range n {
		f, id := fmt.Sprintf("f%d.ts", i), fmt.Sprintf("f%d", i)
		files = append(files, f)
		results = append(results, Result{Imps: []Imp{
			{FileID: id, File: f, Spec: "./t", TypeOnly: true},
			{FileID: id, File: f, Spec: "lib"},
		}})
		// Every third file imports the target again, as a value.
		again := i%3 == 0
		want = append(want, imp{id, "t_ts", "imports_from", !again}, imp{id, "lib", "imports", false})
		if again {
			results = append(results, Result{Imps: []Imp{{FileID: id, File: f, Spec: "./t"}}})
			files = append(files, fmt.Sprintf("g%d.ts", i))
		}
	}
	var got []imp
	for _, e := range Resolve(results, files).Edges {
		got = append(got, imp{e.Source, e.Target, e.Relation.String(), e.TypeOnly()})
	}
	if !slices.Equal(got, want) {
		t.Fatalf("import edges differ: got %d, want %d", len(got), len(want))
	}
}

// TestResolveCallEdgesAcrossParts checks the call edges of a run that fills
// more than one part: no part above impPart, and the order of the calls.
func TestResolveCallEdgesAcrossParts(t *testing.T) {
	const n = 3*impPart/2 + 7
	r := Result{Defs: []Def{{ID: "helper", Name: "helper", File: "a.go"}}, File: "a.go", CallText: []string{"", "helper"}}
	var want []string
	for i := range n {
		id := fmt.Sprintf("c%d", i)
		r.CallText = append(r.CallText, id)
		r.Calls = append(r.Calls, Call{CallerID: uint32(i + 2), Callee: 1, Loc: model.Line(1)})
		want = append(want, id)
	}
	_, parts := ResolveParts([]Result{r}, []string{"a.go"})
	var got []string
	for _, p := range parts {
		for _, e := range p {
			if e.Relation.String() != "calls" {
				continue
			}
			if len(p) > impPart {
				t.Fatalf("part of %d call edges, want at most %d", len(p), impPart)
			}
			got = append(got, e.Source)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("call edges differ: got %d, want %d", len(got), len(want))
	}
}

// TestResolveImportedSetPerFile: the set of imported files is made per caller
// file inside one run, so a file that imports nothing must not bind an
// ambiguous name through what the file before it imports.
func TestResolveImportedSetPerFile(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1)) // one run for all files
	files := []string{"p/x.ts", "q/y.ts", "r/a.ts", "s/b.ts"}
	results := []Result{
		{Defs: []Def{{ID: "x_f", Name: "f", File: "p/x.ts"}}},
		{Defs: []Def{{ID: "y_f", Name: "f", File: "q/y.ts"}}},
		{
			Imps:  []Imp{{FileID: "a", File: "r/a.ts", Spec: "../p/x"}},
			Calls: []Call{{CallerID: 1, Callee: 2, Loc: model.Line(2)}}, CallText: []string{"", "a_main", "f"}, File: "r/a.ts",
		},
		{Calls: []Call{{CallerID: 1, Callee: 2, Loc: model.Line(2)}}, CallText: []string{"", "b_main", "f"}, File: "s/b.ts"},
	}
	var got []string
	for _, e := range Resolve(results, files).Edges {
		if e.Relation.String() == "calls" {
			got = append(got, e.Source+">"+e.Target)
		}
	}
	if want := []string{"a_main>x_f"}; !slices.Equal(got, want) {
		t.Fatalf("call edges = %v, want %v", got, want)
	}
}

// TestResolveImportEdgesDedupByTargetID: two files whose paths make the same
// id are one import target for the dedupe.
func TestResolveImportEdgesDedupByTargetID(t *testing.T) {
	files := []string{"a-b.ts", "a_b.ts", "m.ts"}
	if idutil.MakeID(files[0]) != idutil.MakeID(files[1]) {
		t.Fatal("the two paths no longer make one id")
	}
	results := []Result{{}, {}, {Imps: []Imp{
		{FileID: "m", File: "m.ts", Spec: "./a-b", TypeOnly: true},
		{FileID: "m", File: "m.ts", Spec: "./a_b"},
	}}}
	var got []string
	for _, e := range Resolve(results, files).Edges {
		got = append(got, fmt.Sprintf("%s>%s %s %v", e.Source, e.Target, e.Relation, e.TypeOnly()))
	}
	if want := []string{"m>a_b_ts imports_from false"}; !slices.Equal(got, want) {
		t.Fatalf("import edges = %v, want %v", got, want)
	}
}

// TestResolveMarkdownEdgesAfterModuleRefs pins the order of the tail edges:
// the markdown pass runs beside the calls, and its edges still come last.
func TestResolveMarkdownEdgesAfterModuleRefs(t *testing.T) {
	results := []Result{
		{ModRefs: []ModRef{{FromID: "M", File: "main.tf", Source: "acme/vpc/aws"}}},
		{MDRefs: []MDRef{{FromID: "D", File: "a.md", Target: "b.md"}}},
	}
	var got []string
	for _, e := range Resolve(results, []string{"main.tf", "a.md", "b.md"}).Edges {
		got = append(got, e.Source)
	}
	if !slices.Equal(got, []string{"M", "D"}) {
		t.Errorf("edge sources %v, want [M D]", got)
	}
}

// TestResolveEdgeFiles checks that an edge Resolve makes names the file and
// line of the record it came from, across files.
func TestResolveEdgeFiles(t *testing.T) {
	results := []Result{
		{Defs: []Def{{ID: "U", Name: "helper", File: "util.js"}}},
		{
			Defs:  []Def{{ID: "A", Name: "a", File: "a.js"}},
			Calls: []Call{{CallerID: 1, Callee: 2, Loc: model.Line(5)}}, CallText: []string{"", "A", "helper"}, File: "a.js",
			Imps: []Imp{{FileID: "afile", File: "a.js", Spec: "./util", Loc: model.Line(1)}},
		},
		{
			Defs:  []Def{{ID: "B", Name: "b", File: "b.js"}},
			Calls: []Call{{CallerID: 1, Callee: 2, Loc: model.Line(7)}}, CallText: []string{"", "B", "helper"}, File: "b.js",
			Imps: []Imp{{FileID: "bfile", File: "b.js", Spec: "./util", Loc: model.Line(2)}},
		},
	}
	got := map[string]string{}
	for _, e := range Resolve(results, []string{"util.js", "a.js", "b.js"}).Edges {
		got[e.Source+" "+e.Relation.String()] = e.SourceFile.String() + ":" + e.SourceLocation.String()
	}
	want := map[string]string{"A calls": "a.js:L5", "B calls": "b.js:L7", "afile imports_from": "a.js:L1", "bfile imports_from": "b.js:L2"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}
