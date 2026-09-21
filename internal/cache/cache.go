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
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/dobbo-ca/graphify-go/internal/extract"
	"github.com/dobbo-ca/graphify-go/internal/fsutil"
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
// IndexedAtNs is the wall clock captured immediately BEFORE the content was
// read; see statSigFresh for why it is needed. Entries written by an older
// graphify-go carry 0 and are treated as untrusted (one re-read each).
type StatEntry struct {
	Size        int64  `json:"size"`
	MtimeNs     int64  `json:"mtime_ns"`
	Hash        string `json:"hash"`
	IndexedAtNs int64  `json:"indexed_at_ns"`
}

// mtimeGranularityNs is the assumed filesystem mtime granularity. A stat
// signature only proves a file is unchanged when the clock that stamped its
// mtime is finer-grained than the interval between two writes — which is false
// almost everywhere: NTFS advances mtime on the ~15.6ms system tick, FAT/exFAT
// on 2s, and Linux stamps from the coarse (jiffies) clock even though ext4
// stores nanoseconds. 2s is the conservative default that covers all of them,
// and it costs nothing in practice: only files modified within the last 2s lose
// the fastpath, and those are exactly the files that changed and must be read
// anyway.
const mtimeGranularityNs = 2_000_000_000

// mtimeGranularity returns mtimeGranularityNs, overridable with
// GRAPHIFY_MTIME_GRANULARITY_MS (0 disables the guard, restoring the old
// behaviour). Read fresh on every call so tests and callers can flip it.
func mtimeGranularity() int64 {
	raw := os.Getenv("GRAPHIFY_MTIME_GRANULARITY_MS")
	if raw == "" {
		return mtimeGranularityNs
	}
	ms, err := strconv.ParseFloat(raw, 64)
	if err != nil || ms < 0 {
		return mtimeGranularityNs
	}
	return int64(ms * 1_000_000)
}

// statSigFresh reports whether prev provably describes the file's CURRENT
// content. Beyond matching (size, mtime), the entry must be racily clean in
// git's sense: the content must have been read strictly after the file's mtime
// tick had already closed. Otherwise a write landing between our read and the
// end of that tick would leave mtime (and, for a same-length edit, size)
// untouched, and the stored digest would describe content no longer on disk.
func statSigFresh(prev StatEntry, size, mtime int64) bool {
	if prev.Size != size || prev.MtimeNs != mtime {
		return false
	}
	if prev.IndexedAtNs == 0 {
		return false
	}
	return mtime+mtimeGranularity() <= prev.IndexedAtNs
}

// StatIndex maps a slash-relative file path to its stat fastpath entry.
type StatIndex map[string]StatEntry

// schema is the on-disk cache layout revision. Bump it when the shape of Entry
// or StatEntry changes so older files are discarded rather than misread.
const schema = 2

// Stamp is the cache identity for a binary: its version plus the on-disk schema
// revision. A cache file written under a different stamp is discarded, so an
// extractor fix shipped in a new binary invalidates every cached result instead
// of silently serving results the old extractors produced.
func Stamp(version string) string {
	return fmt.Sprintf("%s-s%d", version, schema)
}

// cacheFile and statFile are the stamped envelopes written to disk.
type cacheFile struct {
	V     string `json:"v"`
	Files Cache  `json:"files"`
}

type statFile struct {
	V     string    `json:"v"`
	Files StatIndex `json:"files"`
}

// Load reads a cache file. A missing, unreadable, or differently-stamped file
// returns an empty cache and no error, so callers can treat "no usable cache"
// as "re-parse everything".
func Load(path, stamp string) Cache {
	data, err := os.ReadFile(path)
	if err != nil {
		return Cache{}
	}
	var f cacheFile
	if err := json.Unmarshal(data, &f); err != nil || f.V != stamp || f.Files == nil {
		return Cache{}
	}
	return f.Files
}

// Save writes the cache as compact JSON under stamp.
func Save(path, stamp string, c Cache) error {
	data, err := json.Marshal(cacheFile{V: stamp, Files: c})
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data, 0o644)
}

// LoadStat reads the stat sidecar. A missing, unreadable, or differently-stamped
// file returns an empty index and no error, so callers fall back to reading and
// hashing every file.
func LoadStat(path, stamp string) StatIndex {
	data, err := os.ReadFile(path)
	if err != nil {
		return StatIndex{}
	}
	var f statFile
	if err := json.Unmarshal(data, &f); err != nil || f.V != stamp || f.Files == nil {
		return StatIndex{}
	}
	return f.Files
}

// SaveStat writes the stat sidecar as compact JSON under stamp.
func SaveStat(path, stamp string, s StatIndex) error {
	data, err := json.Marshal(statFile{V: stamp, Files: s})
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data, 0o644)
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
		if prevOK && statSigFresh(prev, fi.Size(), mtime) {
			return prev.Hash, prev, nil, true
		}
		now := time.Now().UnixNano()
		if b, err := os.ReadFile(absPath); err == nil {
			h := HashBytes(b)
			return h, StatEntry{Size: fi.Size(), MtimeNs: mtime, Hash: h, IndexedAtNs: now}, b, true
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
