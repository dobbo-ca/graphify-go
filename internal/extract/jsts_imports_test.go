package extract

import (
	"slices"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// A .js specifier names a .ts or .tsx file; a real .js file wins.
func TestJSImportJsFallsBackToTsx(t *testing.T) {
	vue := "<script setup lang=\"ts\">\nimport { Button } from './Button.js'\n</script>\n"
	_, edges := resolveSrc(t, [][2]string{
		{"Button.tsx", "export function Button() {}\n"},
		{"Widget.tsx", "export function render() {}\n"},
		{"other.ts", "export function render() {}\n"},
		{"Comp.tsx", "export function Comp() {}\n"},
		{"both.ts", "export function bothTs() {}\n"},
		{"both.tsx", "export function bothTsx() {}\n"},
		{"real.js", "export function realJs() {}\n"},
		{"real.ts", "export function realTs() {}\n"},
		{"jsx.ts", "export function jsxTs() {}\n"},
		{"barrel.ts", "export * from './Comp.js';\n"},
		{"main.ts", `import { Button } from './Button.js';
import { render } from './Widget.js';
import { bothTs } from './both.js';
import { realJs } from './real.js';
import { jsxTs } from './jsx.jsx';
export function main() { Button(); render(); bothTs(); realJs(); jsxTs() }
`},
		{"plain.js", "import { Button } from './Button.js';\n"},
		{"App.vue", vue},
	})
	for _, want := range [][3]string{
		{"main.ts:main.ts", "imports_from", "Button.tsx:Button.tsx"},
		{"main.ts:main.ts", "imports_from", "Widget.tsx:Widget.tsx"},
		{"main.ts:main.ts", "imports_from", "both.ts:both.ts"},
		{"main.ts:main.ts", "imports_from", "real.js:real.js"},
		{"plain.js:plain.js", "imports_from", "Button.tsx:Button.tsx"},
		{"App.vue:App.vue", "imports_from", "Button.tsx:Button.tsx"},
		{"barrel.ts:barrel.ts", "re_exports", "Comp.tsx:Comp.tsx"},
		// the alias binds, not a tie-break
		{"main.ts:main()", "calls", "Widget.tsx:render()"},
	} {
		if !edges[want] {
			t.Errorf("missing edge %v", want)
		}
	}
	for _, not := range [][3]string{
		{"main.ts:main.ts", "imports_from", "both.tsx:both.tsx"},
		{"main.ts:main.ts", "imports_from", "real.ts:real.ts"},
		{"main.ts:main.ts", "imports_from", "jsx.ts:jsx.ts"}, // .jsx never maps to .ts
	} {
		if edges[not] {
			t.Errorf("unexpected edge %v", not)
		}
	}
}

// A call from a component never binds to a Python definition.
func TestSFCCallDoesNotCrossToPython(t *testing.T) {
	_, edges := resolveSrc(t, [][2]string{
		{"App.vue", "<script setup lang=\"ts\">\nfunction run() { processData() }\n</script>\n"},
		{"util.py", "def processData():\n    pass\n"},
	})
	for e := range edges {
		if e[1] == "calls" && e[0][:3] == "App" {
			t.Errorf("unexpected edge %v", e)
		}
	}
}

// edgeSet names each edge "file:label relation file:label", with " deferred"
// after a deferred one, for the relations that tests below look at.
func edgeSet(t *testing.T, srcs [][2]string, rels ...string) map[string]bool {
	t.Helper()
	var results []Result
	var files []string
	for _, s := range srcs {
		results = append(results, FileFromBytes(s[0], []byte(s[1])))
		files = append(files, s[0])
	}
	ext := Resolve(results, files)
	name := map[string]string{}
	for _, n := range ext.Nodes {
		name[n.ID] = n.SourceFile + ":" + n.Label
	}
	out := map[string]bool{}
	for _, e := range ext.Edges {
		if !slices.Contains(rels, e.Relation.String()) {
			continue
		}
		k := name[e.Source] + " " + e.Relation.String() + " " + name[e.Target]
		if e.Deferred() {
			k += " deferred"
		}
		out[k] = true
	}
	return out
}

func checkEdges(t *testing.T, got map[string]bool, want, not []string) {
	t.Helper()
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing edge %q", w)
		}
	}
	for _, n := range not {
		if got[n] {
			t.Errorf("unexpected edge %q", n)
		}
	}
}

// `new X()` binds like a bare call, but across files only with an import.
func TestJSNewExpression(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"shapes.ts", "export class Circle {}\nexport class Item {}\n"},
		{"other.ts", "export class Item {}\nexport class Lonely {}\n"},
		{"orphan.ts", "export class Orphan {}\n"},
		{"side.ts", "export class Side {}\n"},
		{"main.ts", `import { Circle, Item } from './shapes';
import * as ns from './other';
class Local {}
export function mk() { return new Local(); }
export function mkNoParens() { return new Local; }
export const arrow = () => new Local();
export function mkCircle() { return new Circle(); }
export function mkItem() { return new Item(); }
export function mkLonely() { return new Lonely(); }
export function mkNs() { return new ns.Item(); }
export function mkMap() { return new Map(); }
export function mkOrphan() { return new Orphan(); }
export class Holder { make() { return new Circle(); } }
const top = new Circle();
describe('x', () => { new Circle(); new Local(); });
`},
		{"effect.ts", "import './side';\nexport function c() { return new Side(); }\n"},
		{"noeffect.ts", "export function c() { return new Side(); }\n"},
		{"plain.js", "class JsThing {}\nexport function mkJs() { return new JsThing(); }\n"},
		{"builtin.ts", "class Response {}\nexport function r() { return new Response(); }\n"},
	}, "calls")
	checkEdges(t, got, []string{
		"main.ts:mk() calls main.ts:Local",
		"main.ts:mkNoParens() calls main.ts:Local",
		"main.ts:arrow() calls main.ts:Local",
		"main.ts:mkCircle() calls shapes.ts:Circle",
		"main.ts:mkItem() calls shapes.ts:Item", // the import picks one of two
		"main.ts:mkLonely() calls other.ts:Lonely",
		"main.ts:Holder.make() calls shapes.ts:Circle",
		"effect.ts:c() calls side.ts:Side",
		"plain.js:mkJs() calls plain.js:JsThing",
	}, []string{
		"main.ts:mkItem() calls other.ts:Item",
		"main.ts:mkNs() calls other.ts:Item", // a namespace member is no edge
		"main.ts:mkNs() calls shapes.ts:Item",
		"main.ts:mkOrphan() calls orphan.ts:Orphan", // never imported
		"noeffect.ts:c() calls side.ts:Side",
		"builtin.ts:r() calls builtin.ts:Response",
	})
	// no edge: builtin, file level or closure
	for k := range got {
		if strings.HasPrefix(k, "main.ts:mkMap") || strings.HasPrefix(k, "main.ts:main.ts calls") {
			t.Errorf("unexpected edge %q", k)
		}
	}
}

// import() is a file-level dynamic_import once per target, and a deferred
// imports_from from the function holding it; require() stays imports_from.
func TestJSDynamicImport(t *testing.T) {
	dyn := `const top = import("./a.js");
export function f() { return import("./b"); }
export const g = async () => (await import("./c")).c;
export function t() { return import(` + "`./d`" + `); }
const r = require("./e");
export function lazy() { const m = require("./f"); return m; }
export function ext() { return import("lodash"); }
export function twice() { import("./b"); return import("./b", { with: {} }); }
function inner() { return import("./h"); }
export function sub(n) { return import(` + "`./${n}`" + `); }
`
	srcs := [][2]string{{"dyn.ts", dyn}, {"dyn2.js", "export function jf() { return import('./a.js'); }\n"}}
	for _, f := range []string{"a", "b", "c", "d", "e", "f", "h"} {
		srcs = append(srcs, [2]string{f + ".ts", "export function " + f + "() {}\n"})
	}
	got := edgeSet(t, srcs, "imports_from", "dynamic_import", "imports")
	checkEdges(t, got, []string{
		"dyn.ts:dyn.ts dynamic_import a.ts:a.ts",
		"dyn.ts:dyn.ts dynamic_import b.ts:b.ts",
		"dyn.ts:dyn.ts dynamic_import c.ts:c.ts",
		"dyn.ts:dyn.ts dynamic_import d.ts:d.ts",
		"dyn.ts:dyn.ts dynamic_import h.ts:h.ts",
		"dyn.ts:dyn.ts dynamic_import :lodash",
		"dyn2.js:dyn2.js dynamic_import a.ts:a.ts",
		"dyn.ts:f() imports_from b.ts:b.ts deferred",
		"dyn.ts:twice() imports_from b.ts:b.ts deferred",
		"dyn.ts:g() imports_from c.ts:c.ts deferred",
		"dyn.ts:t() imports_from d.ts:d.ts deferred",
		"dyn.ts:inner() imports_from h.ts:h.ts deferred",
		"dyn.ts:ext() imports_from :lodash deferred",
		"dyn2.js:jf() imports_from a.ts:a.ts deferred",
		"dyn.ts:dyn.ts imports_from e.ts:e.ts",
		"dyn.ts:dyn.ts imports_from f.ts:f.ts", // require() stays at the file
	}, []string{
		"dyn.ts:dyn.ts imports_from a.ts:a.ts",
		"dyn.ts:dyn.ts imports_from b.ts:b.ts",
		"dyn.ts:dyn.ts imports_from h.ts:h.ts",
		"dyn.ts:dyn.ts dynamic_import e.ts:e.ts",
		"dyn.ts:dyn.ts imports :lodash",
	})
	n := 0
	for k := range got {
		if strings.HasPrefix(k, "dyn.ts:dyn.ts dynamic_import") {
			n++
		}
	}
	if n != 6 {
		t.Errorf("dyn.ts has %d dynamic_import edges, want 6 (a b c d h lodash)", n)
	}
}

// An import() in a closure of a top-level call is the file's deferred import,
// and replaces its dynamic_import. In a function it is both.
func TestJSDynamicImportInTopLevelClosure(t *testing.T) {
	a := [2]string{"a.ts", "export function a() {}\n"}
	got := edgeSet(t, [][2]string{a, {"spec.ts", "describe('x', () => { import('./a'); });\n"}}, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{"spec.ts:spec.ts imports_from a.ts:a.ts deferred"},
		[]string{"spec.ts:spec.ts dynamic_import a.ts:a.ts", "spec.ts:spec.ts imports_from a.ts:a.ts"})

	// closure beats function
	got = edgeSet(t, [][2]string{a, {"spec.ts", "function f() { import('./a'); }\ndescribe('x', () => { import('./a'); });\n"}}, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{"spec.ts:spec.ts imports_from a.ts:a.ts deferred", "spec.ts:f() imports_from a.ts:a.ts deferred"},
		[]string{"spec.ts:spec.ts dynamic_import a.ts:a.ts"})
}

// Components get a dynamic_import for the file and a deferred edge for the function.
func TestSFCDynamicImport(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"a.ts", "export function a() {}\n"},
		{"b.ts", "export function b() {}\n"},
		{"Comp.vue", "<template><div/></template>\n<script setup lang=\"ts\">\nconst A = () => import('./a.js');\n</script>\n"},
		{"Page.svelte", "<script lang=\"ts\">\n  const B = import('./b');\n</script>\n<div/>\n"},
		{"Lay.astro", "---\nconst m = await import('./a');\n---\n<div/>\n"},
	}, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{
		"Comp.vue:Comp.vue dynamic_import a.ts:a.ts",
		"Comp.vue:A() imports_from a.ts:a.ts deferred",
		"Page.svelte:Page.svelte dynamic_import b.ts:b.ts",
		"Lay.astro:Lay.astro dynamic_import a.ts:a.ts",
	}, []string{
		"Comp.vue:Comp.vue imports_from a.ts:a.ts",
		"Page.svelte:Page.svelte imports_from b.ts:b.ts",
		"Lay.astro:Lay.astro imports_from a.ts:a.ts",
	})
}

// Only a lone literal in `import(` makes a file dynamic_import; a recovered
// call, a comment argument or a `$` template still defers.
func TestJSDynamicImportShapes(t *testing.T) {
	srcs := [][2]string{
		{"main.js", "import a = require(\"./a\");\nfunction f() { import c = require(\"./c\") }\nimport.defer(\"./e\");\n"},
		{"m.ts", `export function x() { return import("./a", { with: { type: "json" } }); }
export const top = import("./b", { with: {} });
export function w() { return import(/* chunk */ './c'); }
export function d() { return import(` + "`./e$f`" + `); }
`},
	}
	for _, f := range []string{"a", "b", "c", "e"} {
		srcs = append(srcs, [2]string{f + ".ts", "export const " + f + " = 1;\n"})
	}
	got := edgeSet(t, srcs, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{
		"main.js:f() imports_from c.ts:c.ts deferred",
		"m.ts:x() imports_from a.ts:a.ts deferred",
		"m.ts:w() imports_from c.ts:c.ts deferred",
		"m.ts:d() imports_from :./e$f deferred",
		"main.js:main.js imports_from a.ts:a.ts", // as on main
	}, []string{
		"main.js:main.js dynamic_import a.ts:a.ts",
		"main.js:main.js dynamic_import c.ts:c.ts",
		"main.js:main.js dynamic_import e.ts:e.ts",
		"m.ts:m.ts dynamic_import a.ts:a.ts",
		"m.ts:m.ts dynamic_import b.ts:b.ts",
		"m.ts:m.ts dynamic_import c.ts:c.ts",
	})
}

// Upstream finds dynamic_import by regex: comments and strings count, a
// line comment or a longer name does not, and a static import hides it.
func TestJSDynamicImportRegex(t *testing.T) {
	srcs := [][2]string{
		{"r.ts", `import { s } from './s';
import type { ty } from './ty';
export * from './re';
/** @type {import("./a").A} */
const str = "import('./b')";
/* import('./c') */
// import('./d')
const x = reimport('./e'), y = import('./s'), z = import('./ty'), w = import('./re');
`},
	}
	for _, f := range []string{"a", "b", "c", "d", "e", "s", "ty", "re"} {
		srcs = append(srcs, [2]string{f + ".ts", "export const " + f + " = 1;\n"})
	}
	got := edgeSet(t, srcs, "imports_from", "dynamic_import", "re_exports")
	checkEdges(t, got, []string{
		"r.ts:r.ts dynamic_import a.ts:a.ts",
		"r.ts:r.ts dynamic_import b.ts:b.ts",
		"r.ts:r.ts dynamic_import c.ts:c.ts",
		"r.ts:r.ts imports_from ty.ts:ty.ts",
	}, []string{
		"r.ts:r.ts dynamic_import d.ts:d.ts",
		"r.ts:r.ts dynamic_import e.ts:e.ts",
		"r.ts:r.ts dynamic_import s.ts:s.ts",
		"r.ts:r.ts dynamic_import ty.ts:ty.ts",
		"r.ts:r.ts dynamic_import re.ts:re.ts",
	})
}

// A component's dynamic_import is its regex alone: no template literal, no
// extra argument. The script still defers.
func TestSFCDynamicImportShapes(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"a.ts", "export const a = 1;\n"},
		{"b.ts", "export const b = 1;\n"},
		{"c.ts", "export const c = 1;\n"},
		{"R.vue", "<script setup lang=\"ts\">\nasync function load() { await import(`./a`); await import('./b', { with: {} }); }\nfunction f() { return import(/* c */ './c') }\n</script>\n"},
	}, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{
		"R.vue:load() imports_from a.ts:a.ts deferred",
		"R.vue:load() imports_from b.ts:b.ts deferred",
		"R.vue:f() imports_from c.ts:c.ts deferred",
	}, []string{
		"R.vue:R.vue dynamic_import a.ts:a.ts",
		"R.vue:R.vue dynamic_import b.ts:b.ts",
		"R.vue:R.vue dynamic_import c.ts:c.ts",
	})
}

// './self.js' in self.ts never falls through to the self.tsx sibling.
func TestJSImportJsSelfSkip(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"self.ts", "import { X } from './self.js';\nexport class X {}\nexport function s() { return new X(); }\n"},
		{"self.tsx", "export class Z {}\n"},
	}, "imports_from", "imports", "calls")
	checkEdges(t, got, []string{"self.ts:s() calls self.ts:X"},
		[]string{"self.ts:self.ts imports_from self.tsx:self.tsx", "self.ts:s() calls self.tsx:X"})
}

// `new X()` binds an imported class by its own name, once, outside its own
// members; a local class does not hide it, as upstream has no node for one.
func TestJSNewBinding(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"x.ts", "export class X {}\nexport class Y {}\nexport class Widget {}\nexport default class Thing {}\n"},
		{"item.ts", "export class Other {}\n"},
		{"barrel.ts", "export { Other as Renamed } from './item';\n"},
		{"b.ts", `import { X, Y, Widget as W } from './x';
import Def from './x';
import { Renamed } from './barrel';
export class B {
  retM(): X { return new X(1); }
  noRetM() { return new Y(2); }
  paramM(x: X) { return new X(3); }
  static make() { return new B(); }
}
export function alias() { return new W(); }
export function dflt() { return new Def(); }
export function barrel() { return new Renamed(); }
export function shadowFn() { function Y() {} return new Y(); }
export function shadowCls() { class Y {} return new Y(); }
`},
	}, "calls")
	checkEdges(t, got, []string{"b.ts:B.noRetM() calls x.ts:Y", "b.ts:shadowCls() calls x.ts:Y"}, []string{
		"b.ts:B.retM() calls x.ts:X", // a references edge is there already
		"b.ts:B.paramM() calls x.ts:X",
		"b.ts:B.make() calls b.ts:B", // upstream drops the owner pair
		"b.ts:alias() calls x.ts:Widget",
		"b.ts:dflt() calls x.ts:Thing",
		"b.ts:barrel() calls item.ts:Other",
		"b.ts:shadowFn() calls x.ts:Y",
	})
}

// A namespace class binds like a bare call, but not from its own member; a
// local class shadows nothing, and an aliased import falls back to the class
// of that name.
func TestJSNewScopes(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"ns.ts", `namespace Foo.Bar { class Greeter {} function foo() { return new Greeter(); } }
module m1 { class C2 {} export function f11() { return new C2(); } }
namespace Solo { export class Box {} export function mk() { return new Box(); } }
namespace N { export class A { m() { const f = () => new A(); } static s() { return new A(); } } export class C { n() { return new C(); } } export class D { g() { return new A(); } } }
module P.Q { export class E { static mk() { return new E(); } } }
`},
		{"m.ts", "export class Widget {}\n"},
		{"u.ts", `import { Widget } from './m';
export function sh3() { if (true) { class Widget {} return new Widget(); } }
export function sh5() { { class Widget {} } return new Widget(); }
export function sh6() { const g = () => { class Widget {} return 1; }; return new Widget(); }
export function own() { class Widget {} return new Widget(); }
`},
		{"lib.ts", "export class ChannelClient {}\nexport class IPCClient {}\n"},
		{"use.ts", "import { ChannelClient as IPCClient } from './lib';\nexport function mk() { return new IPCClient(); }\n"},
		{"st/a.ts", "export class Store {}\n"},
		{"st/b.ts", "export class Store {}\n"},
		{"two.ts", "import { Store as S } from './st/a';\nimport './st/b';\nexport function b2() { return new Store(); }\n"},
	}, "calls")
	checkEdges(t, got, []string{
		"ns.ts:foo() calls ns.ts:Greeter",
		"ns.ts:f11() calls ns.ts:C2",
		"ns.ts:mk() calls ns.ts:Box",
		"u.ts:sh3() calls m.ts:Widget",
		"u.ts:sh5() calls m.ts:Widget",
		"u.ts:sh6() calls m.ts:Widget",
		"u.ts:own() calls m.ts:Widget",
		"ns.ts:D.g() calls ns.ts:A",
		"use.ts:mk() calls lib.ts:IPCClient",
		"two.ts:b2() calls st/a.ts:Store",
	}, []string{
		"use.ts:mk() calls lib.ts:ChannelClient",
		"ns.ts:A.m() calls ns.ts:A", "ns.ts:A.s() calls ns.ts:A", "ns.ts:C.n() calls ns.ts:C", "ns.ts:E.mk() calls ns.ts:E",
	})
}

// A new X() in a closure passed to a top-level call is the file's, unless an
// imports or contains edge already links the pair; module scope makes none.
func TestJSNewFileClosure(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"x/a.ts", "export class Foo {}\nexport class Bar {}\nexport function helper() {}\n"},
		{"t.ts", "import './x/a';\nsuite('s', () => { new Foo(); });\n"},
		{"u.ts", "import * as ns from './x/a';\nsuite('s', async () => { new Bar(); });\nnew Foo();\n"},
		{"v.ts", "import { helper } from './x/a';\nsuite(() => { new Foo(); new Bar(); });\n"},
		{"n.ts", "import { Foo } from './x/a';\nsuite('s', () => { new Foo(); });\n"},
		{"w.ts", "export class Loc {}\nsuite('s', () => { new Loc(); });\n"},
	}, "calls")
	checkEdges(t, got, []string{
		"t.ts:t.ts calls x/a.ts:Foo", "u.ts:u.ts calls x/a.ts:Bar", "v.ts:v.ts calls x/a.ts:Foo", "v.ts:v.ts calls x/a.ts:Bar",
	}, []string{"u.ts:u.ts calls x/a.ts:Foo", "n.ts:n.ts calls x/a.ts:Foo", "w.ts:w.ts calls w.ts:Loc"})
}

// './x.js' is x.ts before x.js.ts or x.js/index.ts, as upstream tries it.
func TestJSImportJsSwapFirst(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"lib/x.ts", "export class A {}\n"}, {"lib/x.js.ts", "export class A {}\n"},
		{"lib2/y.ts", "export class B {}\n"}, {"lib2/y.js/index.ts", "export class B {}\n"},
		{"m.ts", "import './lib/x.js';\nimport './lib2/y.js';\n"},
	}, "imports_from")
	checkEdges(t, got, []string{"m.ts:m.ts imports_from lib/x.ts:x.ts", "m.ts:m.ts imports_from lib2/y.ts:y.ts"},
		[]string{"m.ts:m.ts imports_from lib/x.js.ts:x.js.ts", "m.ts:m.ts imports_from lib2/y.js/index.ts:index.ts"})
}

// A file-level import() replaces the static edge's line, either order.
func TestJSDeferredMergeTakesImportSite(t *testing.T) {
	srcs := [][2]string{
		{"s1.ts", "import {y1} from './y1';\nsetup(() => import('./y1'));\n"},
		{"s2.ts", "setup(() => import('./y2'));\nimport {y2} from './y2';\n"},
		{"s3.ts", "import type {y3} from './y3';\nsetup(() => import('./y3'));\n"},
		{"s4.ts", "setup(() => import('./y4'));\nimport type {y4} from './y4';\n"},
	}
	for _, y := range []string{"y1", "y2", "y3", "y4"} {
		srcs = append(srcs, [2]string{y + ".ts", "export const y = 1;\n"})
	}
	var results []Result
	var files []string
	for _, s := range srcs {
		results = append(results, FileFromBytes(s[0], []byte(s[1])))
		files = append(files, s[0])
	}
	line := map[string]string{"s1.ts": "L2", "s2.ts": "L1", "s3.ts": "L2", "s4.ts": "L1"}
	n := 0
	for _, e := range Resolve(results, files).Edges {
		if e.Relation == model.RelImportsFrom && e.Deferred() {
			if f := e.SourceFile.String(); line[f] != e.SourceLocation.String() {
				t.Errorf("%s: location %s, want %s", f, e.SourceLocation, line[f])
			}
			n++
		}
	}
	if n != 4 {
		t.Errorf("got %d deferred edges, want 4", n)
	}
}

// import(' ./d ') trims spaces inside the quotes.
func TestJSDynamicImportSpaces(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"a.ts", "export function f2() { return import(' ./d2 '); }\n"},
		{"d2.ts", "export const d = 1;\n"},
	}, "imports_from")
	checkEdges(t, got, []string{"a.ts:f2() imports_from d2.ts:d2.ts deferred"}, nil)
}

// An external module imported statically still gets the file's dynamic_import.
func TestJSDynamicImportExternalBehindStatic(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"a.ts", "import React from 'react';\nexport function f() { return import('react'); }\n"},
	}, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{"a.ts:a.ts dynamic_import :react", "a.ts:f() imports_from :react deferred"}, nil)
}

// A file-level deferred import marks the static one of the pair, whatever the order.
func TestJSDeferredBeatsStaticEitherOrder(t *testing.T) {
	b := [2]string{"b.js", "const a = require('./a');\n"}
	for _, a := range []string{
		"describe('x', () => { import('./b'); });\nconst b = require('./b');\n",
		"const b = require('./b');\ndescribe('x', () => { import('./b'); });\n",
		"import './b';\ndescribe('x', () => { import('./b'); });\n",
	} {
		got := edgeSet(t, [][2]string{{"a.js", a}, b}, "imports_from")
		checkEdges(t, got, []string{"a.js:a.js imports_from b.js:b.js deferred"}, []string{"a.js:a.js imports_from b.js:b.js"})
	}
}

// A static import of the target hides the file's dynamic_import.
func TestJSDynamicImportBehindStaticImport(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"a.ts", "export const a = 1;\n"},
		{"s.ts", "import './a';\nexport function g() { return import('./a'); }\n"},
		{"late.ts", "export function g() { return import('./a'); }\nimport './a';\n"},
	}, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{"s.ts:g() imports_from a.ts:a.ts deferred", "late.ts:late.ts imports_from a.ts:a.ts"},
		[]string{"s.ts:s.ts dynamic_import a.ts:a.ts", "late.ts:late.ts dynamic_import a.ts:a.ts"})
}

// An ambiguous new X() binds only by a unique symbol import, then by a unique
// imported file; never by directory, and never past a second symbol import.
func TestJSNewAmbiguous(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"a.ts", "export class X {}\n"},
		{"sub/b.ts", "export class X {}\n"},
		{"c.ts", "import './a';\nimport './sub/b';\nexport function f() { return new X(); }\n"},
		{"d.ts", "import * as A from './a';\nimport * as B from './sub/b';\nexport function g() { return new X(); }\n"},
		{"sub/s.ts", "import '../a';\nimport './b';\nexport function k() { return new X(); }\n"},
		{"e.ts", "import './sub/b';\nexport function h() { return new X(); }\n"},
		{"st/a.ts", "export class Store {}\n"},
		{"st/b.ts", "export class Store {}\n"},
		{"u1.ts", "import { Store } from './st/a';\nimport { Store as S2 } from './st/b';\nexport function f1() { return new Store(); }\n"},
		{"u2.ts", "import { Store } from './st/a';\nimport type { Store as T2 } from './st/b';\nexport function g2() { return new Store(); }\n"},
		{"u3.ts", "import { Store } from './st/a';\nexport function h3() { return new Store(); }\n"},
	}, "calls")
	checkEdges(t, got, []string{"e.ts:h() calls sub/b.ts:X", "u3.ts:h3() calls st/a.ts:Store"}, []string{
		"c.ts:f() calls a.ts:X", "c.ts:f() calls sub/b.ts:X",
		"d.ts:g() calls a.ts:X", "d.ts:g() calls sub/b.ts:X",
		"sub/s.ts:k() calls a.ts:X", "sub/s.ts:k() calls sub/b.ts:X",
		"u1.ts:f1() calls st/a.ts:Store", "u1.ts:f1() calls st/b.ts:Store",
		"u2.ts:g2() calls st/a.ts:Store", "u2.ts:g2() calls st/b.ts:Store",
	})
}

// The regex pass tracks lines and `//` per line while it scans once.
func TestJSDynamicImportLines(t *testing.T) {
	src := "import('./a');\n\nconst x = 1; import('./b'); // import('./c')\n// import('./d')\nf(import('./e'), import('./f'));\n"
	var got []string
	for _, im := range FileFromBytes("m.ts", []byte(src)).Imps {
		if im.Dyn == dynImport {
			got = append(got, im.Spec+"@"+im.Loc.String())
		}
	}
	if want := "./a@L1 ./b@L3 ./e@L5 ./f@L5"; strings.Join(got, " ") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// import("/x") names no package: only the function defers it.
func TestJSDynamicImportAbsolute(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"a.ts", "/** @type {typeof import(\"/some-mod\")} */\nconst top = import(\"/abs/top\");\nexport function f() { return import(\"/abs/inner\"); }\n"},
		{"R.vue", "<script setup lang=\"ts\">\nfunction g() { return import('/abs/vue'); }\n</script>\n"},
	}, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{"a.ts:f() imports_from :/abs/inner deferred", "R.vue:g() imports_from :/abs/vue deferred"}, []string{
		"a.ts:a.ts dynamic_import :/some-mod", "a.ts:a.ts dynamic_import :/abs/top", "a.ts:a.ts dynamic_import :/abs/inner",
		"R.vue:R.vue dynamic_import :/abs/vue",
	})
}

// A .js specifier that falls back to its own file is resolved, not external.
func TestJSImportJsSelfNotExternal(t *testing.T) {
	srcs := [][2]string{
		{"self.ts", "import './self.js';\nexport async function s() { return import('./self.js'); }\n"},
		{"self.tsx", "export class Z {}\n"},
		{"me.ts", "export class M {}\n"},
		{"me.tsx", "import './me.jsx';\n"},
	}
	var results []Result
	var files []string
	for _, s := range srcs {
		results = append(results, FileFromBytes(s[0], []byte(s[1])))
		files = append(files, s[0])
	}
	for _, n := range Resolve(results, files).Nodes {
		if strings.Contains(n.Label, ".js") {
			t.Errorf("external stub node %q", n.Label)
		}
	}
	got := edgeSet(t, srcs, "imports", "imports_from", "dynamic_import")
	checkEdges(t, got, nil, []string{
		"self.ts:self.ts imports :./self.js", "me.tsx:me.tsx imports :./me.jsx", "self.ts:self.ts imports_from self.tsx:self.tsx",
		"self.ts:self.ts imports_from self.ts:self.ts", "me.tsx:me.tsx imports_from me.tsx:me.tsx",
	})
}

// Only the static './x.js' self import is dropped: import() of the file
// itself still gives the file's dynamic_import and the function's deferred edge.
func TestJSDynamicImportSelfFallback(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"self.ts", "export function s1() { return import('./self.js'); }\n"},
		{"tx.tsx", "export function s2() { return import('./tx.jsx'); }\n"},
		{"st.ts", "import './st.js';\nexport function s3() { return import('./st.js'); }\n"},
	}, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{
		"self.ts:self.ts dynamic_import self.ts:self.ts", "self.ts:s1() imports_from self.ts:self.ts deferred",
		"tx.tsx:tx.tsx dynamic_import tx.tsx:tx.tsx", "tx.tsx:s2() imports_from tx.tsx:tx.tsx deferred",
		"st.ts:st.ts dynamic_import st.ts:st.ts",
	}, []string{"st.ts:st.ts imports_from st.ts:st.ts"})
}

// Upstream's closure walk skips the params and type of a nested callable, so
// an import() there gives the file its dynamic_import and no deferred edge.
func TestJSDynamicImportNestedSignature(t *testing.T) {
	srcs := [][2]string{{"main.ts", `declare function setup(f: () => void): void;
declare function stub(o: any): void;
setup(() => {
  stub({ get(m: import('./a.js').Ta) { return 1; } });
  stub({ get(): import('./c.js').Tc | undefined { return undefined; } });
  class K { m(): import('./d.js').Td | undefined { return undefined; } }
  class L { constructor(p: import('./e.js').Te) {} }
  function inner(x: import('./f.js').Tf) { return import('./g.js'); }
  const g = (t: import('./b.js').Tb[]) => t;
});
export function f() {
  function nested(x = import('./h.js')) { return x; }
}
export let sys = (() => { function nf() { return import('./i1.js'); } const o = { om() { return import('./i2.js'); } }; class K { cm() { return import('./i3.js'); } } function* g() { yield import('./i4.js'); } })();
`}}
	for _, f := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i1", "i2", "i3", "i4"} {
		srcs = append(srcs, [2]string{f + ".ts", "export const " + f + " = 1;\n"})
	}
	got := edgeSet(t, srcs, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{
		"main.ts:main.ts imports_from b.ts:b.ts deferred", // arrow params stay
		"main.ts:inner() imports_from g.ts:g.ts deferred", // bodies stay
		"main.ts:main.ts dynamic_import a.ts:a.ts",
		"main.ts:main.ts dynamic_import c.ts:c.ts",
		"main.ts:main.ts dynamic_import d.ts:d.ts",
		"main.ts:main.ts dynamic_import e.ts:e.ts",
		"main.ts:main.ts dynamic_import f.ts:f.ts",
		"main.ts:main.ts dynamic_import h.ts:h.ts",
		"main.ts:sys imports_from i1.ts:i1.ts deferred", // var IIFE body stays
		"main.ts:sys imports_from i4.ts:i4.ts deferred",
	}, []string{
		"main.ts:sys imports_from i2.ts:i2.ts deferred",
		"main.ts:sys imports_from i3.ts:i3.ts deferred",
		"main.ts:main.ts imports_from a.ts:a.ts deferred",
		"main.ts:main.ts imports_from c.ts:c.ts deferred",
		"main.ts:main.ts imports_from d.ts:d.ts deferred",
		"main.ts:main.ts imports_from e.ts:e.ts deferred",
		"main.ts:main.ts imports_from f.ts:f.ts deferred",
		"main.ts:f() imports_from h.ts:h.ts deferred",
	})
}

// A generic call that tree-sitter misreads as a comparison has its import()
// type masked upstream, so only the regex pass sees it.
func TestJSDynamicImportMaskedTypeArgs(t *testing.T) {
	srcs := [][2]string{{"s.ts", `declare function foo<T>(...a: any[]): Promise<T>;
declare function suite(n: string, f: () => void): void;
export function a1() { return foo<typeof import("./t1")>(); }
export async function a2() { return await foo<typeof import('./t2')>(1, ''); }
export function a3() { return foo<import("./t3").X>(); }
export function a4() { return new Map<string, typeof import("./t4")>(); }
suite("x", () => { foo<typeof import("./u")>(); });
export function n1() { return foo<(typeof import('./a'))>(); }
export function n3() { return foo<[typeof import('./c')]>(); }
export function m12() { return foo<(typeof import('./l'))['vl']>(); }
`}}
	for _, f := range []string{"t1", "t2", "t3", "t4", "u", "a", "c", "l"} {
		srcs = append(srcs, [2]string{f + ".ts", "export const " + f + " = 1;\n"})
	}
	got := edgeSet(t, srcs, "imports_from", "dynamic_import")
	checkEdges(t, got, []string{
		"s.ts:s.ts dynamic_import t1.ts:t1.ts", "s.ts:s.ts dynamic_import t3.ts:t3.ts", "s.ts:s.ts dynamic_import u.ts:u.ts",
		"s.ts:a2() imports_from t2.ts:t2.ts deferred", "s.ts:a4() imports_from t4.ts:t4.ts deferred",
	}, []string{
		"s.ts:a1() imports_from t1.ts:t1.ts deferred", "s.ts:a3() imports_from t3.ts:t3.ts deferred",
		"s.ts:s.ts imports_from u.ts:u.ts deferred",
		"s.ts:n1() imports_from a.ts:a.ts deferred", "s.ts:n3() imports_from c.ts:c.ts deferred", "s.ts:m12() imports_from l.ts:l.ts deferred",
	})
}

// A component keeps an import and a call bound through it, whatever the language.
func TestSFCImportKeepsPythonCall(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"util.py", "def helper(): pass\n"},
		{"App.vue", "<script setup lang=\"ts\">\nimport { helper } from './util.py';\nfunction run() { helper(); other(); }\n</script>\n"},
		{"o.py", "def other(): pass\n"},
	}, "imports_from", "calls")
	checkEdges(t, got, []string{"App.vue:App.vue imports_from util.py:util.py", "App.vue:run() calls util.py:helper()"},
		[]string{"App.vue:run() calls o.py:other()"})
}

// Namespaces, values and class members of the same name make a new X() ambiguous.
func TestJSNewAmbiguousWithOtherNodes(t *testing.T) {
	got := edgeSet(t, [][2]string{
		{"s1.ts", "export class A {}\nexport class D {}\nexport class E {}\nexport class Order {}\nexport class User {}\nexport class Solo {}\nexport namespace M { export const z = 1; }\n"},
		{"t.ts", "export namespace A { export const x = 1; }\nexport const D = 1;\nexport class K { E() {} }\n"},
		{"api.ts", "export class Client { User() { return 1; } }\n"},
		{"m.ts", "export class M {}\n"},
		{"u.ts", "import * as S from './s1';\nimport * as T from './t';\nimport * as Q from './m';\nexport function f() { new A(); new D(); new E(); new M(); }\n"},
		{"w.ts", "import { A, D } from './s1';\nimport './t';\nexport function h() { new A(); new D(); new E(); }\n"},
		{"svc.ts", "import { Order as O } from './s1';\nimport { Client } from './api';\nexport function make() { new User(); new Order(); return new Client(); }\n"},
		{"solo.ts", "import { Solo } from './s1';\nexport function k() { return new Solo(); }\n"},
	}, "calls")
	checkEdges(t, got, []string{"svc.ts:make() calls api.ts:Client", "solo.ts:k() calls s1.ts:Solo", "w.ts:h() calls s1.ts:A", "w.ts:h() calls s1.ts:D"}, []string{
		"u.ts:f() calls s1.ts:A", "u.ts:f() calls s1.ts:D", "u.ts:f() calls s1.ts:E", "u.ts:f() calls m.ts:M", "u.ts:f() calls s1.ts:M",
		"w.ts:h() calls s1.ts:E", "svc.ts:make() calls s1.ts:User",
	})
}

// Upstream's dynamic_import carries no location.
func TestJSDynamicImportNoLocation(t *testing.T) {
	results := []Result{
		FileFromBytes("a.ts", []byte("\nexport const x = import('./b');\n")),
		FileFromBytes("b.ts", []byte("export const b = 1;\n")),
	}
	n := 0
	for _, e := range Resolve(results, []string{"a.ts", "b.ts"}).Edges {
		if e.Relation == model.RelDynamicImport {
			n++
			if e.SourceLocation != 0 {
				t.Errorf("dynamic_import has location %v", e.SourceLocation)
			}
		}
	}
	if n != 1 {
		t.Errorf("got %d dynamic_import edges, want 1", n)
	}
}
