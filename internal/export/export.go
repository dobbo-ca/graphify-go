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
	"os"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
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
	ID             string `json:"id"`
	Label          string `json:"label"`
	FileType       string `json:"file_type"`
	SourceFile     string `json:"source_file"`
	SourceLocation string `json:"source_location,omitempty"`
	Community      *int   `json:"community"`
	NormLabel      string `json:"norm_label"`
	ComputedName   string `json:"computed_name,omitempty"`
}

type jsonLink struct {
	Source          string  `json:"source"`
	Target          string  `json:"target"`
	Relation        string  `json:"relation"`
	Confidence      string  `json:"confidence"`
	SourceFile      string  `json:"source_file,omitempty"`
	SourceLocation  string  `json:"source_location,omitempty"`
	Weight          float64 `json:"weight,omitempty"`
	ConfidenceScore float64 `json:"confidence_score"`
}

type jsonGraph struct {
	Directed      bool       `json:"directed"`
	Multigraph    bool       `json:"multigraph"`
	Graph         struct{}   `json:"graph"`
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

	nc := cluster.NodeCommunity(communities)
	var out jsonGraph
	out.Hyperedges = []any{}
	out.BuiltAtCommit = builtAtCommit

	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		var comm *int
		if c, ok := nc[id]; ok {
			comm = &c
		}
		nl := normLabel(n.Label)
		if n.ComputedName != "" {
			nl = nl + " " + normLabel(n.ComputedName)
		}
		out.Nodes = append(out.Nodes, jsonNode{
			ID: n.ID, Label: n.Label, FileType: n.FileType,
			SourceFile: n.SourceFile, SourceLocation: n.SourceLocation,
			Community: comm, NormLabel: nl, ComputedName: n.ComputedName,
		})
	}
	for _, e := range g.Edges() {
		score := e.ConfidenceScore
		if score == 0 {
			score = confidenceScore[e.Confidence]
		}
		out.Links = append(out.Links, jsonLink{
			Source: e.Source, Target: e.Target, Relation: e.Relation,
			Confidence: e.Confidence, SourceFile: e.SourceFile,
			SourceLocation: e.SourceLocation, Weight: e.Weight, ConfidenceScore: score,
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// CheckShrink decides whether writing g over an existing graph.json at path would
// silently drop nodes, distinguishing a legitimate refactor/deletion shrink from a
// suspicious one. It mirrors upstream watch._check_shrink: the crude node-count
// guard in ToJSON cannot tell the two apart, so callers that know which sources
// were rebuilt this run call CheckShrink first and then ToJSON with force=true.
//
// A net shrink is legitimate when every node that would be lost belongs to a source
// file that was (re)built or removed this run — none belong to skipped, the current
// source files that FAILED to process this run. A lost node from a skipped file is
// the silent loss (#479) worth refusing. CheckShrink returns nil when the write is
// safe (force set; no readable/non-empty graph on disk; the new graph is not
// smaller; or every lost node is accounted), ErrGraphShrink when a lost node's
// source file was skipped, or ErrGraphUnverifiable when a non-empty existing
// graph.json cannot be parsed to make the comparison. It never writes.
func CheckShrink(path string, g *model.Graph, skipped map[string]bool, force bool) error {
	if force {
		return nil
	}
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
		Nodes []struct {
			ID         string `json:"id"`
			SourceFile string `json:"source_file"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &existing); err != nil {
		return fmt.Errorf("%w %s to verify the new graph is not smaller (%v); pass --force to override", ErrGraphUnverifiable, path, err)
	}
	newN := g.NumNodes()
	if newN >= len(existing.Nodes) {
		// Growth (or same size): never a shrink.
		return nil
	}
	// Net shrink. It is legitimate only when every lost node belongs to a source
	// rebuilt or removed this run. A lost node whose source file is still present
	// but was skipped (failed to read/parse) is an unexplained loss — refuse.
	for _, n := range existing.Nodes {
		if _, ok := g.Nodes[n.ID]; ok {
			continue // node survived
		}
		if n.SourceFile != "" && skipped[n.SourceFile] {
			return fmt.Errorf("%w: existing has %d nodes, new has %d (net -%d); a lost node belongs to %s, which failed to process this run; pass --force to override",
				ErrGraphShrink, len(existing.Nodes), newN, len(existing.Nodes)-newN, n.SourceFile)
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
