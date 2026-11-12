// Package model holds the graph data types shared across the pipeline: the
// extraction schema (nodes + edges) emitted by extractors and the assembled
// undirected Graph that downstream stages cluster, analyze, and export.
package model

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"unsafe"
)

// Node is a single entity in the graph (a file, function, type, or method).
type Node struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	FileType       string `json:"file_type"`
	SourceFile     string `json:"source_file"`
	SourceLocation string `json:"source_location,omitempty"`
	ComputedName   string `json:"computed_name,omitempty"`
	// Attributes holds a block's literal key/value pairs (Terraform only today),
	// so "which resources run t3.large" is a graph query. omitempty keeps
	// graph.json byte-identical for corpora that set nothing.
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Edge is a directed relationship between two nodes. The graph stores edges
// undirected (so community detection and degree work), but Source/Target always
// record the true direction (caller→callee, importer→imported).
type Edge struct {
	Source     string     `json:"source"`
	Target     string     `json:"target"`
	Relation   Relation   `json:"relation"`
	Confidence Confidence `json:"confidence"`
	// Zero size: where json reads "type_only".
	TypeOnlyJSON typeOnlyJSON `json:"type_only"`
	// Zero size: where json reads "deferred".
	DeferredJSON deferredJSON `json:"deferred"`
	SourceFile   File         `json:"source_file,omitempty"`
	// Bits 30 and 29 are the type-only and deferred flags: read with Loc.
	SourceLocation  Loc     `json:"source_location,omitempty"`
	Weight          float64 `json:"weight,omitempty"`
	ConfidenceScore float64 `json:"confidence_score,omitempty"`
}

// Loc returns the edge's source location.
func (e Edge) Loc() Loc { return e.SourceLocation &^ locFlags }

// TypeOnly reports an import edge erased at compile time (TypeScript
// `import type`), which cannot form a runtime import cycle.
func (e Edge) TypeOnly() bool { return e.SourceLocation&locTypeOnly != 0 }

// SetTypeOnly marks the edge type-only, or not.
func (e *Edge) SetTypeOnly(on bool) {
	e.SourceLocation &^= locTypeOnly
	if on {
		e.SourceLocation |= locTypeOnly
	}
}

// Deferred reports an import() edge: a real dependency that runs late, so
// it cannot form a static import cycle.
func (e Edge) Deferred() bool { return e.SourceLocation&locDeferred != 0 }

// SetDeferred marks the edge deferred, or not.
func (e *Edge) SetDeferred(on bool) {
	e.SourceLocation &^= locDeferred
	if on {
		e.SourceLocation |= locDeferred
	}
}

// MarshalJSON writes the flags as "type_only" and "deferred".
func (e Edge) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Source          string     `json:"source"`
		Target          string     `json:"target"`
		Relation        Relation   `json:"relation"`
		Confidence      Confidence `json:"confidence"`
		TypeOnly        bool       `json:"type_only,omitempty"`
		Deferred        bool       `json:"deferred,omitempty"`
		SourceFile      File       `json:"source_file,omitempty"`
		SourceLocation  Loc        `json:"source_location,omitempty"`
		Weight          float64    `json:"weight,omitempty"`
		ConfidenceScore float64    `json:"confidence_score,omitempty"`
	}{e.Source, e.Target, e.Relation, e.Confidence, e.TypeOnly(), e.Deferred(), e.SourceFile, e.Loc(), e.Weight, e.ConfidenceScore})
}

// typeOnlyJSON is the type of Edge.TypeOnlyJSON only. An UnmarshalJSON on
// Edge itself decodes edges 1.7x slower.
type typeOnlyJSON struct{}

// UnmarshalJSON sets the flag of the Edge around t.
func (t *typeOnlyJSON) UnmarshalJSON(b []byte) error {
	e := (*Edge)(unsafe.Add(unsafe.Pointer(t), -int(unsafe.Offsetof(Edge{}.TypeOnlyJSON))))
	on := e.TypeOnly() // a null leaves it
	err := json.Unmarshal(b, &on)
	e.SetTypeOnly(on)
	return err
}

// deferredJSON is the type of Edge.DeferredJSON only, like typeOnlyJSON.
type deferredJSON struct{}

// UnmarshalJSON sets the flag of the Edge around t.
func (t *deferredJSON) UnmarshalJSON(b []byte) error {
	e := (*Edge)(unsafe.Add(unsafe.Pointer(t), -int(unsafe.Offsetof(Edge{}.DeferredJSON))))
	on := e.Deferred() // a null leaves it
	err := json.Unmarshal(b, &on)
	e.SetDeferred(on)
	return err
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
	adj   [][]uint32           // undirected neighbours by node position, no repeats
	pairs map[uint64]struct{}  // node pairs already in adj
	seen  map[edgeKey]struct{} // edges already stored, for dedup
	seenS map[string]struct{}  // same, for ids that contain NUL
	index map[string]uint32    // node id -> position in order
	slab  []Edge               // backing store the next edges are cut from
	order []string             // node insertion order, for stable iteration

	// Unclassified counts, per lowercased extension, the files the corpus walk
	// saw but no extractor handles. It is a graph-level attribute (networkx's
	// G.graph dict), not part of the node/edge data: it tells a consumer how
	// much of the repo the graph actually covers.
	Unclassified map[string]int
	// OverBudget counts the files whose parse the budget stopped.
	OverBudget int
}

// edgeKey identifies an edge by node position and relation number. Numbers keep
// the dedup set a fraction of the size of one concatenated string per edge.
type edgeKey struct{ src, tgt, rel uint32 }

// New returns an empty graph.
func New() *Graph {
	return &Graph{
		Nodes: map[string]*Node{},
		pairs: map[uint64]struct{}{},
		seen:  map[edgeKey]struct{}{},
		index: map[string]uint32{},
	}
}

// Reserve makes room for n nodes.
func (g *Graph) Reserve(n int) {
	g.Nodes = make(map[string]*Node, n)
	g.index = make(map[string]uint32, n)
	g.order = make([]string, 0, n)
	g.adj = make([][]uint32, 0, n)
}

// AddNode inserts or overwrites a node (last write wins, mirroring nx.add_node).
func (g *Graph) AddNode(n Node) {
	if _, ok := g.Nodes[n.ID]; !ok {
		g.index[n.ID] = uint32(len(g.order))
		g.order = append(g.order, n.ID)
		g.adj = append(g.adj, nil)
	}
	cp := n
	g.Nodes[n.ID] = &cp
}

// AddEdge records a directed edge between existing nodes. Exact duplicates
// (same source, target, and relation) are ignored; opposite directions and
// different relations on the same pair are all kept.
func (g *Graph) AddEdge(e Edge) {
	src, ok := g.index[e.Source]
	if !ok {
		return
	}
	tgt, ok := g.index[e.Target]
	if !ok {
		return
	}
	if keep, _ := g.store(e, src, tgt); keep {
		g.link(src, tgt)
	}
}

// NoNode in src tells AddEdgesAt to skip an edge.
const NoNode = ^uint32(0)

// AddEdgesAt adds *edges[i] for each i of order, as AddEdge would after
// setting Source and Target to the IDs of the nodes at src[i] and tgt[i]. The caller
// has the positions already, so no edge is looked up by ID here, and the
// neighbour lists are filled beside the edge list. An edge that names those
// IDs already is kept where it is, not copied: a second copy of every edge was
// 100 MB on a large corpus. The caller must not write to the edges afterwards.
func (g *Graph) AddEdgesAt(edges []*Edge, order []int32, src, tgt []uint32) {
	g.edges = slices.Grow(g.edges, len(order))
	linked := make(chan struct{})
	go func() {
		// A dropped duplicate names a pair that is linked already, so the
		// links do not need the verdicts; a NUL collision is the exception.
		for _, i := range order {
			if src[i] != NoNode {
				g.link(src[i], tgt[i])
			}
		}
		close(linked)
	}()
	collided := false
	for _, i := range order {
		if src[i] == NoNode {
			continue
		}
		e := edges[i]
		if s, t := g.order[src[i]], g.order[tgt[i]]; e.Source != s || e.Target != t {
			cp := *e
			cp.Source, cp.Target = s, t
			if keep, byString := g.store(cp, src[i], tgt[i]); !keep && byString {
				collided = true
			}
			continue
		}
		if dup, byString := g.dup(e, src[i], tgt[i]); !dup {
			g.edges = append(g.edges, e)
		} else if byString {
			collided = true
		}
	}
	<-linked
	if collided {
		// Link again from the edges that were kept.
		g.pairs = map[uint64]struct{}{}
		clear(g.adj)
		for _, e := range g.edges {
			g.link(g.index[e.Source], g.index[e.Target])
		}
	}
}

// store appends e unless an edge with its source, target and relation is there
// already. byString reports that the verdict came from the joined key.
func (g *Graph) store(e Edge, src, tgt uint32) (keep, byString bool) {
	if dup, byString := g.dup(&e, src, tgt); dup {
		return false, byString
	}
	// One allocation per 1024 edges instead of one each.
	if len(g.slab) == cap(g.slab) {
		g.slab = make([]Edge, 0, 1024)
	}
	g.slab = append(g.slab, e)
	g.edges = append(g.edges, &g.slab[len(g.slab)-1])
	return true, false
}

// dup reports whether an edge with e's source, target and relation is there
// already, and records e's if not.
func (g *Graph) dup(e *Edge, src, tgt uint32) (dup, byString bool) {
	rel := e.Relation.String()
	if hasNUL(e.Source) || hasNUL(e.Target) || hasNUL(rel) {
		// The old joined key let such edges collide; keep that.
		k := e.Source + "\x00" + e.Target + "\x00" + rel
		if _, dup := g.seenS[k]; dup {
			return true, true
		}
		if g.seenS == nil {
			g.seenS = map[string]struct{}{}
		}
		g.seenS[k] = struct{}{}
	} else {
		key := edgeKey{src, tgt, uint32(e.Relation)}
		if _, dup := g.seen[key]; dup {
			return true, false
		}
		g.seen[key] = struct{}{}
	}
	return false, false
}

// link makes src and tgt neighbours.
func (g *Graph) link(src, tgt uint32) {
	if p := pairKey(src, tgt); !g.hasPair(p) {
		g.pairs[p] = struct{}{}
		g.adj[src] = append(g.adj[src], tgt)
		if src != tgt {
			g.adj[tgt] = append(g.adj[tgt], src)
		}
	}
}

func pairKey(a, b uint32) uint64 {
	if a > b {
		a, b = b, a
	}
	return uint64(a)<<32 | uint64(b)
}

func (g *Graph) hasPair(p uint64) bool {
	_, ok := g.pairs[p]
	return ok
}

func hasNUL(s string) bool { return strings.IndexByte(s, 0) >= 0 }

// HasEdge reports whether an undirected edge connects a and b.
func (g *Graph) HasEdge(a, b string) bool {
	i, ok := g.index[a]
	j, ok2 := g.index[b]
	return ok && ok2 && g.hasPair(pairKey(i, j))
}

// Degree returns the number of distinct neighbors of a node.
func (g *Graph) Degree(id string) int { return len(g.NeighborIndexes(id)) }

// Neighbors returns a node's neighbor IDs in sorted order (stable iteration).
func (g *Graph) Neighbors(id string) []string {
	nbrs := g.NeighborIndexes(id)
	ns := make([]string, 0, len(nbrs))
	for _, n := range nbrs {
		ns = append(ns, g.order[n])
	}
	sort.Strings(ns)
	return ns
}

// Index returns a node's position in NodeIDs.
func (g *Graph) Index(id string) (uint32, bool) {
	i, ok := g.index[id]
	return i, ok
}

// NeighborIndexes returns a node's neighbors as positions in NodeIDs, in no
// particular order, for callers that do not need the IDs Neighbors builds. The
// caller must not modify it.
func (g *Graph) NeighborIndexes(id string) []uint32 {
	i, ok := g.index[id]
	if !ok {
		return nil
	}
	return g.adj[i]
}

// NodeIDs returns all node IDs in insertion order.
func (g *Graph) NodeIDs() []string { return g.order }

// NumNodes returns the node count.
func (g *Graph) NumNodes() int { return len(g.Nodes) }

// NumEdges returns the total number of stored edges.
func (g *Graph) NumEdges() int { return len(g.edges) }

// Edges returns every stored edge in insertion order.
func (g *Graph) Edges() []*Edge { return g.edges }
