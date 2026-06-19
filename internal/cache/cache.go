// Package cache persists per-file extraction results so `graphify update` can
// rebuild the graph after re-parsing only the files that changed. The expensive
// step is the tree-sitter parse; resolving and assembling the whole graph from
// cached per-file results is cheap, so caching the results (not just hashes)
// lets an incremental rebuild produce output byte-identical to a full build.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"

	"github.com/dobbo-ca/graphify-go/internal/extract"
)

// FileName is the cache file written under graphify-out alongside graph.json.
const FileName = ".graphify_cache.json"

// StatFileName is the stat sidecar written alongside FileName. It lets an
// incremental run skip re-reading and re-hashing files whose size and mtime are
// unchanged — the same trade-off make(1) makes.
const StatFileName = ".graphify_stat.json"

// Entry is one file's content hash and its cached extraction result.
type Entry struct {
	Hash   string         `json:"hash"`
	Result extract.Result `json:"result"`
}

// Cache maps a slash-relative file path to its cached entry.
type Cache map[string]Entry

// StatEntry records a file's size, modification time, and content hash so an
// unchanged file (matching size+mtime) can reuse its hash without being read.
type StatEntry struct {
	Size    int64  `json:"size"`
	MtimeNs int64  `json:"mtime_ns"`
	Hash    string `json:"hash"`
}

// StatIndex maps a slash-relative file path to its stat fastpath entry.
type StatIndex map[string]StatEntry

// Load reads a cache file. A missing or unreadable file returns an empty cache
// and no error, so callers can treat "no cache yet" as "re-parse everything".
func Load(path string) Cache {
	data, err := os.ReadFile(path)
	if err != nil {
		return Cache{}
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return Cache{}
	}
	return c
}

// Save writes the cache as compact JSON.
func Save(path string, c Cache) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadStat reads the stat sidecar. A missing or unreadable file returns an empty
// index and no error, so callers fall back to reading and hashing every file.
func LoadStat(path string) StatIndex {
	data, err := os.ReadFile(path)
	if err != nil {
		return StatIndex{}
	}
	var s StatIndex
	if err := json.Unmarshal(data, &s); err != nil {
		return StatIndex{}
	}
	return s
}

// SaveStat writes the stat sidecar as compact JSON.
func SaveStat(path string, s StatIndex) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// HashBytes returns the hex SHA-256 of b.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// HashFile returns the content hash of the file at absPath plus the StatEntry to
// persist for it, using a make(1)-style stat fastpath: when prev (the previous
// run's entry for this path, prevOK) matches the file's current size and mtime,
// its hash is reused and the file is not read; src is then nil. Otherwise the
// file is read and hashed, and src holds its bytes for the caller to parse. It
// is best-effort: a stat failure falls through to a full read. A read failure
// returns ok=false so the caller can skip the file.
func HashFile(absPath string, prev StatEntry, prevOK bool) (hash string, entry StatEntry, src []byte, ok bool) {
	if fi, err := os.Stat(absPath); err == nil {
		mtime := fi.ModTime().UnixNano()
		if prevOK && prev.Size == fi.Size() && prev.MtimeNs == mtime {
			return prev.Hash, prev, nil, true
		}
		if b, err := os.ReadFile(absPath); err == nil {
			h := HashBytes(b)
			return h, StatEntry{Size: fi.Size(), MtimeNs: mtime, Hash: h}, b, true
		}
		return "", StatEntry{}, nil, false
	}
	// stat failed — fall back to a plain read without a stat entry to store.
	b, err := os.ReadFile(absPath)
	if err != nil {
		return "", StatEntry{}, nil, false
	}
	return HashBytes(b), StatEntry{}, b, true
}
