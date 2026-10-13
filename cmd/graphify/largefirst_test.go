package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The large-file lane changes the parse order only: results come back in
// file order with the same content.
func TestAssembleLargeFirst(t *testing.T) {
	root := t.TempDir()
	body := func(n int) string {
		return "package p\n\nfunc F() {}\n" + strings.Repeat("// pad\n", n/7)
	}
	files := []string{"a.go", "big1.go", "b.go", "big2.go", "gone.go", "c.go"}
	sizes := []int{10, bigFile, 20, bigFile + 4096, 0, 30}
	for i, f := range files {
		if f == "gone.go" {
			continue
		}
		if err := os.WriteFile(filepath.Join(root, f), []byte(body(sizes[i])), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := largeFirst(root, files), []int{3, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("largeFirst = %v, want %v", got, want)
	}
	results, newCache, _, stats := assemble(root, files, nil, nil)
	if stats.parsed != 5 || len(newCache) != 5 {
		t.Fatalf("parsed %d, cache %d, want 5 and 5", stats.parsed, len(newCache))
	}
	want := []string{"a.go", "big1.go", "b.go", "big2.go", "c.go"}
	for i, f := range want {
		if !reflect.DeepEqual(results[i], newCache[f].Result) {
			t.Fatalf("result %d is not the result of %s", i, f)
		}
	}
	if reflect.DeepEqual(results[0], results[1]) {
		t.Fatal("results of different files are equal")
	}
}
