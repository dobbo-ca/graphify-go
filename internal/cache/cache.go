// Package cache persists per-file extraction results so `graphify update` can
// rebuild the graph after re-parsing only the files that changed. The expensive
// step is the tree-sitter parse; resolving and assembling the whole graph from
// cached per-file results is cheap, so caching the results (not just hashes)
// lets an incremental rebuild produce output byte-identical to a full build.
package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dobbo-ca/graphify-go/internal/extract"
	"github.com/dobbo-ca/graphify-go/internal/fsutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
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
const schema = 17

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
	if f, err := os.Open(path); err == nil {
		c := loadSplit(f, stamp)
		f.Close()
		if c != nil {
			return c
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Cache{}
	}
	var f cacheFile
	if err := json.Unmarshal(data, &f); err != nil || f.V != stamp || f.Files == nil {
		return Cache{}
	}
	dropBadCalls(f.Files)
	return f.Files
}

// Stale reports whether the file at path is a cache written under another
// stamp, which Load drops. It reads only the head of the file. A missing file
// and one that is not a cache are not stale.
func Stale(path, stamp string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	want, _ := json.Marshal(stamp)
	want = append([]byte(`{"v":`), want...)
	got := make([]byte, len(want))
	n, _ := io.ReadFull(f, got)
	return bytes.HasPrefix(got[:n], []byte(`{"v":"`)) && !bytes.Equal(got[:n], want)
}

// dropBadCalls removes the entries whose calls number a text that is not
// there, so those files are parsed again.
func dropBadCalls(c Cache) {
	for k, e := range c {
		if !e.Result.CallsOK() {
			delete(c, k)
		}
	}
}

// loadSplit decodes a cache file in pieces on several goroutines: the
// single decode was the longest serial step of an update. It cuts the files
// object where an entry looks to end and decodes each piece as an object of
// its own. The cuts are guesses, but pieces that all decode hold between
// them exactly the members of the whole object, so a bad cut can only make
// a piece fail; then, or when the file is not in writeCache's layout, it
// returns nil and the caller decodes the document whole.
//
// It reads r as it goes: holding the file whole (hundreds of MB) beside the
// decoded entries doubled the heap an update starts from.
func loadSplit(r io.Reader, stamp string) Cache {
	head, err := json.Marshal(cacheFile{V: stamp})
	if err != nil {
		return nil
	}
	head = append(head[:len(head)-len("null}")], '{')
	got := make([]byte, len(head))
	if _, err := io.ReadFull(r, got); err != nil || !bytes.Equal(got, head) {
		return nil
	}
	// Small pieces: bigger ones raised the peak.
	const want = 256 << 10
	type piece struct {
		i int
		b []byte
	}
	var (
		mu    sync.Mutex
		parts []Cache
		bad   atomic.Bool
		wg    sync.WaitGroup
	)
	work := make(chan piece)
	free := make(chan []byte, 16)
	// Past 8 decoders the wall stops falling and the heap peak keeps rising.
	for range min(runtime.GOMAXPROCS(0), 8) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range work {
				var c Cache
				if json.Unmarshal(p.b, &c) != nil || c == nil {
					bad.Store(true)
				}
				dropBadCalls(c)
				mu.Lock()
				parts[p.i] = c
				mu.Unlock()
				select {
				case free <- p.b[:0]:
				default:
				}
			}
		}()
	}
	send := func(b []byte) {
		mu.Lock()
		i := len(parts)
		parts = append(parts, nil)
		mu.Unlock()
		work <- piece{i, b}
	}
	// pend is always an object of its own so far: '{' and whole entries.
	pend := append(make([]byte, 0, 2*want), '{')
	eof := false
	for !eof && !bad.Load() {
		// At least double on a miss, so one huge entry is not rescanned per read.
		n := max(want, len(pend))
		pend = slices.Grow(pend, n)
		m, err := io.ReadFull(r, pend[len(pend):len(pend)+n])
		pend = pend[:len(pend)+m]
		eof = err != nil
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			bad.Store(true)
		}
		for len(pend) > want {
			cut := entryEnd(pend, want)
			if cut < 0 {
				break
			}
			var next []byte
			select {
			case next = <-free:
			default:
				next = make([]byte, 0, 2*want)
			}
			next = append(append(next, '{'), pend[cut+1:]...)
			pend[cut] = '}'
			send(pend[:cut+1])
			pend = next
		}
	}
	// What is left ends the files object and the document.
	if bytes.HasSuffix(pend, []byte("}}}")) || string(pend) == "{}}" {
		send(pend[:len(pend)-1])
	} else {
		bad.Store(true)
	}
	close(work)
	wg.Wait()
	if bad.Load() {
		return nil
	}
	c := Cache{}
	for _, p := range parts {
		// In file order, so a repeated key resolves as in a whole decode.
		maps.Copy(c, p)
	}
	return c
}

// entryEnd returns the offset, at or after from, of a comma that looks to
// separate two entries: `}},"<key>":{"hash":"`. -1 when there is none.
func entryEnd(b []byte, from int) int {
	for {
		i := bytes.Index(b[from:], []byte(`}},"`))
		if i < 0 {
			return -1
		}
		cut := from + i + 2
		k := cut + 2
		for k < len(b) && b[k] != '"' {
			if b[k] == '\\' {
				k++
			}
			k++
		}
		if k < len(b) && bytes.HasPrefix(b[k:], []byte(`":{"hash":"`)) {
			return cut
		}
		from = cut
	}
}

// Save writes the cache as compact JSON under stamp.
func Save(path, stamp string, c Cache) error {
	return fsutil.WriteAtomic(path, 0o644, func(w io.Writer) error {
		return writeCache(w, stamp, c, false)
	})
}

// SaveDrain is Save, but it deletes each entry from c once it is written, so
// a caller that holds the results nowhere else gets them freed as the file is
// encoded. c is left empty, or partly emptied on an error.
func SaveDrain(path, stamp string, c Cache) error {
	return fsutil.WriteAtomic(path, 0o644, func(w io.Writer) error {
		return writeCache(w, stamp, c, true)
	})
}

// writeCache streams the bytes json.Marshal(cacheFile{V: stamp, Files: c})
// would produce, entry by entry in the encoder's sorted key order. The
// file runs to hundreds of MB on large repos, and marshalling it whole made
// that buffer and its growth copies the process peak.
func writeCache(w io.Writer, stamp string, c Cache, drain bool) error {
	head, err := json.Marshal(cacheFile{V: stamp})
	if err != nil {
		return err
	}
	if c == nil {
		_, err := w.Write(head)
		return err
	}
	// head ends in `"files":null}`; the entries replace the null.
	if _, err := w.Write(head[:len(head)-len("null}")]); err != nil {
		return err
	}
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// One buffer, one goroutine: the direct encoder outruns the writes.
	var b []byte
	sep := byte('{')
	for _, k := range keys {
		e := c[k]
		if drain {
			delete(c, k)
		}
		b = model.AppendJSONString(append(b[:0], sep), k)
		b = model.AppendJSONString(append(b, `:{"hash":`...), e.Hash)
		if b, err = e.Result.AppendJSON(append(b, `,"result":`...)); err != nil {
			return err
		}
		b = append(b, '}')
		if _, err = w.Write(b); err != nil {
			return err
		}
		sep = ','
	}
	end := "}}"
	if len(keys) == 0 {
		end = "{}}"
	}
	_, err = io.WriteString(w, end)
	return err
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

// ioSlots caps the goroutines inside Stat and ReadFile: past 4 the kernel
// spends more time contending on path lookups than the extra readers save.
var ioSlots = make(chan struct{}, 4)

// HashFile returns the content hash of the file at absPath plus the StatEntry to
// persist for it, using a make(1)-style stat fastpath: when prev (the previous
// run's entry for this path, prevOK) matches the file's current size and mtime,
// its hash is reused and the file is not read; src is then nil. Otherwise the
// file is read and hashed, and src holds its bytes for the caller to parse. It
// is best-effort: a stat failure falls through to a full read. A read failure
// returns ok=false so the caller can skip the file.
func HashFile(absPath string, prev StatEntry, prevOK bool) (hash string, entry StatEntry, src []byte, ok bool) {
	ioSlots <- struct{}{}
	fi, statErr := os.Stat(absPath)
	var mtime, now int64
	if statErr == nil {
		mtime = fi.ModTime().UnixNano()
		if prevOK && statSigFresh(prev, fi.Size(), mtime) {
			<-ioSlots
			return prev.Hash, prev, nil, true
		}
		now = time.Now().UnixNano()
	}
	b, err := os.ReadFile(absPath)
	<-ioSlots
	if err != nil {
		return "", StatEntry{}, nil, false
	}
	h := HashBytes(b)
	if statErr != nil {
		// stat failed: no stat entry to store.
		return h, StatEntry{}, b, true
	}
	return h, StatEntry{Size: fi.Size(), MtimeNs: mtime, Hash: h, IndexedAtNs: now}, b, true
}
