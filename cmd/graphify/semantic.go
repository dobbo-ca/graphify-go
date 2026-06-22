package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dobbo-ca/graphify-go/internal/model"
	"github.com/dobbo-ca/graphify-go/internal/semantic"
)

// semanticCacheFile is the content-hash cache for the semantic stage, written
// alongside graph.json so only changed notes re-pay LLM tokens on a rebuild.
const semanticCacheFile = ".graphify_semantic.json"

// semanticOpts carries the enrichment-stage settings derived from build flags.
type semanticOpts struct {
	enabled bool
	backend string
}

// proseTypes are the markdown-derived node file_types whose source documents
// are sent to the semantic backend. The markdown extractor emits one concept
// node per prose file with file_type from frontmatter `type` (default
// "document"); these are the notes the LLM reads.
var proseTypes = map[string]bool{
	"document": true,
	"note":     true,
	"learning": true,
	"decision": true,
	"adr":      true,
}

// enrich runs the opt-in semantic stage over ext and returns the additively
// enriched extraction. It picks the backend, gathers prose notes (re-reading
// their source files), runs semantic.Enrich with the persisted cache, and saves
// the updated cache. The deterministic core in ext is never mutated.
func enrich(root string, ext model.Extraction, opts semanticOpts) (model.Extraction, error) {
	ctx := context.Background()
	backend, err := newSemanticBackend(ctx, opts.backend)
	if err != nil {
		return ext, err
	}

	notes := collectNotes(root, ext)
	if len(notes) == 0 {
		return ext, nil
	}

	prev := loadSemanticCache(filepath.Join(root, "graphify-out", semanticCacheFile))
	out, newCache, err := semantic.Enrich(ctx, semantic.Config{Backend: backend}, ext, notes, prev)
	if err != nil {
		return ext, err
	}
	if err := saveSemanticCache(filepath.Join(root, "graphify-out", semanticCacheFile), newCache); err != nil {
		// A cache-write failure is non-fatal: the enrichment is already in the
		// graph; we just won't get the incremental token savings next run.
		fmt.Fprintf(os.Stderr, "  semantic: cache not saved (%v)\n", err)
	}
	fmt.Printf("semantic enrichment (%s): %d notes considered\n", backend.Name(), len(notes))
	return out, nil
}

// newSemanticBackend constructs the requested backend. Only "bedrock" is
// implemented; the interface leaves room for an internal gateway later.
func newSemanticBackend(ctx context.Context, name string) (semantic.Backend, error) {
	switch name {
	case "bedrock":
		return semantic.NewBedrockBackend(ctx, "us-east-1")
	default:
		return nil, fmt.Errorf("unknown semantic backend %q (want: bedrock)", name)
	}
}

// collectNotes turns each prose concept node in ext into a Note, re-reading its
// source file for the body. Nodes without a source file (pure concept nodes) or
// of a non-prose file_type are skipped.
func collectNotes(root string, ext model.Extraction) []semantic.Note {
	var notes []semantic.Note
	for _, n := range ext.Nodes {
		if n.SourceFile == "" || !proseTypes[n.FileType] {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, n.SourceFile))
		if err != nil {
			continue
		}
		notes = append(notes, semantic.Note{ID: n.ID, File: n.SourceFile, Content: string(body)})
	}
	return notes
}

func loadSemanticCache(path string) semantic.Cache {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c semantic.Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	return c
}

func saveSemanticCache(path string, c semantic.Cache) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
