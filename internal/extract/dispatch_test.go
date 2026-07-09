package extract

import "testing"

// A .mts file routes through the TypeScript extractor: the TS-only `interface`
// and type-annotated function parse (the plain JS grammar would not), and their
// definitions surface as nodes.
func TestFileFromBytesMtsAsTypeScript(t *testing.T) {
	src := []byte("export interface Widget { id: number; }\n" +
		"export function build(w: Widget): string {\n" +
		"  return w.id.toString();\n" +
		"}\n")
	res := FileFromBytes("web/util.mts", src)

	labels := map[string]bool{}
	for _, n := range res.Nodes {
		labels[n.Label] = true
	}
	for _, want := range []string{"Widget", "build()"} {
		if !labels[want] {
			t.Errorf("missing node label %q (mts not extracted as TypeScript): %v", want, labels)
		}
	}
}

// An extensionless script whose shebang names bash routes through the bash
// extractor: its function definition surfaces as a `name()` node.
func TestFileFromBytesShebangBash(t *testing.T) {
	src := []byte("#!/usr/bin/env bash\n" +
		"greet() {\n" +
		"  echo hi\n" +
		"}\n")
	res := FileFromBytes("scripts/deploy", src)

	found := false
	for _, n := range res.Nodes {
		if n.Label == "greet()" {
			found = true
		}
	}
	if !found {
		t.Error("expected greet() node from extensionless bash shebang script")
	}
}
