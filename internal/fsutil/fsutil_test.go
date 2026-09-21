package fsutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicReplacesAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("new")) {
		t.Fatalf("got %q, want %q", got, "new")
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("perm = %v, want 0644", fi.Mode().Perm())
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Fatalf("temp file left behind: %v", ents)
	}
}

// A concurrent reader must see either the whole old file or the whole new one.
func TestWriteFileAtomicReaderSeesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.json")
	old := bytes.Repeat([]byte("a"), 1<<20)
	fresh := bytes.Repeat([]byte("b"), 1<<20)
	if err := os.WriteFile(path, old, 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- WriteFileAtomic(path, fresh, 0o644) }()
	for i := 0; i < 200; i++ {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, old) && !bytes.Equal(got, fresh) {
			t.Fatalf("partial read of %d bytes", len(got))
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
