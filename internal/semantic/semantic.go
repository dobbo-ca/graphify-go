// Package semantic is the opt-in LLM enrichment stage. It runs between
// extract.Resolve and cluster.Cluster: an LLM reads each prose note and emits
// concept/rationale nodes plus cites / conceptually_related_to /
// semantically_similar_to edges, so communities reflect concepts and otherwise
// isolated notes gain edges into their real cluster.
//
// Enrichment is ADDITIVE ONLY. It never mutates or removes a deterministic
// AST/markdown node or edge: deterministic nodes win on id collisions, and
// every emitted edge is forced to INFERRED|AMBIGUOUS confidence so the
// deterministic core stays byte-identical when the stage is off. A
// content-hash cache means only changed notes re-pay tokens, and the
// sensitive-file skip plus a size cap run before any content reaches a backend.
package semantic

import (
	"context"
	"fmt"
	"os"

	"github.com/dobbo-ca/graphify-go/internal/cache"
	"github.com/dobbo-ca/graphify-go/internal/detect"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// ConceptType is the file_type for semantic concept nodes. It is distinct from
// extract's "concept" (external-dependency) nodes so the report's God-Node and
// Surprising-Connection analysis can recognise LLM-derived concepts as real
// abstractions without changing how external-dependency nodes are treated — and
// so corpora built without --semantic never produce this type, keeping their
// output byte-identical.
const ConceptType = "semantic_concept"

// RationaleType is the file_type for rationale nodes a backend may emit (a short
// explanation of why two things relate). Analysed as a real entity, like
// ConceptType.
const RationaleType = "rationale"

// MaxNoteBytes caps the prose sent to a backend per note. Notes larger than
// this are skipped (mirroring the extract path's size discipline) so a single
// pathological file cannot blow up token cost.
const MaxNoteBytes = 200 * 1024

// validRelations are the only edge relations the semantic stage emits. An edge
// a backend returns with any other relation is dropped.
var validRelations = map[string]bool{
	"cites":                   true,
	"conceptually_related_to": true,
	"semantically_similar_to": true,
}

// Note is one prose document handed to the backend: its graph node ID (so
// emitted edges can attach to it), its repo-relative path, and its body.
type Note struct {
	ID      string
	File    string
	Content string
}

// Backend is an LLM concept-extraction provider. Implementations read one
// note's prose and return the additive concept/rationale nodes and the edges
// (from the note, into concepts or other nodes) that the model inferred.
// Designing this as an interface lets an internal OpenAI-compatible gateway be
// added later without touching the enrichment plumbing.
type Backend interface {
	// Name identifies the backend (e.g. "bedrock"), for diagnostics.
	Name() string
	// Extract returns nodes and edges inferred from one note. A non-nil error
	// is treated as a per-note failure: that note contributes nothing, but the
	// run continues.
	Extract(ctx context.Context, n Note) (nodes []model.Node, edges []model.Edge, err error)
}

// Config carries the knobs for one enrichment run.
type Config struct {
	Backend Backend
}

// Cache maps a note's graph ID to its content hash and the semantic fragments
// last extracted from it, so an unchanged note reuses its result instead of
// re-paying tokens. It mirrors the internal/cache content-hash pattern.
type Cache map[string]Entry

// Entry is one note's cached semantic extraction, keyed in Cache by note ID.
type Entry struct {
	Hash  string       `json:"hash"`
	Nodes []model.Node `json:"nodes"`
	Edges []model.Edge `json:"edges"`
}

// Enrich runs the semantic stage over notes and merges the result additively
// into base. It returns the enriched extraction and the cache to persist.
//
// prev is the previous run's cache (nil for a cold run); notes whose content
// hash matches prev reuse their cached fragments and the backend is not called.
// Sensitive files and notes over MaxNoteBytes are skipped before any content
// reaches the backend. A per-note backend error is logged and skipped — it
// never aborts the run or touches the deterministic core.
func Enrich(ctx context.Context, cfg Config, base model.Extraction, notes []Note, prev Cache) (model.Extraction, Cache, error) {
	if cfg.Backend == nil {
		return base, prev, fmt.Errorf("semantic: no backend configured")
	}

	// IDs that already exist in the deterministic core. Deterministic nodes win
	// on collision (last-write-wins is reserved for the core itself), and an
	// emitted edge may only target an id that exists in the core or in a
	// concept node emitted this run.
	known := make(map[string]bool, len(base.Nodes))
	for _, n := range base.Nodes {
		known[n.ID] = true
	}

	newCache := make(Cache, len(notes))
	var addNodes []model.Node
	var addEdges []model.Edge
	// Track concept-node IDs emitted this run so edges to them resolve, and so
	// the same concept isn't appended twice.
	emitted := map[string]bool{}

	for _, note := range notes {
		if detect.IsSensitive(note.File) {
			continue
		}
		if len(note.Content) > MaxNoteBytes {
			continue
		}

		hash := cache.HashBytes([]byte(note.Content))
		var nodes []model.Node
		var edges []model.Edge

		if prev != nil {
			if e, ok := prev[note.ID]; ok && e.Hash == hash {
				nodes, edges = e.Nodes, e.Edges
				newCache[note.ID] = e
				addNodes, addEdges, emitted = collect(addNodes, addEdges, emitted, known, note, nodes, edges)
				continue
			}
		}

		ns, es, err := cfg.Backend.Extract(ctx, note)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  semantic: skipped %s (%v)\n", note.File, err)
			continue
		}
		nodes, edges = sanitize(note, ns, es)
		newCache[note.ID] = Entry{Hash: hash, Nodes: nodes, Edges: edges}
		addNodes, addEdges, emitted = collect(addNodes, addEdges, emitted, known, note, nodes, edges)
	}

	out := model.Extraction{
		Nodes: append(append([]model.Node(nil), base.Nodes...), addNodes...),
		Edges: append(append([]model.Edge(nil), base.Edges...), addEdges...),
	}
	return out, newCache, nil
}

// sanitize coerces a backend's raw output into safe semantic fragments: every
// emitted node is a concept/rationale node (file_type forced so it can never
// masquerade as a code node), and every edge is from the note, carries a valid
// semantic relation, and an INFERRED|AMBIGUOUS confidence. Anything else is
// dropped. Endpoint existence is enforced later in collect, once the full set
// of emitted concept ids is known.
func sanitize(note Note, nodes []model.Node, edges []model.Edge) ([]model.Node, []model.Edge) {
	var outN []model.Node
	for _, n := range nodes {
		if n.ID == "" {
			continue
		}
		if n.FileType != RationaleType {
			n.FileType = ConceptType
		}
		// Concept/rationale nodes are model inferences, not file-located facts —
		// strip any source provenance a backend invented (ComputedName, which can
		// carry the model's rationale text, is kept).
		n.SourceFile, n.SourceLocation = "", ""
		outN = append(outN, n)
	}
	var outE []model.Edge
	for _, e := range edges {
		if !validRelations[e.Relation] {
			continue
		}
		if e.Source != note.ID { // edges originate from the note being read
			continue
		}
		if e.Confidence != "AMBIGUOUS" {
			e.Confidence = "INFERRED"
		}
		// Strip any source-file provenance a backend invented — these are
		// model inferences, not file-located facts.
		e.SourceFile, e.SourceLocation = "", ""
		outE = append(outE, e)
	}
	return outN, outE
}

// collect folds one note's sanitized fragments into the run accumulators. It
// appends each concept node once (never overwriting a deterministic node), then
// keeps only edges whose target resolves to a known core node or a concept node
// emitted this run — dropping danglers so the additive layer can't introduce a
// phantom endpoint.
func collect(addNodes []model.Node, addEdges []model.Edge, emitted, known map[string]bool, note Note, nodes []model.Node, edges []model.Edge) ([]model.Node, []model.Edge, map[string]bool) {
	for _, n := range nodes {
		if known[n.ID] || emitted[n.ID] {
			continue // deterministic node wins; concept already added
		}
		emitted[n.ID] = true
		addNodes = append(addNodes, n)
	}
	for _, e := range edges {
		if !known[e.Target] && !emitted[e.Target] {
			continue // dangling — target is neither core nor an emitted concept
		}
		addEdges = append(addEdges, e)
	}
	return addNodes, addEdges, emitted
}
