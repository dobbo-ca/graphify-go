package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStatIndexRoundTrip checks the stat sidecar survives a save/load cycle and
// that a missing or corrupt file degrades to an empty index.
func TestStatIndexRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StatFileName)

	want := StatIndex{"a.go": {Size: 12, MtimeNs: 345, Hash: "deadbeef"}}
	if err := SaveStat(path, want); err != nil {
		t.Fatalf("SaveStat: %v", err)
	}
	got := LoadStat(path)
	if got["a.go"] != want["a.go"] {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got["a.go"], want["a.go"])
	}

	if len(LoadStat(filepath.Join(dir, "missing.json"))) != 0 {
		t.Error("LoadStat of a missing file should be empty")
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(LoadStat(path)) != 0 {
		t.Error("LoadStat of corrupt JSON should be empty")
	}
}

// TestHashFileFastpath verifies that a matching size+mtime entry reuses the
// stored hash without reading the file (src == nil), while a stat change reads
// and re-hashes.
func TestHashFileFastpath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.go")
	if err := os.WriteFile(f, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// First encounter: no prev entry → full read, hash computed, bytes returned.
	h1, e1, src1, ok := HashFile(f, StatEntry{}, false)
	if !ok || src1 == nil {
		t.Fatalf("first HashFile: ok=%v src!=nil=%v, want true/true", ok, src1 != nil)
	}
	if e1.Hash != h1 || e1.Size != int64(len("package p\n")) {
		t.Errorf("first stat entry mismatch: %+v (hash %s)", e1, h1)
	}

	// Corrupt the contents but restore the original size and mtime, so the
	// fastpath must match on stat and serve the stored hash without reading.
	fi, _ := os.Stat(f)
	if err := os.WriteFile(f, []byte("XXXXXXXXXX"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	h2, _, src2, ok := HashFile(f, e1, true)
	if !ok {
		t.Fatal("fastpath HashFile: ok=false")
	}
	if src2 != nil {
		t.Error("fastpath should not read the file (src should be nil)")
	}
	if h2 != h1 {
		t.Errorf("fastpath should reuse stored hash %s, got %s", h1, h2)
	}

	// Bump the mtime so size+mtime no longer match: now it must read and hash.
	if err := os.Chtimes(f, time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	h3, _, src3, ok := HashFile(f, e1, true)
	if !ok || src3 == nil {
		t.Fatalf("post-mtime-change HashFile: ok=%v src!=nil=%v", ok, src3 != nil)
	}
	if h3 == h1 {
		t.Error("changed file should produce a different hash than the stale entry")
	}
}

// TestHashFileMissing reports ok=false for a file that cannot be read.
func TestHashFileMissing(t *testing.T) {
	if _, _, _, ok := HashFile(filepath.Join(t.TempDir(), "nope.go"), StatEntry{}, false); ok {
		t.Error("HashFile of a missing file should return ok=false")
	}
}
