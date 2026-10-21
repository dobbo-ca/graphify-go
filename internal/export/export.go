// Package export writes the assembled graph to disk. graph.json is the primary
// artifact (committed by CI, read by the query commands and the Claude skill);
// it uses the same NetworkX node-link shape as the Python original so existing
// tooling keeps working.
package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/fsutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
	"github.com/dobbo-ca/graphify-go/internal/security"
)

var confidenceScore = map[string]float64{"EXTRACTED": 1.0, "INFERRED": 0.5, "AMBIGUOUS": 0.2}

// ErrGraphShrink is returned by ToJSON when writing would replace an existing,
// larger graph.json with one that has fewer nodes and force is false. The guard
// exists to avoid silently losing nodes (#479) — e.g. missing chunk files from a
// prior session, or a dedup pass that collapsed same-named symbols on an --update.
var ErrGraphShrink = errors.New("refusing to overwrite graph.json with a smaller graph")

// ErrGraphUnverifiable is returned by ToJSON when a non-empty existing graph.json
// cannot be parsed to verify the new graph is not a silent shrink. Fail SAFE:
// refuse rather than clobber a possibly-good graph over a transient read/parse
// failure. Pass force to override.
var ErrGraphUnverifiable = errors.New("refusing to overwrite unparseable graph.json")

type jsonNode struct {
	ID             string            `json:"id"`
	Label          string            `json:"label"`
	FileType       string            `json:"file_type"`
	SourceFile     string            `json:"source_file"`
	SourceLocation string            `json:"source_location,omitempty"`
	Community      *int              `json:"community"`
	NormLabel      string            `json:"norm_label"`
	ComputedName   string            `json:"computed_name,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
}

type jsonLink struct {
	Source          string    `json:"source"`
	Target          string    `json:"target"`
	Relation        string    `json:"relation"`
	Confidence      string    `json:"confidence"`
	Deferred        bool      `json:"deferred,omitempty"`
	SourceFile      string    `json:"source_file,omitempty"`
	SourceLocation  model.Loc `json:"source_location,omitempty"`
	Weight          float64   `json:"weight,omitempty"`
	ConfidenceScore float64   `json:"confidence_score"`
}

// graphAttrs holds the graph-level attributes stored under graph.json's "graph"
// key (networkx's G.graph dict). It records corpus coverage: how many files the
// walk saw that no extractor handles, and which extensions they were.
type graphAttrs struct {
	UnclassifiedFiles int            `json:"unclassified_files"`
	UnclassifiedExts  map[string]int `json:"unclassified_extensions,omitempty"`
}

type jsonGraph struct {
	Directed      bool       `json:"directed"`
	Multigraph    bool       `json:"multigraph"`
	Graph         graphAttrs `json:"graph"`
	Nodes         []jsonNode `json:"nodes"`
	Links         []jsonLink `json:"links"`
	Hyperedges    []any      `json:"hyperedges"`
	BuiltAtCommit string     `json:"built_at_commit,omitempty"`
}

// ToJSON writes the graph to path in NetworkX node-link format with per-node
// community/norm_label and per-link confidence_score, plus the commit it was
// built from (for staleness checks).
//
// Unless force is set, ToJSON refuses to overwrite an existing, non-empty
// graph.json when doing so would drop nodes: it returns ErrGraphShrink when the
// new graph has fewer nodes than the one on disk, or ErrGraphUnverifiable when
// the existing file cannot be parsed to make that comparison. In both cases the
// file on disk is left untouched. An absent or empty target is written normally.
func ToJSON(g *model.Graph, communities map[int][]string, path, builtAtCommit string, force bool) error {
	if !force {
		if err := guardNoShrink(path, g.NumNodes()); err != nil {
			return err
		}
	}

	return fsutil.WriteAtomic(path, 0o644, func(w io.Writer) error {
		return writeJSON(w, g, communities, builtAtCommit)
	})
}

// jsonChunk is how many nodes or links one goroutine encodes at a time. A
// variable so tests can force chunk boundaries. Small on purpose: at 4096 the
// per-CPU buffers ran to megabytes each and set the process peak.
var jsonChunk = 512

// envelopeSplit is where the node and link arrays sit in the indented envelope
// when both are nil. Top-level keys are the only lines at this indent, and a
// quote inside any string is escaped, so it matches exactly once.
const envelopeSplit = "\n  \"nodes\": null,\n  \"links\": null,\n"

// writeJSON streams the document that json.MarshalIndent(jsonGraph, "", "  ")
// would produce, byte for byte, without holding it: graph.json runs to hundreds
// of MB on large repos and the whole-document marshal set the process peak.
func writeJSON(w io.Writer, g *model.Graph, communities map[int][]string, builtAtCommit string) error {
	nc := cluster.NodeCommunity(communities)
	var out jsonGraph
	out.Hyperedges = []any{}
	out.BuiltAtCommit = builtAtCommit
	for ext, n := range g.Unclassified {
		if out.Graph.UnclassifiedExts == nil {
			out.Graph.UnclassifiedExts = map[string]int{}
		}
		out.Graph.UnclassifiedExts[ext] = n
		out.Graph.UnclassifiedFiles += n
	}
	env, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	head, tail, ok := bytes.Cut(env, []byte(envelopeSplit))
	if !ok {
		return errors.New("export: graph.json envelope has an unexpected shape")
	}

	if _, err := w.Write(head); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\n  \"nodes\": "); err != nil {
		return err
	}
	ids := g.NodeIDs()
	if err := writeChunks(w, len(ids), func(b []byte, i int) ([]byte, error) {
		return appendJSONNode(b, toJSONNode(g.Nodes[ids[i]], nc))
	}); err != nil {
		return err
	}
	if _, err := io.WriteString(w, ",\n  \"links\": "); err != nil {
		return err
	}
	edges := g.Edges()
	if err := writeChunks(w, len(edges), func(b []byte, i int) ([]byte, error) {
		return appendJSONLink(b, toJSONLink(edges[i]))
	}); err != nil {
		return err
	}
	if _, err := io.WriteString(w, ",\n"); err != nil {
		return err
	}
	_, err = w.Write(tail)
	return err
}

// writeChunks writes the indented top-level array of n records. enc appends
// record i, led by a comma, and must be safe to call from several goroutines.
// One chunk of jsonChunk records per CPU is encoded at a time, each into a
// buffer its worker keeps, and the batch is written in order.
func writeChunks(w io.Writer, n int, enc func(b []byte, i int) ([]byte, error)) error {
	if n == 0 {
		// The whole-document encoder saw a nil slice here.
		_, err := io.WriteString(w, "null")
		return err
	}
	workers := runtime.GOMAXPROCS(0)
	datas := make([][]byte, workers)
	errs := make([]error, workers)
	for i := 0; i < n; i += workers * jsonChunk {
		var wg sync.WaitGroup
		k := 0
		for lo := i; lo < n && k < workers; lo, k = lo+jsonChunk, k+1 {
			wg.Add(1)
			go func(k, lo int) {
				defer wg.Done()
				b, err := datas[k][:0], error(nil)
				for j := lo; j < min(n, lo+jsonChunk) && err == nil; j++ {
					b, err = enc(b, j)
				}
				datas[k], errs[k] = b, err
			}(k, lo)
		}
		wg.Wait()
		for c := 0; c < k; c++ {
			if errs[c] != nil {
				return errs[c]
			}
			if i == 0 && c == 0 {
				datas[c][0] = '['
			}
			if _, err := w.Write(datas[c]); err != nil {
				return err
			}
		}
	}
	_, err := io.WriteString(w, "\n  ]")
	return err
}

// The append functions below write what json.MarshalIndent writes for one
// record of the nodes or links array; anything past the plain cases goes
// through encoding/json itself. TestWriteJSONMatchesMarshalIndent holds them
// to the whole-document encoder.

func appendJSONNode(b []byte, n jsonNode) ([]byte, error) {
	b = append(b, ",\n    {\n      \"id\": "...)
	b = model.AppendJSONString(b, n.ID)
	b = append(b, ",\n      \"label\": "...)
	b = model.AppendJSONString(b, n.Label)
	b = append(b, ",\n      \"file_type\": "...)
	b = model.AppendJSONString(b, n.FileType)
	b = append(b, ",\n      \"source_file\": "...)
	b = model.AppendJSONString(b, n.SourceFile)
	if n.SourceLocation != "" {
		b = append(b, ",\n      \"source_location\": "...)
		b = model.AppendJSONString(b, n.SourceLocation)
	}
	b = append(b, ",\n      \"community\": "...)
	if n.Community == nil {
		b = append(b, "null"...)
	} else {
		b = strconv.AppendInt(b, int64(*n.Community), 10)
	}
	b = append(b, ",\n      \"norm_label\": "...)
	b = model.AppendJSONString(b, n.NormLabel)
	if n.ComputedName != "" {
		b = append(b, ",\n      \"computed_name\": "...)
		b = model.AppendJSONString(b, n.ComputedName)
	}
	if len(n.Attributes) > 0 {
		q, err := json.MarshalIndent(n.Attributes, "      ", "  ")
		if err != nil {
			return b, err
		}
		b = append(b, ",\n      \"attributes\": "...)
		b = append(b, q...)
	}
	return append(b, "\n    }"...), nil
}

func appendJSONLink(b []byte, l jsonLink) ([]byte, error) {
	b = append(b, ",\n    {\n      \"source\": "...)
	b = model.AppendJSONString(b, l.Source)
	b = append(b, ",\n      \"target\": "...)
	b = model.AppendJSONString(b, l.Target)
	b = append(b, ",\n      \"relation\": "...)
	b = model.AppendJSONString(b, l.Relation)
	b = append(b, ",\n      \"confidence\": "...)
	b = model.AppendJSONString(b, l.Confidence)
	if l.Deferred {
		b = append(b, ",\n      \"deferred\": true"...)
	}
	if l.SourceFile != "" {
		b = append(b, ",\n      \"source_file\": "...)
		b = model.AppendJSONString(b, l.SourceFile)
	}
	if l.SourceLocation != 0 {
		b = l.SourceLocation.AppendJSON(append(b, ",\n      \"source_location\": "...))
	}
	var err error
	if l.Weight != 0 {
		b = append(b, ",\n      \"weight\": "...)
		if b, err = model.AppendJSONFloat(b, l.Weight); err != nil {
			return b, err
		}
	}
	b = append(b, ",\n      \"confidence_score\": "...)
	if b, err = model.AppendJSONFloat(b, l.ConfidenceScore); err != nil {
		return b, err
	}
	return append(b, "\n    }"...), nil
}

func toJSONNode(n *model.Node, nc map[string]int) jsonNode {
	var comm *int
	if c, ok := nc[n.ID]; ok {
		comm = &c
	}
	nl := normLabel(n.Label)
	// Computed names are built from repo content (null-label inputs,
	// frontmatter); cap and strip them once here, where every producer
	// converges on serialization.
	cn := security.SanitizeLabel(n.ComputedName)
	if cn != "" {
		nl = nl + " " + normLabel(cn)
	}
	return jsonNode{
		ID: n.ID, Label: n.Label, FileType: n.FileType,
		SourceFile: n.SourceFile, SourceLocation: n.SourceLocation,
		Community: comm, NormLabel: nl, ComputedName: cn,
		Attributes: n.Attributes,
	}
}

func toJSONLink(e *model.Edge) jsonLink {
	score := e.ConfidenceScore
	if score == 0 {
		score = confidenceScore[e.Confidence.String()]
	}
	return jsonLink{
		Source: e.Source, Target: e.Target, Relation: e.Relation.String(),
		Confidence: e.Confidence.String(), Deferred: e.Deferred(), SourceFile: e.SourceFile.String(),
		SourceLocation: e.Loc(), Weight: e.Weight, ConfidenceScore: score,
	}
}

// CheckShrink decides whether writing g over an existing graph.json at path would
// silently drop nodes, distinguishing a legitimate refactor/deletion shrink from a
// suspicious one. It mirrors upstream watch._check_shrink: the crude node-count
// guard in ToJSON cannot tell the two apart, so callers that know which sources
// were rebuilt this run call CheckShrink first and then ToJSON with force=true.
//
// A net shrink is legitimate when every node that would be lost belongs to a
// source that was rebuilt in place (its file is still in the new graph) or truly
// deleted (its file is gone from disk). Two cases are refused as silent loss:
// a lost node whose source FAILED to process this run (it is in skipped, #479),
// and a lost node whose source left the corpus yet still exists on disk — it was
// EXCLUDED (e.g. a new .gitignore/.graphifyignore rule), not deleted, so eviction
// would be data loss (fail-closed, #1795). CheckShrink returns nil when the write
// is safe (force set; no readable/non-empty graph on disk; the new graph is not
// smaller; or every lost node is accounted), ErrGraphShrink for a skipped or
// excluded-but-present source, or ErrGraphUnverifiable when a non-empty existing
// graph.json cannot be parsed to make the comparison. It never writes. root is
// the project root, used to resolve a lost node's (root-relative) source path for
// the on-disk existence check.
func CheckShrink(path string, g *model.Graph, skipped map[string]bool, root string, force bool) error {
	if force {
		return nil
	}
	return ReadShrinkGuard(path).Check(g, skipped, root)
}

// ShrinkGuard is CheckShrink in two steps, so the graph.json on disk can be
// read while the new graph is still being built.
type ShrinkGuard struct {
	path  string
	nodes []oldNode
	err   error
	done  chan struct{}
}

// ReadShrinkGuard starts reading the graph.json at path.
func ReadShrinkGuard(path string) *ShrinkGuard {
	s := &ShrinkGuard{path: path, done: make(chan struct{})}
	go func() {
		s.nodes, s.err = oldNodes(path)
		close(s.done)
	}()
	return s
}

// Check is CheckShrink without force, against the nodes read at the start.
func (s *ShrinkGuard) Check(g *model.Graph, skipped map[string]bool, root string) error {
	<-s.done
	// An absent, unreadable or empty file has no nodes to lose.
	path, existing, err := s.path, s.nodes, s.err
	if err != nil {
		return fmt.Errorf("%w %s to verify the new graph is not smaller (%v); pass --force to override", ErrGraphUnverifiable, path, err)
	}
	newN := g.NumNodes()
	if newN >= len(existing) {
		// Growth (or same size): never a shrink.
		return nil
	}
	// Sources present in the freshly built graph: a lost node from one of these
	// is a normal in-file refactor (the file was rebuilt, its nodes changed), not
	// an eviction, so it never blocks the write.
	currentSources := make(map[string]bool)
	for _, nd := range g.Nodes {
		if nd.SourceFile != "" {
			currentSources[nd.SourceFile] = true
		}
	}
	// Net shrink. A lost node is legitimate only when its source was rebuilt in
	// place or truly deleted from disk. Refuse a lost node whose source failed to
	// process (skipped) or left the corpus while still on disk (excluded).
	for _, n := range existing {
		if _, ok := g.Nodes[n.ID]; ok {
			continue // node survived
		}
		if n.SourceFile == "" || currentSources[n.SourceFile] {
			continue // derived node, or source rebuilt in place — not an eviction
		}
		if skipped[n.SourceFile] {
			return fmt.Errorf("%w: existing has %d nodes, new has %d (net -%d); a lost node belongs to %s, which failed to process this run; pass --force to override",
				ErrGraphShrink, len(existing), newN, len(existing)-newN, n.SourceFile)
		}
		// Only a real file on disk is eviction evidence. A synthetic node whose
		// SourceFile names a directory (e.g. a Terraform local-module reference) is
		// not an extractable source, so a still-present directory must not block the
		// shrink — mirrors upstream skipping nodes with no file extractor.
		if fi, err := os.Stat(filepath.Join(root, n.SourceFile)); err == nil && fi.Mode().IsRegular() {
			// Source still on disk but absent from the corpus: excluded by a new
			// ignore rule, not deleted. Fail closed so its nodes are not evicted.
			return fmt.Errorf("%w: existing has %d nodes, new has %d (net -%d); a lost node belongs to %s, which still exists on disk but left the corpus (excluded, not deleted); pass --force to override",
				ErrGraphShrink, len(existing), newN, len(existing)-newN, n.SourceFile)
		}
	}
	return nil
}

// guardNoShrink refuses to overwrite an existing graph.json at path when the new
// graph (newN nodes) would drop nodes relative to what is on disk. An absent,
// unreadable, or empty file is treated as nothing-to-lose and returns nil.
func guardNoShrink(path string, newN int) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		// Absent (or unreadable) target: no nodes to lose — write normally.
		return nil
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		// Empty/whitespace file (e.g. a freshly touched path): proceed.
		return nil
	}
	var existing struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &existing); err != nil {
		return fmt.Errorf("%w %s to verify the new graph is not smaller (%v); pass force to override", ErrGraphUnverifiable, path, err)
	}
	if existingN := len(existing.Nodes); newN < existingN {
		return fmt.Errorf("%w: existing has %d nodes, new has %d (net -%d); pass force to override", ErrGraphShrink, existingN, newN, existingN-newN)
	}
	return nil
}

// normLabel mirrors export._strip_diacritics + lowercase: NFKD-decompose, drop
// combining marks, lowercase. Used as a fast case/accent-insensitive search key.
func normLabel(label string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(label) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return strings.ToLower(b.String())
}
