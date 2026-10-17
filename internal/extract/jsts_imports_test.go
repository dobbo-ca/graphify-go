package extract

import "testing"

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
