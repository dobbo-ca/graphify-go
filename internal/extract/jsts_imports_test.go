package extract

import (
	"slices"
	"strings"
	"testing"
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
		// bound by the alias, not the tie-break: render() is defined twice
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
	// no edge for a builtin name, or for a new at the file level or in its closure
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

	// the closure of the file wins over a function, for the same target
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
