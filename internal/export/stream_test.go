package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// wholeJSON is the pre-streaming encoder: one MarshalIndent of the document.
func wholeJSON(t *testing.T, g *model.Graph, communities map[int][]string, commit string) []byte {
	t.Helper()
	nc := cluster.NodeCommunity(communities)
	var out jsonGraph
	out.Hyperedges = []any{}
	out.BuiltAtCommit = commit
	for ext, n := range g.Unclassified {
		if out.Graph.UnclassifiedExts == nil {
			out.Graph.UnclassifiedExts = map[string]int{}
		}
		out.Graph.UnclassifiedExts[ext] = n
		out.Graph.UnclassifiedFiles += n
	}
	for _, id := range g.NodeIDs() {
		out.Nodes = append(out.Nodes, toJSONNode(g.Nodes[id], nc))
	}
	for _, e := range g.Edges() {
		out.Links = append(out.Links, toJSONLink(e))
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// streamGraph has n nodes and m edges whose fields cover what the encoder
// treats specially: HTML and control characters, non-ASCII, invalid UTF-8,
// omitempty fields, attribute maps (nil, empty, multi-key) and weights.
func streamGraph(n, m int) (*model.Graph, map[int][]string) {
	labels := []string{"plain", "<b>&amp;</b>", "naïve café", "日本語   sep", "tab\there\nnl", "quote\"back\\slash", "bad\xffutf8", "\"nodes\": null,", "del\x7f", "sep\u2028\u2029", "a&b", "a<b", "a>b", "a\\b", "a\"b", "a\x1fb", "a\x80b"}
	g := model.New()
	communities := map[int][]string{}
	for i := 0; i < n; i++ {
		node := model.Node{ID: fmt.Sprintf("n%d", i), Label: labels[i%len(labels)], FileType: "code", SourceFile: "a/ü.go"}
		switch i % 4 {
		case 1:
			node.SourceLocation = "L" + fmt.Sprint(i)
			node.Attributes = map[string]string{}
		case 2:
			node.ComputedName = "computed " + labels[(i+1)%len(labels)]
			node.Attributes = map[string]string{"z": "<1>", "a": "é", "\"nodes\": null,": "x"}
		}
		g.AddNode(node)
		if i%3 != 0 {
			communities[i%5] = append(communities[i%5], node.ID)
		}
	}
	rels := []string{"calls", "imports", "contains", "rel<é>"}
	for i := 0; i < m; i++ {
		e := model.Edge{Source: fmt.Sprintf("n%d", i%n), Target: fmt.Sprintf("n%d", (i*7+1)%n), Relation: model.Rel(rels[i%len(rels)] + fmt.Sprint(i/n)), Confidence: model.Conf([]string{"EXTRACTED", "INFERRED", "AMBIGUOUS", "OTHER"}[i%4])}
		switch i % 3 {
		case 1:
			e.SourceFile, e.SourceLocation, e.Weight = model.FileOf("b/日.py"), model.Line(9), 0.1+float64(i)/3
		case 2:
			e.ConfidenceScore = 1e-7 * float64(i)
			if i%5 == 0 {
				e.ConfidenceScore, e.Weight = 1e21*float64(i+1), -1e-7
			}
		}
		g.AddEdge(e)
	}
	return g, communities
}

func TestWriteJSONMatchesMarshalIndent(t *testing.T) {
	old := jsonChunk
	defer func() { jsonChunk = old }()
	jsonChunk = 8
	type tc struct{ nodes, edges int }
	cases := []tc{{0, 0}, {1, 0}, {1, 1}, {2, 1}}
	// 300 crosses a batch of chunks on up to 37 CPUs.
	for _, n := range []int{7, 8, 9, 15, 16, 17, 100, 300} {
		for _, m := range []int{0, 1, 7, 8, 9, 16, 17, 100, 300} {
			cases = append(cases, tc{n, m})
		}
	}
	for _, c := range cases {
		g, communities := streamGraph(max(c.nodes, 1), c.edges)
		if c.nodes == 0 {
			g, communities = model.New(), nil
		}
		for _, unc := range []map[string]int{nil, {}, {".x\"y": 2, ".<a>": 1}} {
			for _, commit := range []string{"", "abc123"} {
				g.Unclassified = unc
				want := wholeJSON(t, g, communities, commit)
				var got bytes.Buffer
				if err := writeJSON(&got, g, communities, commit); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got.Bytes(), want) {
					t.Fatalf("nodes=%d edges=%d unclassified=%v commit=%q: streamed output differs\n got: %s\nwant: %s", c.nodes, c.edges, unc, commit, got.Bytes(), want)
				}
			}
		}
	}
}
