// Package model holds the graph data types shared across the pipeline: the
// extraction schema (nodes + edges) emitted by extractors and the assembled
// undirected Graph that downstream stages cluster, analyze, and export.
package model

import "sort"

// Node is a single entity in the graph (a file, function, type, or method).
type Node struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	FileType       string `json:"file_type"`
	SourceFile     string `json:"source_file"`
	SourceLocation string `json:"source_location,omitempty"`
}

// Edge is a directed relationship between two nodes. The graph stores edges
// undirected (so community detection and degree work), but Source/Target always
// record the true direction (caller→callee, importer→imported).
type Edge struct {
	Source          string  `json:"source"`
	Target          string  `json:"target"`
	Relation        string  `json:"relation"`
	Confidence      string  `json:"confidence"`
	SourceFile      string  `json:"source_file,omitempty"`
	SourceLocation  string  `json:"source_location,omitempty"`
	Weight          float64 `json:"weight,omitempty"`
	ConfidenceScore float64 `json:"confidence_score,omitempty"`
}

// Extraction is one extractor's output for one file.
type Extraction struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Graph is the assembled knowledge graph. It keeps the full directed edge list
// (so a→b and b→a, or two different relations on one pair, both survive) plus an
// undirected neighbour set used for degree, traversal, and community detection.
type Graph struct {
	Nodes map[string]*Node
	edges []*Edge
	adj   map[string]map[string]bool // undirected neighbour set
	seen  map[string]bool            // dedup key "src\x00tgt\x00relation"
	order []string                   // node insertion order, for stable iteration
}

// New returns an empty graph.
func New() *Graph {
	return &Graph{
		Nodes: map[string]*Node{},
		adj:   map[string]map[string]bool{},
		seen:  map[string]bool{},
	}
}

// AddNode inserts or overwrites a node (last write wins, mirroring nx.add_node).
func (g *Graph) AddNode(n Node) {
	if _, ok := g.Nodes[n.ID]; !ok {
		g.order = append(g.order, n.ID)
		g.adj[n.ID] = map[string]bool{}
	}
	cp := n
	g.Nodes[n.ID] = &cp
}

// AddEdge records a directed edge between existing nodes. Exact duplicates
// (same source, target, and relation) are ignored; opposite directions and
// different relations on the same pair are all kept.
func (g *Graph) AddEdge(e Edge) {
	if _, ok := g.Nodes[e.Source]; !ok {
		return
	}
	if _, ok := g.Nodes[e.Target]; !ok {
		return
	}
	key := e.Source + "\x00" + e.Target + "\x00" + e.Relation
	if g.seen[key] {
		return
	}
	g.seen[key] = true
	cp := e
	g.edges = append(g.edges, &cp)
	g.adj[e.Source][e.Target] = true
	g.adj[e.Target][e.Source] = true
}

// HasEdge reports whether an undirected edge connects a and b.
func (g *Graph) HasEdge(a, b string) bool { return g.adj[a][b] }

// Degree returns the number of distinct neighbors of a node.
func (g *Graph) Degree(id string) int { return len(g.adj[id]) }

// Neighbors returns a node's neighbor IDs in sorted order (stable iteration).
func (g *Graph) Neighbors(id string) []string {
	ns := make([]string, 0, len(g.adj[id]))
	for n := range g.adj[id] {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return ns
}

// NodeIDs returns all node IDs in insertion order.
func (g *Graph) NodeIDs() []string { return g.order }

// NumNodes returns the node count.
func (g *Graph) NumNodes() int { return len(g.Nodes) }

// NumEdges returns the total number of stored edges.
func (g *Graph) NumEdges() int { return len(g.edges) }

// Edges returns every stored edge in insertion order.
func (g *Graph) Edges() []*Edge { return g.edges }
