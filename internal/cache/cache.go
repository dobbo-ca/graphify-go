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

// Entry is one file's content hash and its cached extraction result.
type Entry struct {
	Hash   string         `json:"hash"`
	Result extract.Result `json:"result"`
}

// Cache maps a slash-relative file path to its cached entry.
type Cache map[string]Entry

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

// HashBytes returns the hex SHA-256 of b.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
