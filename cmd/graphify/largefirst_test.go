package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dobbo-ca/graphify-go/internal/extract"
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

// Any two of these files are over bigInFlight, so they must parse one at a
// time however many workers reach the large-file lane.
func TestAssembleLargeFilesInFlight(t *testing.T) {
	root := t.TempDir()
	files := []string{"big1.go", "big2.go", "big3.go"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(root, f), make([]byte, bigInFlight/2+1), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var held, peak int
	defer func(f func(string, []byte) extract.Result) { parseFile = f }(parseFile)
	parseFile = func(_ string, src []byte) extract.Result {
		mu.Lock()
		held += len(src)
		peak = max(peak, held)
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		held -= len(src)
		mu.Unlock()
		return extract.Result{}
	}
	if _, _, _, stats := assemble(root, files, nil, nil); stats.parsed != 3 {
		t.Fatalf("parsed %d, want 3", stats.parsed)
	}
	if peak > bigInFlight {
		t.Fatalf("%d bytes of large files in flight, limit %d", peak, bigInFlight)
	}
}
