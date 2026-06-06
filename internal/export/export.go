// Package export writes the assembled graph to disk. graph.json is the primary
// artifact (committed by CI, read by the query commands and the Claude skill);
// it uses the same NetworkX node-link shape as the Python original so existing
// tooling keeps working. graph.html is a small optional viewer for humans.
package export

import (
	"encoding/json"
	"os"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

var confidenceScore = map[string]float64{"EXTRACTED": 1.0, "INFERRED": 0.5, "AMBIGUOUS": 0.2}

type jsonNode struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	FileType       string `json:"file_type"`
	SourceFile     string `json:"source_file"`
	SourceLocation string `json:"source_location,omitempty"`
	Community      *int   `json:"community"`
	NormLabel      string `json:"norm_label"`
}

type jsonLink struct {
	Source          string  `json:"source"`
	Target          string  `json:"target"`
	Relation        string  `json:"relation"`
	Confidence      string  `json:"confidence"`
	SourceFile      string  `json:"source_file,omitempty"`
	SourceLocation  string  `json:"source_location,omitempty"`
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
func ToJSON(g *model.Graph, communities map[int][]string, path, builtAtCommit string) error {
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
		out.Nodes = append(out.Nodes, jsonNode{
			ID: n.ID, Label: n.Label, FileType: n.FileType,
			SourceFile: n.SourceFile, SourceLocation: n.SourceLocation,
			Community: comm, NormLabel: normLabel(n.Label),
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
			SourceLocation: e.SourceLocation, ConfidenceScore: score,
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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
