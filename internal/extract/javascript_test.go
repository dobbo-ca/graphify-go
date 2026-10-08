package extract

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
)

// A `// NOTE:` comment becomes a rationale node with an edge to the file, and an
// `ADR-0001` reference in a comment becomes a normalized doc_ref node cited by
// the file.
func TestExtractJSRationale(t *testing.T) {
	src := []byte("// NOTE: debounce to avoid thrash\n" +
		"// See ADR-0001 for the retry policy.\n" +
		"export const boot = () => {\n" +
		"  return 1;\n" +
		"};\n")
	res := FileFromBytes("web/app.js", src)

	fileID := idutil.MakeID("web/app.js")

	label := map[string]string{}
	ftype := map[string]string{}
	for _, n := range res.Nodes {
		label[n.ID] = n.Label
		ftype[n.ID] = n.FileType
	}

	// NOTE comment -> rationale node with a rationale_for edge to the file.
	rationaleOK := false
	for _, e := range res.Edges {
		if e.Relation == "rationale_for" && e.Target == fileID &&
			ftype[e.Source] == "rationale" && strings.HasPrefix(label[e.Source], "NOTE: debounce") {
			rationaleOK = true
		}
	}
	if !rationaleOK {
		t.Errorf("expected rationale_for edge from NOTE comment to file %s", fileID)
	}

	// ADR-0001 comment -> normalized doc_ref node + cites edge from the file.
	docRefID := idutil.MakeID("docref", "ADR-0001")
	if ftype[docRefID] != "doc_ref" || label[docRefID] != "ADR-0001" {
		t.Errorf("expected doc_ref node %s labeled ADR-0001, got type=%q label=%q",
			docRefID, ftype[docRefID], label[docRefID])
	}
	citesOK := false
	for _, e := range res.Edges {
		if e.Relation == "cites" && e.Source == fileID && e.Target == docRefID {
			citesOK = true
		}
	}
	if !citesOK {
		t.Errorf("expected cites edge %s -> %s", fileID, docRefID)
	}
}

// CommonJS require, re-exports and dynamic import each link to the target file.
func TestJSImportFormsResolveToFiles(t *testing.T) {
	cases := []struct{ name, src string }{
		{"require", "const lib = require('./lib');\n"},
		{"nested require", "function f() {\n  return require(\"./lib\").x;\n}\n"},
		{"export star", "export * from './lib';\n"},
		{"export named", "export { thing } from './lib';\n"},
		{"dynamic import", "async function f() {\n  const m = await import('./lib');\n}\n"},
	}
	for _, c := range cases {
		got := importEdges(map[string]string{"src/a.ts": c.src, "src/lib.ts": "export const thing = 1;\n"}, nil)
		if want := []string{"src_a_ts imports_from src_lib_ts"}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", c.name, got, want)
		}
	}
	// Not imports: a computed spec, a method named require, a plain export.
	src := "const a = require(name);\nconst b = obj.require('./lib');\nexport { b };\n"
	if got := importEdges(map[string]string{"src/a.js": src, "src/lib.js": ""}, nil); len(got) != 0 {
		t.Errorf("non-imports: got %v, want none", got)
	}
}

// `export type { T } from` is type-only, like `import type`.
func TestJSReExportTypeOnly(t *testing.T) {
	for src, want := range map[string]bool{
		"export type { T } from './lib';\n": true,
		"export { T } from './lib';\n":      false,
	} {
		imps := FileFromBytes("a.ts", []byte(src)).Imps
		if len(imps) != 1 || imps[0].TypeOnly != want {
			t.Errorf("%q: got %+v, want one import with TypeOnly=%v", src, imps, want)
		}
	}
}
