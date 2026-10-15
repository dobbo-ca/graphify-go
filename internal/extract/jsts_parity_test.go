package extract

import (
	"slices"
	"testing"
)

func wantGraph(t *testing.T, nodes map[[2]string]bool, edges map[[3]string]bool, wn [][2]string, we, no [][3]string) {
	t.Helper()
	for _, n := range wn {
		if !nodes[n] {
			t.Errorf("missing node %v", n)
		}
	}
	for _, e := range we {
		if !edges[e] {
			t.Errorf("missing edge %v", e)
		}
	}
	for _, e := range no {
		if edges[e] {
			t.Errorf("unexpected edge %v", e)
		}
	}
}

// Nested declarations are nodes under their parent and own their calls.
func TestJSNestedFunctions(t *testing.T) {
	src := `function helper() {}
function outer() {
  function inner() { helper(); }
  inner();
  const cb = () => { function deep() { helper(); } deep(); };
  after();
}
function after() {}
const arrow = () => { function inArrow() { helper(); } inArrow(); };
class K { m() { function inMethod() { helper(); } inMethod(); } }
`
	nodes, edges := resolveSrc(t, [][2]string{{"n.ts", src}})
	wantGraph(t, nodes, edges,
		[][2]string{{"n.ts", "inner()"}, {"n.ts", "deep()"}, {"n.ts", "inArrow()"}, {"n.ts", "inMethod()"}},
		[][3]string{
			{"n.ts:outer()", "contains", "n.ts:inner()"},
			{"n.ts:outer()", "contains", "n.ts:deep()"},
			{"n.ts:arrow()", "contains", "n.ts:inArrow()"},
			{"n.ts:K.m()", "contains", "n.ts:inMethod()"},
			{"n.ts:inner()", "calls", "n.ts:helper()"},
			{"n.ts:deep()", "calls", "n.ts:helper()"},
			{"n.ts:inArrow()", "calls", "n.ts:helper()"},
			{"n.ts:inMethod()", "calls", "n.ts:helper()"},
			{"n.ts:outer()", "calls", "n.ts:inner()"},
			{"n.ts:outer()", "calls", "n.ts:after()"}, // after inner ends
		},
		[][3]string{
			{"n.ts:outer()", "calls", "n.ts:helper()"},
			{"n.ts:arrow()", "calls", "n.ts:helper()"},
			{"n.ts:K.m()", "calls", "n.ts:helper()"},
		})
}

// Declarations behind a wrapper are file-level; closure calls go to the
// nearest tracked body, or the file for a top-level callback.
func TestJSWrappedDeclarations(t *testing.T) {
	src := `function helper() {}
(function () { function inIife() { helper(); } })();
if (x) { function inIf() {} }
describe("x", () => { helper(); function inCb() { helper(); } class InCls {} });
function f() { [1].map(() => helper()); }
`
	nodes, edges := resolveSrc(t, [][2]string{{"w.ts", src}})
	wantGraph(t, nodes, edges,
		[][2]string{{"w.ts", "inIife()"}, {"w.ts", "inIf()"}, {"w.ts", "inCb()"}, {"w.ts", "InCls"}},
		[][3]string{
			{"w.ts:w.ts", "contains", "w.ts:inIife()"},
			{"w.ts:w.ts", "contains", "w.ts:inCb()"},
			{"w.ts:w.ts", "contains", "w.ts:InCls"},
			{"w.ts:inIife()", "calls", "w.ts:helper()"},
			{"w.ts:inCb()", "calls", "w.ts:helper()"},
			{"w.ts:f()", "calls", "w.ts:helper()"},
		}, [][3]string{{"w.ts:w.ts", "calls", "w.ts:helper()"}})
}

// A nested function binds only from inside its parent.
func TestJSNestedScope(t *testing.T) {
	a := `export function top() {
  function only() {}
  function caller() { only(); }
}
function user() { only(); }
`
	b := "function far() { only(); }\n"
	_, edges := resolveSrc(t, [][2]string{{"a.ts", a}, {"b.ts", b}})
	wantGraph(t, nil, edges, nil, [][3]string{
		{"a.ts:caller()", "calls", "a.ts:only()"},
	}, [][3]string{
		{"b.ts:far()", "calls", "a.ts:only()"},
		{"a.ts:user()", "calls", "a.ts:only()"},
	})
}

// A declaration below module level makes no node; its calls go to the
// enclosing body, or the file for a top-level callback.
func TestJSLocalDeclarationsNoNode(t *testing.T) {
	src := `function helper() {}
(function () { var inner = function () { helper(); }; })();
if (x) { const inIf = () => { helper(); }; }
describe("x", () => { const lh = () => {}; });
`
	nodes, edges := resolveSrc(t, [][2]string{{"l.ts", src}})
	for _, l := range []string{"inner()", "inIf()", "lh()"} {
		if nodes[[2]string{"l.ts", l}] {
			t.Errorf("unexpected node %s", l)
		}
	}
	wantGraph(t, nodes, edges, nil, nil, [][3]string{{"l.ts:l.ts", "calls", "l.ts:helper()"}})
}

// Only a closure in a call statement makes the file its caller.
func TestJSFileCallerShapes(t *testing.T) {
	src := `function a() {} function b() {} function c() {} function d() {} function e() {}
late = () => { a(); };
export default () => { b(); };
!function () { c(); }();
x.y = { z: () => d() };
ok && foo(() => e());
`
	_, edges := resolveSrc(t, [][2]string{{"c.ts", src}})
	var no [][3]string
	for _, f := range []string{"a", "b", "c", "d", "e"} {
		no = append(no, [3]string{"c.ts:c.ts", "calls", "c.ts:" + f + "()"})
	}
	wantGraph(t, nil, edges, nil, nil, no)
}

// A const initialised by a call, array or object makes no declaration
// nodes; a ternary or var initializer still does.
func TestJSConstInitializerNoRecursion(t *testing.T) {
	src := `const w1 = wrap(() => { function g1() {} });
const w3 = [function () { function g3() {} }];
export const w8 = wrap(() => { function g10() {} });
const w2 = cond ? function () { function g2() {} } : null;
var w6 = wrap(function () { function g7() {} });
`
	nodes, _ := resolveSrc(t, [][2]string{{"k.ts", src}})
	for _, l := range []string{"g1()", "g3()", "g10()"} {
		if nodes[[2]string{"k.ts", l}] {
			t.Errorf("unexpected node %s", l)
		}
	}
	wantGraph(t, nodes, nil, [][2]string{{"k.ts", "g2()"}, {"k.ts", "g7()"}}, nil, nil)
}

// A module-level const or let that is no function: a node for an object,
// array, cast, call or new value, or for any exported plain name.
func TestJSTopLevelConstNodes(t *testing.T) {
	src := `export const LIMIT = 3;
const plain = 5;
const obj = { a: 1 };
const arr = [1];
const made = make();
const inst = new Map();
let counter = 0;
let lobj = {};
var legacy = {};
export let x1 = "s", x2 = 2;
export const { p, q: r } = make();
const cast = {} as Foo;
const sat = {} satisfies Foo;
const noinit: number;
export const { d1 } = other;
function f() { const local = {}; }
`
	nodes, edges := resolveSrc(t, [][2]string{{"v.ts", src}})
	var wn [][2]string
	var we [][3]string
	for _, l := range []string{"LIMIT", "obj", "arr", "made", "inst", "lobj", "x1", "x2", "p", "q", "cast", "sat"} {
		wn = append(wn, [2]string{"v.ts", l})
		we = append(we, [3]string{"v.ts:v.ts", "contains", "v.ts:" + l})
	}
	for _, l := range []string{"plain", "counter", "legacy", "noinit", "r", "d1", "local"} {
		if nodes[[2]string{"v.ts", l}] {
			t.Errorf("unexpected node %s", l)
		}
	}
	wantGraph(t, nodes, edges, wn, we, nil)
}

// A closure in a call initializer is called by the const; an object
// literal's methods are not tracked.
func TestJSConstClosureCalls(t *testing.T) {
	src := `function h1() {} function h2() {} function h3() {} function h4() {}
const ex = wrap(() => { h1(); function g() { h2(); } });
const sat = wrap(() => h3()) satisfies X;
const lit = { a: () => h4() };
`
	_, edges := resolveSrc(t, [][2]string{{"w.ts", src}})
	wantGraph(t, nil, edges, nil,
		[][3]string{{"w.ts:ex", "calls", "w.ts:h1()"}, {"w.ts:sat", "calls", "w.ts:h3()"}},
		[][3]string{{"w.ts:lit", "calls", "w.ts:h4()"}, {"w.ts:w.ts", "calls", "w.ts:h1()"}})
}

// A const sharing a callee's name must not hide the call.
func TestJSConstKeepsCallBinding(t *testing.T) {
	_, edges := resolveSrc(t, [][2]string{
		{"a.ts", "export function trace() {}"},
		{"ns/ts.ts", "export * from \"../a\";"},
		{"b.ts", "import { trace } from \"./ns/ts\";\nexport function run() { trace(); }"},
		{"c.ts", "let trace: any[] = [];"},
	})
	wantGraph(t, nil, edges, nil, [][3]string{{"b.ts:run()", "calls", "a.ts:trace()"}}, nil)
}

// A generator function expression is a function, not a const.
func TestJSExportedGeneratorConst(t *testing.T) {
	src := "function p1() {}\nexport const E = function* () { p1(); };\nexport const A = async function* () { p1(); };\n"
	nodes, edges := resolveSrc(t, [][2]string{{"g.ts", src}})
	wantGraph(t, nodes, edges, [][2]string{{"g.ts", "E()"}, {"g.ts", "A()"}}, [][3]string{
		{"g.ts:E()", "calls", "g.ts:p1()"}, {"g.ts:A()", "calls", "g.ts:p1()"}}, nil)
	if nodes[[2]string{"g.ts", "E"}] || nodes[[2]string{"g.ts", "A"}] {
		t.Error("generator const made a bare node")
	}
}

// Prose naming a const does not link to it.
func TestJSConstNoMarkdownReference(t *testing.T) {
	_, edges := resolveSrc(t, [][2]string{
		{"a.ts", "export const SETTINGS = { a: 1 };\nexport function realFn() {}"},
		{"README.md", "# Doc\n\nUse `SETTINGS` with `realFn`.\n"},
	})
	wantGraph(t, nil, edges, nil, [][3]string{{"README.md:README.md", "references", "a.ts:realFn()"}},
		[][3]string{{"README.md:README.md", "references", "a.ts:SETTINGS"}})
}

// Namespaces and modules are file-contained nodes; members carry the
// namespace in their id, so equal names in two namespaces both survive.
func TestTSNamespaces(t *testing.T) {
	src := `namespace NS {
  export class Inner { im() {} }
  export function nf() {}
  namespace Deep { export function df() { nf(); } }
}
namespace Other { export function nf() {} }
module Old { export enum E { A } }
declare module "pkg" { export function pf(): void; }
declare class Amb { am(): void; }
declare function afn(): void;
declare const ac: number;
declare namespace DN { function dnf(): void; }
declare global { interface Window { w(): void } }
export declare function edf(): void;
namespace A.B { export function ab() {} }
`
	nodes, edges := resolveSrc(t, [][2]string{{"ns.ts", src}})
	wantGraph(t, nodes, edges,
		[][2]string{{"ns.ts", "NS"}, {"ns.ts", "Deep"}, {"ns.ts", "Other"}, {"ns.ts", "Old"}, {"ns.ts", "pkg"},
			{"ns.ts", "DN"}, {"ns.ts", "A.B"}, {"ns.ts", "Inner"}, {"ns.ts", "E"}, {"ns.ts", "Amb"},
			{"ns.ts", "Window"}, {"ns.ts", "ab()"}, {"ns.ts", "df()"}},
		[][3]string{
			{"ns.ts:ns.ts", "contains", "ns.ts:NS"},
			{"ns.ts:ns.ts", "contains", "ns.ts:Deep"},
			{"ns.ts:ns.ts", "contains", "ns.ts:A.B"},
			{"ns.ts:ns.ts", "contains", "ns.ts:Inner"},
			{"ns.ts:ns.ts", "contains", "ns.ts:Amb"},
		}, nil)
	n := 0
	for _, nd := range FileFromBytes("ns.ts", []byte(src)).Nodes {
		if nd.Label == "nf()" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("nf() nodes = %d, want 2", n)
	}
	for _, bad := range []string{"afn()", "pf()", "dnf()", "edf()", "ac", "global"} {
		if nodes[[2]string{"ns.ts", bad}] {
			t.Errorf("unexpected node %s", bad)
		}
	}
}

// callTargets lists "file:line" of what the caller label calls.
func callTargets(t *testing.T, srcs [][2]string, file, caller string) map[string]bool {
	t.Helper()
	var results []Result
	var files []string
	for _, s := range srcs {
		results = append(results, FileFromBytes(s[0], []byte(s[1])))
		files = append(files, s[0])
	}
	ext := Resolve(results, files)
	byID := map[string]string{}
	var from []string
	for _, n := range ext.Nodes {
		byID[n.ID] = n.SourceFile + ":" + n.SourceLocation
		if n.SourceFile == file && n.Label == caller {
			from = append(from, n.ID)
		}
	}
	out := map[string]bool{}
	for _, e := range ext.Edges {
		if e.Relation.String() == "calls" && slices.Contains(from, e.Source) {
			out[byID[e.Target]] = true
		}
	}
	return out
}

func wantCalls(t *testing.T, srcs [][2]string, file, caller string, want ...string) {
	t.Helper()
	got := callTargets(t, srcs, file, caller)
	if len(got) != len(want) {
		t.Errorf("%s calls %v, want %v", caller, got, want)
		return
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("%s calls %v, want %v", caller, got, want)
		}
	}
}

// A call binds to the innermost namespace that has the name.
func TestTSNamespaceLexicalCalls(t *testing.T) {
	a := "export function mk() {}\nnamespace P { export function mk() {} export function other() { mk(); } }\nexport function local() { mk(); }\n"
	b := "import { mk } from \"./a\";\nexport function u() { mk(); }\n"
	srcs := [][2]string{{"a.ts", a}, {"b.ts", b}}
	wantCalls(t, srcs, "a.ts", "local()", "a.ts:L1")
	wantCalls(t, srcs, "a.ts", "other()", "a.ts:L2")
	wantCalls(t, srcs, "b.ts", "u()", "a.ts:L1")

	c := `namespace N {
  export function a() { b(); }
  export function b() {}
  export namespace In { export function c() { b(); d(); } function d() {} }
}
namespace O {
  export function b() {}
  export function z() { b(); }
}
function b() {}
function topcall() { b(); }
`
	srcs = [][2]string{{"c.ts", c}}
	wantCalls(t, srcs, "c.ts", "a()", "c.ts:L3")
	wantCalls(t, srcs, "c.ts", "c()", "c.ts:L3", "c.ts:L4")
	wantCalls(t, srcs, "c.ts", "z()", "c.ts:L7")
	wantCalls(t, srcs, "c.ts", "topcall()", "c.ts:L10")

	d := "namespace NS {\n  export function nf() {}\n  namespace Deep { export function df() { nf(); } }\n}\nnamespace Other { export function nf() {} }\n"
	wantCalls(t, [][2]string{{"d.ts", d}}, "d.ts", "df()", "d.ts:L2")
}

// A namespace named `_` or `$` still has its members.
func TestTSDegenerateNamespaceMembers(t *testing.T) {
	src := "declare namespace _ { interface Coll {} class Kept {} }\nnamespace $ { export function d() {} }\nnamespace Outer { namespace _ { export class Hidden {} } }\n"
	nodes, _ := resolveSrc(t, [][2]string{{"o.ts", src}})
	for _, l := range []string{"Coll", "Kept", "d()", "Hidden"} {
		if !nodes[[2]string{"o.ts", l}] {
			t.Errorf("missing node %s", l)
		}
	}
}

// A namespace member never takes the id of a class member or top-level name.
func TestTSNamespaceMemberIDs(t *testing.T) {
	src := `class Foo { create() {} }
namespace Foo { export function create() {} }
function Bar() { function helper() {} }
namespace Bar { export function helper() {} }
namespace A_B { export function u() {} }
function A_B_u() {}
`
	var got = map[string]int{}
	for _, n := range FileFromBytes("m.ts", []byte(src)).Nodes {
		got[n.Label]++
	}
	for l, c := range map[string]int{"create()": 1, "Foo.create()": 1, "helper()": 2, "u()": 1, "A_B_u()": 1} {
		if got[l] != c {
			t.Errorf("nodes %q = %d, want %d", l, got[l], c)
		}
	}
	_, edges := resolveSrc(t, [][2]string{{"m.ts", src}})
	if edges[[3]string{"m.ts:m.ts", "contains", "m.ts:Foo.create()"}] {
		t.Error("file contains the method Foo.create()")
	}
}

// Exported variables of a namespace or ambient module make an orphan node.
func TestTSNamespaceVariableNodes(t *testing.T) {
	src := `namespace M {
  export const arrow = () => {};
  export const obj = { a: 1 };
  export let arr = [1];
  const priv = { b: 2 };
}
declare module "m1" { export const k1: number; export let k2: string; }
declare namespace G { const k3: number; export const k4: { a: 1 }; }
`
	nodes, edges := resolveSrc(t, [][2]string{{"v.ts", src}})
	for _, l := range []string{"arrow", "obj", "arr", "k1", "k2", "k4"} {
		if !nodes[[2]string{"v.ts", l}] {
			t.Errorf("missing node %s", l)
		}
	}
	for _, l := range []string{"priv", "k3", "arrow()"} {
		if nodes[[2]string{"v.ts", l}] {
			t.Errorf("unexpected node %s", l)
		}
	}
	if edges[[3]string{"v.ts:v.ts", "contains", "v.ts:k1"}] {
		t.Error("k1 is an orphan node upstream")
	}
}

// A namespace const is no call target; functions and classes are.
func TestTSNamespaceCallTargets(t *testing.T) {
	src := `namespace T {
  export const cfn = () => {};
  export function useC() { cfn(); }
  export class Cls {}
  export function useK() { Cls(); }
  export function fn() {}
  export function useF() { fn(); }
}
`
	_, edges := resolveSrc(t, [][2]string{{"t.ts", src}})
	wantGraph(t, nil, edges, nil, [][3]string{
		{"t.ts:useK()", "calls", "t.ts:Cls"}, {"t.ts:useF()", "calls", "t.ts:fn()"},
	}, [][3]string{
		{"t.ts:useC()", "calls", "t.ts:cfn"}, {"t.ts:t.ts", "contains", "t.ts:cfn"},
	})
}

// A method body is a boundary: its calls go nowhere, also in a closure
// the file or a const owns.
func TestJSMethodInClosureDropsCalls(t *testing.T) {
	src := `function h01() {} function h08() {} function h1() {} function h2() {}
foo(() => { return { m01() { h01(); } }; });
foo(() => { const K = class { m08() { h08(); } }; });
const c1 = foo(() => { return { m() { h1(); } }; });
const c2 = foo(class { m() { h2(); } });
`
	_, edges := resolveSrc(t, [][2]string{{"m.ts", src}})
	var no [][3]string
	for _, h := range []string{"h01", "h08", "h1", "h2"} {
		no = append(no, [3]string{"m.ts:m.ts", "calls", "m.ts:" + h + "()"}, [3]string{"m.ts:c1", "calls", "m.ts:" + h + "()"},
			[3]string{"m.ts:c2", "calls", "m.ts:" + h + "()"})
	}
	wantGraph(t, nil, edges, nil, nil, no)
}

// A file-level callback or IIFE calling a function of the file adds no
// edge: upstream's contains edge hides it.
func TestJSFileClosureCallsHidden(t *testing.T) {
	src := `function helper() {}
describe("x", () => { helper(); });
[1].forEach(() => helper());
(function () { helper(); })();
p.then(() => helper());
`
	_, edges := resolveSrc(t, [][2]string{{"f.ts", src}})
	wantGraph(t, nil, edges, nil, nil, [][3]string{{"f.ts:f.ts", "calls", "f.ts:helper()"}})
}

// A member-assigned function has no nested nodes; calls in its inner
// declarations are its own.
func TestJSMemberAssignNoNested(t *testing.T) {
	src := `function helper() {}
exports.run = function () { function inRun() { helper(); } inRun(); };
module.exports.go = () => { function inGo() { helper(); } };
Ctor.prototype.m = function () { function inM() { helper(); } };
exports.gen = async function* () { helper(); };
`
	nodes, edges := resolveSrc(t, [][2]string{{"e.ts", src}})
	for _, l := range []string{"inRun()", "inGo()", "inM()"} {
		if nodes[[2]string{"e.ts", l}] {
			t.Errorf("unexpected node %s", l)
		}
	}
	wantGraph(t, nodes, edges, nil, [][3]string{
		{"e.ts:run()", "calls", "e.ts:helper()"}, {"e.ts:go()", "calls", "e.ts:helper()"},
		{"e.ts:m()", "calls", "e.ts:helper()"}, {"e.ts:gen()", "calls", "e.ts:helper()"},
	}, nil)
}

// A module-level const is a call target, in its file and through an import.
func TestJSConstCallTargets(t *testing.T) {
	a := "export const eObj = {};\nexport const eCall = make();\nconst loc = new Map();\nfunction f() { loc(); eObj(); }\n"
	b := "import { eCall } from \"./a\";\nfunction g() { eCall(); }\n"
	_, edges := resolveSrc(t, [][2]string{{"a.ts", a}, {"b.ts", b}})
	wantGraph(t, nil, edges, nil, [][3]string{
		{"a.ts:f()", "calls", "a.ts:loc"}, {"a.ts:f()", "calls", "a.ts:eObj"}, {"b.ts:g()", "calls", "a.ts:eCall"},
	}, nil)
}

// A static block holds file-level declarations; an exported destructure
// makes a node per key.
func TestJSStaticBlockAndPatternKeys(t *testing.T) {
	src := `function helper() {}
class S { static { function inStatic() { helper(); } inStatic(); } }
export const { a, b: c, d: { e }, f = 1, g: h = 2, ...rest } = x();
`
	nodes, edges := resolveSrc(t, [][2]string{{"s.ts", src}})
	wantGraph(t, nodes, edges, [][2]string{{"s.ts", "inStatic()"}, {"s.ts", "a"}, {"s.ts", "b"}, {"s.ts", "d"}, {"s.ts", "g"}},
		[][3]string{{"s.ts:inStatic()", "calls", "s.ts:helper()"}}, nil)
	for _, l := range []string{"c", "e", "f", "h", "rest"} {
		if nodes[[2]string{"s.ts", l}] {
			t.Errorf("unexpected node %s", l)
		}
	}
}

// A namespace member without `export` is no call target outside it.
func TestTSNamespacePrivateNotGlobal(t *testing.T) {
	p := "namespace P {\n  function hidden() {}\n  export function shown() {}\n  export function parse() { hidden(); }\n}\n"
	u := "export function user() { hidden(); }\n"
	v := "export function viewer() { shown(); }\n"
	srcs := [][2]string{{"p.ts", p}, {"u.ts", u}, {"v.ts", v}}
	wantCalls(t, srcs, "u.ts", "user()")
	wantCalls(t, srcs, "v.ts", "viewer()", "p.ts:L3")
	wantCalls(t, srcs, "p.ts", "parse()", "p.ts:L2")
}

// A bare call in JS/TS never names a class method.
func TestJSBareCallSkipsMethods(t *testing.T) {
	srcs := [][2]string{
		{"k.ts", "export class K { some() {} only() {} }\n"},
		{"f.ts", "export function some() {}\n"},
		{"c.ts", "export function u() { some(); only(); }\n"},
		{"d.ts", "export class D { run() { this.only(); } only() {} }\n"},
	}
	wantCalls(t, srcs, "c.ts", "u()", "f.ts:L1")
	wantCalls(t, srcs, "d.ts", "D.run()", "d.ts:L1")
}

// An exported variable without a node of its own still gets one, edgeless.
func TestJSExportedVariableOrphans(t *testing.T) {
	src := `export const c1: number;
export let l1: string;
export var v1 = 1;
export var v2;
export const { d1, d2: d3 } = obj;
export const [e1, e2] = arr;
export let { f1 } = obj;
export var { g1 } = obj;
export const { m1 } = a.b;
export const h1 = 5;
`
	nodes, edges := resolveSrc(t, [][2]string{{"a.ts", src}})
	wantGraph(t, nodes, edges,
		[][2]string{{"a.ts", "c1"}, {"a.ts", "l1"}, {"a.ts", "v1"}, {"a.ts", "v2"},
			{"a.ts", "{ d1, d2: d3 }"}, {"a.ts", "[e1, e2]"}, {"a.ts", "{ f1 }"}, {"a.ts", "h1"}},
		[][3]string{{"a.ts:a.ts", "contains", "a.ts:h1"}},
		[][3]string{{"a.ts:a.ts", "contains", "a.ts:c1"}, {"a.ts:a.ts", "contains", "a.ts:v1"}})
	for _, bad := range []string{"{ g1 }", "g1", "{ m1 }", "m1"} {
		if nodes[[2]string{"a.ts", bad}] {
			t.Errorf("unexpected node %s", bad)
		}
	}
	// under an ERROR node
	nodes, _ = resolveSrc(t, [][2]string{{"b.js", "function wrap() {\n    export const bad = 1;\n}\n"}})
	if !nodes[[2]string{"b.js", "bad"}] {
		t.Errorf("missing node bad under error")
	}
}

// Member assignments below module level and `var v = function*` make no node.
func TestJSMemberAssignModuleLevelOnly(t *testing.T) {
	src := `function t() {}
o.top = function () {};
if (c) { o.nested = function () {}; }
(function () { o.iife = function () {}; })();
class K { static { o.stat = function () {}; } }
var v = function* () { t(); };
var w = function () {};
`
	nodes, _ := resolveSrc(t, [][2]string{{"a.js", src}})
	for _, l := range []string{"top()", "w()"} {
		if !nodes[[2]string{"a.js", l}] {
			t.Errorf("missing node %s", l)
		}
	}
	for _, l := range []string{"nested()", "iife()", "stat()", "v()"} {
		if nodes[[2]string{"a.js", l}] {
			t.Errorf("unexpected node %s", l)
		}
	}
}

// Heritage binds a same-file or named-import base, never a guess.
func TestJSHeritage(t *testing.T) {
	m := `export class Base {}
export class Solo {}
export class Nm {}
export function Fn() {}
export default class Dm {}
`
	use := `import { Base as B2, Fn, Nm } from "./m";
import D from "./m";
import * as ns from "./m";
class NotImported extends Solo {}
class Aliased extends B2 {}
class Amb extends Base {}
class OfFn extends Fn {}
class ViaDef extends D {}
class Q extends ns.Nm {}
class G extends Nm<string> {}
class Call extends mix(B2) {}
class Own extends Later implements Iface, ns.X {}
class Later {}
interface Iface extends Later {}
interface J extends B2, Unknown {}
`
	other := "export class Base {}\n"
	nodes, edges := resolveSrc(t, [][2]string{{"m.ts", m}, {"use.ts", use}, {"other.ts", other}})
	wantGraph(t, nodes, edges, nil,
		[][3]string{
			{"use.ts:Aliased", "inherits", "m.ts:Base"},
			{"use.ts:ViaDef", "inherits", "m.ts:Dm"},
			{"use.ts:G", "inherits", "m.ts:Nm"},
			{"use.ts:Own", "inherits", "use.ts:Later"},
			{"use.ts:Own", "implements", "use.ts:Iface"},
			{"use.ts:Iface", "inherits", "use.ts:Later"},
			{"use.ts:J", "inherits", "m.ts:Base"},
		},
		[][3]string{
			{"use.ts:NotImported", "inherits", "m.ts:Solo"},
			{"use.ts:Amb", "inherits", "m.ts:Base"},
			{"use.ts:Amb", "inherits", "other.ts:Base"},
			{"use.ts:OfFn", "inherits", "m.ts:Fn()"},
			{"use.ts:Q", "inherits", "m.ts:Nm"},
			{"use.ts:Call", "inherits", "m.ts:Base"},
		})
	nodes, edges = resolveSrc(t, [][2]string{{"a.js", "class B {}\nclass C extends B {}\nclass D extends x.B {}\nconst lib = {};\nclass F extends lib.Old {}\nclass Q extends lib(B) {}\n"}})
	wantGraph(t, nodes, edges, nil, [][3]string{{"a.js:C", "inherits", "a.js:B"}},
		[][3]string{{"a.js:D", "inherits", "a.js:B"}, {"a.js:F", "inherits", "a.js:lib"}, {"a.js:Q", "inherits", "a.js:lib"}, {"a.js:Q", "inherits", "a.js:B"}})
}

// An export alias names the binding, `default` and a re-exported import
// included.
func TestJSAliasedExports(t *testing.T) {
	a := "export default class Dflt {}\nexport class A {}\nexport function f() {}\n"
	idx := "export { f as g, A as AA, default as Named } from \"./a\";\n"
	idd := "export { default } from \"./a\";\n"
	loc := "import { A as Inner } from \"./a\";\nexport { Inner as Outer };\n"
	use := `import { g, AA, Named } from "./idx";
import Dd from "./idd";
import { Outer } from "./loc";
import { default as Def } from "./a";
class E1 extends AA {}
class E2 extends Named {}
class E3 extends Dd {}
class E4 extends Outer {}
class E5 extends Def {}
function h() { g(); }
`
	use2 := "import { f, A } from \"./idx\";\nclass E6 extends A {}\n"
	nodes, edges := resolveSrc(t, [][2]string{{"a.ts", a}, {"idx.ts", idx}, {"idd.ts", idd}, {"loc.ts", loc}, {"use.ts", use}, {"use2.ts", use2}})
	wantGraph(t, nodes, edges, nil,
		[][3]string{
			{"use.ts:use.ts", "imports", "a.ts:f()"},
			{"use.ts:use.ts", "imports", "a.ts:A"},
			{"use.ts:use.ts", "imports", "a.ts:Dflt"},
			{"use.ts:E1", "inherits", "a.ts:A"},
			{"use.ts:E2", "inherits", "a.ts:Dflt"},
			{"use.ts:E3", "inherits", "a.ts:Dflt"},
			{"use.ts:E4", "inherits", "a.ts:A"},
			{"use.ts:E5", "inherits", "a.ts:Dflt"},
			{"loc.ts:loc.ts", "re_exports", "a.ts:a.ts"},
			{"use.ts:h()", "calls", "a.ts:f()"},
		},
		[][3]string{
			{"use2.ts:use2.ts", "imports", "a.ts:f()"},
			{"use2.ts:use2.ts", "imports", "a.ts:A"},
			{"use2.ts:E6", "inherits", "a.ts:A"},
			{"loc.ts:loc.ts", "re_exports", "a.ts:A"},
		})
}

// A named import targets the definition, through barrels; a namespace import
// targets nothing; a re-export gives re_exports.
func TestJSImportSymbols(t *testing.T) {
	m := "export function x() {}\nexport class K {}\nexport const C1 = { a: 1 };\nexport default class Dm {}\n"
	m2 := "export function y() {}\nexport interface Ifc {}\n"
	m3 := "function dd() {}\nexport default dd;\n"
	m4 := "export default function () {}\nexport const z = () => {};\n"
	m5 := "class Foo {}\nexport { Foo as default };\n"
	idx := "export { x, K } from \"./m\";\nexport * from \"./m2\";\nexport * as nsx from \"./m4\";\nexport { default as Dm2 } from \"./m\";\n"
	use := `import { x, y, K as KK, Dm2, gone } from "./index";
import d3 from "./m3";
import d4, { z } from "./m4";
import d5 from "./m5";
import type { Ifc } from "./m2";
import { C1, default as dflt } from "./m";
import * as ns from "./m";
`
	nodes, edges := resolveSrc(t, [][2]string{{"m.ts", m}, {"m2.ts", m2}, {"m3.ts", m3}, {"m4.ts", m4}, {"m5.ts", m5}, {"index.ts", idx}, {"use.ts", use}})
	wantGraph(t, nodes, edges, nil,
		[][3]string{
			{"use.ts:use.ts", "imports", "m.ts:x()"},
			{"use.ts:use.ts", "imports", "m2.ts:y()"},
			{"use.ts:use.ts", "imports", "m.ts:K"},
			{"use.ts:use.ts", "imports", "m3.ts:dd()"},
			{"use.ts:use.ts", "imports", "m4.ts:z()"},
			{"use.ts:use.ts", "imports", "m5.ts:Foo"},
			{"use.ts:use.ts", "imports", "m2.ts:Ifc"},
			{"use.ts:use.ts", "imports", "m.ts:C1"},
			{"use.ts:use.ts", "imports", "m.ts:Dm"},
			{"index.ts:index.ts", "re_exports", "m.ts:x()"},
			{"index.ts:index.ts", "re_exports", "m.ts:K"},
			{"index.ts:index.ts", "re_exports", "m.ts:m.ts"},
			{"index.ts:index.ts", "re_exports", "m2.ts:m2.ts"},
			{"index.ts:index.ts", "re_exports", "m4.ts:m4.ts"},
		},
		[][3]string{
			{"use.ts:use.ts", "imports", "m4.ts:m4.ts"},
			{"index.ts:index.ts", "re_exports", "m.ts:Dm"},
			{"index.ts:index.ts", "re_exports", "m4.ts:z()"},
		})
}

// `./a.js` names a.ts, and a cycle of `export *` barrels ends.
func TestJSImportJsSpecifierAndBarrelCycle(t *testing.T) {
	srcs := [][2]string{
		{"a.ts", "export class A {}\n"},
		{"b.ts", "import { A } from './a.js';\nexport class B extends A {}\n"},
	}
	// 40 barrels that all re-export each other, and an importer of unknown names.
	names := ""
	for i := 0; i < 40; i++ {
		body := ""
		for j := 0; j < 40; j++ {
			if j != i {
				body += "export * from './m" + itoa(j) + "';\n"
			}
		}
		srcs = append(srcs, [2]string{"m" + itoa(i) + ".ts", body})
		names += "x" + itoa(i) + ", "
	}
	srcs = append(srcs, [2]string{"use.ts", "import { " + names + "} from './m0';\n"})
	srcs = append(srcs, [2]string{"self.ts", "import { S } from './self.js';\nexport class S {}\n"},
		[2]string{"c.mts", "export class C {}\n"}, [2]string{"d.ts", "import { C } from './c.mjs';\n"})
	_, edges := resolveSrc(t, srcs)
	for _, want := range [][3]string{
		{"b.ts:b.ts", "imports", "a.ts:A"},
		{"b.ts:B", "inherits", "a.ts:A"},
	} {
		if !edges[want] {
			t.Errorf("missing edge %v", want)
		}
	}
	// upstream pairs only .js/.ts and .jsx/.tsx, and a file does not import itself
	for _, not := range [][3]string{
		{"self.ts:self.ts", "imports_from", "self.ts:self.ts"},
		{"d.ts:d.ts", "imports_from", "c.mts:c.mts"},
	} {
		if edges[not] {
			t.Errorf("unexpected edge %v", not)
		}
	}
}

// A function-valued field and a body-less signature are methods of the class.
func TestJSClassMemberForms(t *testing.T) {
	src := `function helper() {}
export class C {
  plain = 3;
  declare d: number;
  arrow = () => { helper(); };
  static sarrow = () => { helper(); };
  field = function () { helper(); function inField() {} };
  typed: () => void = function () { helper(); };
  over(a: string): void;
  over(a: any) { helper(); }
}
export abstract class A {
  abstract f(): void;
  override abstract h(): void;
}
declare class D { constructor(); dsig(): void; }
class S { only(); }
`
	nodes, edges := resolveSrc(t, [][2]string{{"m.ts", src}})
	wantGraph(t, nodes, edges,
		[][2]string{{"m.ts", "C.arrow()"}, {"m.ts", "C.sarrow()"}, {"m.ts", "C.field()"}, {"m.ts", "C.typed()"},
			{"m.ts", "A.f()"}, {"m.ts", "A.h()"}, {"m.ts", "D.constructor()"}, {"m.ts", "D.dsig()"}, {"m.ts", "S.only()"}},
		[][3]string{
			{"m.ts:C", "contains", "m.ts:C.arrow()"},
			{"m.ts:C", "contains", "m.ts:C.sarrow()"},
			{"m.ts:C", "contains", "m.ts:C.field()"},
			{"m.ts:D", "contains", "m.ts:D.dsig()"},
			{"m.ts:S", "contains", "m.ts:S.only()"},
			{"m.ts:C.arrow()", "calls", "m.ts:helper()"},
			{"m.ts:C.sarrow()", "calls", "m.ts:helper()"},
			{"m.ts:C.field()", "calls", "m.ts:helper()"},
			{"m.ts:C.typed()", "calls", "m.ts:helper()"},
		}, nil)
	for _, n := range [][2]string{{"m.ts", "C.plain()"}, {"m.ts", "C.d()"}, {"m.ts", "A.abstract()"}, {"m.ts", "inField()"}} {
		if nodes[n] {
			t.Errorf("unexpected node %v", n)
		}
	}
	// An overload's node sits at its first signature, as upstream.
	ext := Resolve([]Result{FileFromBytes("m.ts", []byte(src))}, []string{"m.ts"})
	for _, n := range ext.Nodes {
		if n.Label == "C.over()" && n.SourceLocation != "L9" {
			t.Errorf("C.over() at %s, want L9", n.SourceLocation)
		}
	}
}

// An overloaded method with a body is no abstract method: an abstract
// one loses a call to an equal-named method elsewhere.
func TestJSOverloadNotAbstract(t *testing.T) {
	r := FileFromBytes("a.ts", []byte("export class API {\n  run(x: string): void;\n  run(x: any) {}\n  sig(): void;\n}\n"))
	got := map[string]bool{}
	for _, d := range r.Defs {
		got[d.Name] = d.Abstract
	}
	if a, ok := got["run"]; !ok || a {
		t.Errorf("run: present %v abstract %v, want a concrete def", ok, a)
	}
	if !got["sig"] {
		t.Errorf("sig: want an abstract def")
	}
}
