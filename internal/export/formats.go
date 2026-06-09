package export

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Additional export formats derived from a built graph.json. They read the
// canonical artifact rather than rebuilding, so `graphify export <format>`
// reflects exactly what was committed. GraphML and CSV use the stdlib encoders
// for correct escaping; DOT is small enough to format directly.

func readGraphJSON(path string) (jsonGraph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return jsonGraph{}, err
	}
	var g jsonGraph
	if err := json.Unmarshal(data, &g); err != nil {
		return jsonGraph{}, err
	}
	return g, nil
}

// --- GraphML ---

type gmlKey struct {
	ID       string `xml:"id,attr"`
	For      string `xml:"for,attr"`
	AttrName string `xml:"attr.name,attr"`
	AttrType string `xml:"attr.type,attr"`
}

type gmlData struct {
	Key string `xml:"key,attr"`
	Val string `xml:",chardata"`
}

type gmlNode struct {
	ID   string    `xml:"id,attr"`
	Data []gmlData `xml:"data"`
}

type gmlEdge struct {
	Source string    `xml:"source,attr"`
	Target string    `xml:"target,attr"`
	Data   []gmlData `xml:"data"`
}

type gmlGraph struct {
	EdgeDefault string    `xml:"edgedefault,attr"`
	Nodes       []gmlNode `xml:"node"`
	Edges       []gmlEdge `xml:"edge"`
}

type graphml struct {
	XMLName xml.Name `xml:"graphml"`
	Xmlns   string   `xml:"xmlns,attr"`
	Keys    []gmlKey `xml:"key"`
	Graph   gmlGraph `xml:"graph"`
}

// GraphMLFromJSON writes the graph at jsonPath to outPath as GraphML.
func GraphMLFromJSON(jsonPath, outPath string) error {
	g, err := readGraphJSON(jsonPath)
	if err != nil {
		return err
	}
	doc := graphml{
		Xmlns: "http://graphml.graphdrawing.org/xmlns",
		Keys: []gmlKey{
			{ID: "label", For: "node", AttrName: "label", AttrType: "string"},
			{ID: "file_type", For: "node", AttrName: "file_type", AttrType: "string"},
			{ID: "source_file", For: "node", AttrName: "source_file", AttrType: "string"},
			{ID: "community", For: "node", AttrName: "community", AttrType: "int"},
			{ID: "relation", For: "edge", AttrName: "relation", AttrType: "string"},
			{ID: "confidence", For: "edge", AttrName: "confidence", AttrType: "string"},
		},
		Graph: gmlGraph{EdgeDefault: "directed"},
	}
	for _, n := range g.Nodes {
		data := []gmlData{
			{Key: "label", Val: n.Label},
			{Key: "file_type", Val: n.FileType},
			{Key: "source_file", Val: n.SourceFile},
		}
		if n.Community != nil {
			data = append(data, gmlData{Key: "community", Val: strconv.Itoa(*n.Community)})
		}
		doc.Graph.Nodes = append(doc.Graph.Nodes, gmlNode{ID: n.ID, Data: data})
	}
	for _, e := range g.Links {
		doc.Graph.Edges = append(doc.Graph.Edges, gmlEdge{
			Source: e.Source, Target: e.Target,
			Data: []gmlData{{Key: "relation", Val: e.Relation}, {Key: "confidence", Val: e.Confidence}},
		})
	}
	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, append([]byte(xml.Header), append(out, '\n')...), 0o644)
}

// --- DOT (Graphviz) ---

// DOTFromJSON writes the graph at jsonPath to outPath as a Graphviz digraph.
func DOTFromJSON(jsonPath, outPath string) error {
	g, err := readGraphJSON(jsonPath)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("digraph graphify {\n")
	b.WriteString("  node [shape=box];\n")
	for _, n := range g.Nodes {
		fmt.Fprintf(&b, "  %s [label=%s];\n", dotQuote(n.ID), dotQuote(n.Label))
	}
	for _, e := range g.Links {
		fmt.Fprintf(&b, "  %s -> %s [label=%s];\n", dotQuote(e.Source), dotQuote(e.Target), dotQuote(e.Relation))
	}
	b.WriteString("}\n")
	return os.WriteFile(outPath, []byte(b.String()), 0o644)
}

// dotQuote returns s as a DOT double-quoted ID, escaping quotes and backslashes
// and flattening newlines.
func dotQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", "")
	return `"` + r.Replace(s) + `"`
}

// --- CSV ---

// CSVFromJSON writes the graph's nodes and edges to two CSV files.
func CSVFromJSON(jsonPath, nodesPath, edgesPath string) error {
	g, err := readGraphJSON(jsonPath)
	if err != nil {
		return err
	}
	if err := writeCSV(nodesPath, []string{"id", "label", "file_type", "source_file", "source_location", "community"},
		func(w *csv.Writer) error {
			for _, n := range g.Nodes {
				comm := ""
				if n.Community != nil {
					comm = strconv.Itoa(*n.Community)
				}
				if err := w.Write([]string{n.ID, n.Label, n.FileType, n.SourceFile, n.SourceLocation, comm}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
		return err
	}
	return writeCSV(edgesPath, []string{"source", "target", "relation", "confidence", "confidence_score"},
		func(w *csv.Writer) error {
			for _, e := range g.Links {
				if err := w.Write([]string{e.Source, e.Target, e.Relation, e.Confidence,
					strconv.FormatFloat(e.ConfidenceScore, 'g', -1, 64)}); err != nil {
					return err
				}
			}
			return nil
		})
}

func writeCSV(path string, header []string, rows func(*csv.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return err
	}
	if err := rows(w); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}
