package extract

import (
	"os"
	"path/filepath"
	"testing"
)

// A JS symbol whose name normalizes to an empty ID (a minified `$`) must not
// produce a node; it would collapse to the bare file stem and be pure noise
// (#1899). A real function alongside it must still be extracted.
func TestExtractJSSkipsDegenerateName(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "vendor.js"),
		[]byte("function $(){ return 1 }\nfunction real(){ return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := File(root, "vendor.js")
	if err != nil {
		t.Fatalf("File(vendor.js): %v", err)
	}
	labels := labelSet(r)
	if labels["$()"] {
		t.Errorf("degenerate `$` function must not produce a node, got labels %v", labels)
	}
	if !labels["real()"] {
		t.Errorf("real function should still be extracted, got %v", labels)
	}
}

// A JSONC `"//"` comment key normalizes to an empty ID and must not produce a
// node; real config keys alongside it must still be extracted (#1899).
func TestExtractJSONSkipsCommentKey(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"),
		[]byte(`{"//": "a note", "compilerOptions": {"strict": true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := File(root, "tsconfig.json")
	if err != nil {
		t.Fatalf("File(tsconfig.json): %v", err)
	}
	labels := labelSet(r)
	if labels["//"] {
		t.Errorf("JSONC `//` comment key must not produce a node, got labels %v", labels)
	}
	if !labels["compilerOptions"] {
		t.Errorf("real config key should still be extracted, got %v", labels)
	}
}
