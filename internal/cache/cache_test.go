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
	if err := SaveStat(path, Stamp("v1"), want); err != nil {
		t.Fatalf("SaveStat: %v", err)
	}
	got := LoadStat(path, Stamp("v1"))
	if got["a.go"] != want["a.go"] {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got["a.go"], want["a.go"])
	}

	if len(LoadStat(filepath.Join(dir, "missing.json"), Stamp("v1"))) != 0 {
		t.Error("LoadStat of a missing file should be empty")
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(LoadStat(path, Stamp("v1"))) != 0 {
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
	// Age the file past the mtime granularity so its stat signature can be
	// trusted; a just-written file is racily clean and never takes the fastpath.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(f, old, old); err != nil {
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

// TestHashFileRacilyClean covers the racily-clean hole: a same-length edit
// inside the file's mtime tick leaves size and mtime untouched, so the stat
// signature alone cannot prove the cached hash still describes the content.
func TestHashFileRacilyClean(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.go")
	if err := os.WriteFile(f, []byte("func AAAA() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h1, e1, _, ok := HashFile(f, StatEntry{}, false)
	if !ok {
		t.Fatal("first HashFile: ok=false")
	}

	// Same-length rewrite, mtime restored: stat signature is identical.
	fi, _ := os.Stat(f)
	if err := os.WriteFile(f, []byte("func BBBB() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	h2, _, src2, ok := HashFile(f, e1, true)
	if !ok || src2 == nil {
		t.Fatalf("racily-clean HashFile: ok=%v src!=nil=%v, want true/true", ok, src2 != nil)
	}
	if h2 == h1 {
		t.Error("same-size edit within the mtime tick must produce a changed hash")
	}

	// GRAPHIFY_MTIME_GRANULARITY_MS=0 disables the guard, restoring the old
	// (unsafe) fastpath.
	t.Setenv("GRAPHIFY_MTIME_GRANULARITY_MS", "0")
	if _, _, src3, ok := HashFile(f, e1, true); !ok || src3 != nil {
		t.Errorf("guard disabled: ok=%v src!=nil=%v, want true/false", ok, src3 != nil)
	}
}

// TestHashFileLegacyEntry checks a sidecar entry from an older graphify-go
// (no indexed_at_ns) is distrusted once and re-read.
func TestHashFileLegacyEntry(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.go")
	if err := os.WriteFile(f, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(f)
	legacy := StatEntry{Size: fi.Size(), MtimeNs: fi.ModTime().UnixNano(), Hash: "stale"}
	h, entry, src, ok := HashFile(f, legacy, true)
	if !ok || src == nil {
		t.Fatalf("legacy entry: ok=%v src!=nil=%v, want true/true", ok, src != nil)
	}
	if h == "stale" {
		t.Error("legacy entry without indexed_at_ns must not be trusted")
	}
	if entry.IndexedAtNs == 0 {
		t.Error("rewritten entry should carry indexed_at_ns")
	}
}

// TestStampMismatchDiscards checks that a cache and stat sidecar written by one
// binary version are ignored by another, so an extractor fix shipped in a new
// build invalidates every cached result instead of being served stale ones.
func TestStampMismatchDiscards(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, FileName)
	statPath := filepath.Join(dir, StatFileName)

	want := Cache{"a.go": {Hash: "abc"}}
	wantStat := StatIndex{"a.go": {Size: 1, MtimeNs: 2, Hash: "abc"}}
	if err := Save(cachePath, Stamp("v1.0.0"), want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := SaveStat(statPath, Stamp("v1.0.0"), wantStat); err != nil {
		t.Fatalf("SaveStat: %v", err)
	}

	// Same stamp: round-trips.
	if got := Load(cachePath, Stamp("v1.0.0")); got["a.go"].Hash != "abc" {
		t.Errorf("same-stamp Load: got %+v, want the saved entry", got)
	}
	if got := LoadStat(statPath, Stamp("v1.0.0")); got["a.go"] != wantStat["a.go"] {
		t.Errorf("same-stamp LoadStat: got %+v, want %+v", got["a.go"], wantStat["a.go"])
	}

	// Different binary version: both are discarded.
	if got := Load(cachePath, Stamp("v1.1.0")); len(got) != 0 {
		t.Errorf("mismatched-stamp Load should be empty, got %+v", got)
	}
	if got := LoadStat(statPath, Stamp("v1.1.0")); len(got) != 0 {
		t.Errorf("mismatched-stamp LoadStat should be empty, got %+v", got)
	}

	// An unstamped (pre-upgrade) file is discarded too.
	if err := os.WriteFile(cachePath, []byte(`{"a.go":{"hash":"abc"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(cachePath, Stamp("v1.0.0")); len(got) != 0 {
		t.Errorf("unstamped Load should be empty, got %+v", got)
	}
}

// TestSchemaBumpDiscardsPreFeatureCache reproduces the bug where a warm cache
// from a pre-feature binary was reused verbatim: version is "dev" for every
// locally built binary, so only the schema const distinguishes a stale entry.
// A cache written under the old schema-1 stamp must be discarded even though
// the version string ("dev") is unchanged.
func TestSchemaBumpDiscardsPreFeatureCache(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, FileName)

	preFeatureStamp := "dev-s1"
	if err := Save(cachePath, preFeatureStamp, Cache{"main.tf": {Hash: "abc"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := Load(cachePath, Stamp("dev")); len(got) != 0 {
		t.Errorf("pre-feature schema cache should be discarded under current stamp, got %+v", got)
	}
}
