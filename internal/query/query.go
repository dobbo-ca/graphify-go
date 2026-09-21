// Package query answers questions against a built graph.json without rebuilding:
// find nodes by name, explain a node and its neighbours, and find the shortest
// dependency path between two nodes. These back the CLI commands the Claude
// skill uses instead of grepping the source tree.
package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/security"
)

// Graph is a loaded graph.json.
type Graph struct {
	Nodes []Node     `json:"nodes"`
	Links []Link     `json:"links"`
	Attrs GraphAttrs `json:"graph"`

	byID map[string]*Node
	adj  map[string]map[string]bool
	edge map[[2]string]*Link // directed (source,target) -> link, for relation lookup
}

// GraphAttrs mirrors graph.json's graph-level attribute dict. It carries the
// corpus-coverage counters the build recorded: files walked past because no
// extractor handles their type.
type GraphAttrs struct {
	UnclassifiedFiles int            `json:"unclassified_files"`
	UnclassifiedExts  map[string]int `json:"unclassified_extensions"`
}

// UnclassifiedSummary reports how many files the build saw but could not
// classify, naming the three biggest extensions, so a consumer can judge
// whether the graph covers enough of the repo to trust. It returns "" when the
// corpus was fully classified (or was built before this was recorded).
func (g *Graph) UnclassifiedSummary() string {
	if g.Attrs.UnclassifiedFiles == 0 {
		return ""
	}
	exts := make([]string, 0, len(g.Attrs.UnclassifiedExts))
	for e := range g.Attrs.UnclassifiedExts {
		exts = append(exts, e)
	}
	sort.Slice(exts, func(i, j int) bool {
		a, b := g.Attrs.UnclassifiedExts[exts[i]], g.Attrs.UnclassifiedExts[exts[j]]
		if a != b {
			return a > b
		}
		return exts[i] < exts[j]
	})
	if len(exts) > 3 {
		exts = exts[:3]
	}
	line := fmt.Sprintf("Unclassified: %d file(s) no extractor handles", g.Attrs.UnclassifiedFiles)
	if len(exts) == 0 {
		return line
	}
	parts := make([]string, len(exts))
	for i, e := range exts {
		parts[i] = fmt.Sprintf("%s %d", security.SanitizeLabel(e), g.Attrs.UnclassifiedExts[e])
	}
	return line + " (" + strings.Join(parts, ", ") + ")"
}

// Node mirrors a graph.json node.
type Node struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	FileType       string `json:"file_type"`
	SourceFile     string `json:"source_file"`
	SourceLocation string `json:"source_location"`
	Community      *int   `json:"community"`
	NormLabel      string `json:"norm_label"`
	ComputedName   string `json:"computed_name"`
}

// Link mirrors a graph.json edge.
type Link struct {
	Source         string `json:"source"`
	Target         string `json:"target"`
	Relation       string `json:"relation"`
	Confidence     string `json:"confidence"`
	SourceFile     string `json:"source_file"`
	SourceLocation string `json:"source_location"`
}

// Load reads and validates a graph.json at path. The path must resolve inside a
// graphify-out directory and stay under the size cap.
func Load(path string) (*Graph, error) {
	if err := security.CheckGraphFileSize(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	g.byID = make(map[string]*Node, len(g.Nodes))
	for i := range g.Nodes {
		g.byID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	g.adj = map[string]map[string]bool{}
	g.edge = map[[2]string]*Link{}
	for i := range g.Links {
		l := &g.Links[i]
		if g.adj[l.Source] == nil {
			g.adj[l.Source] = map[string]bool{}
		}
		if g.adj[l.Target] == nil {
			g.adj[l.Target] = map[string]bool{}
		}
		g.adj[l.Source][l.Target] = true
		g.adj[l.Target][l.Source] = true
		if _, ok := g.edge[[2]string{l.Source, l.Target}]; !ok {
			g.edge[[2]string{l.Source, l.Target}] = l
		}
	}
	return &g, nil
}

// Match is a node found by Query.
type Match struct {
	ID, Label, Location string
}

// Query returns nodes whose id, label, or normalised label matches the given
// regular expression (case-insensitive). Results are sorted by label.
func Query(g *Graph, pattern string) ([]Match, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, err
	}
	var out []Match
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if re.MatchString(n.Label) || re.MatchString(n.ID) || re.MatchString(n.NormLabel) {
			out = append(out, Match{n.ID, n.Label, loc(n)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out, nil
}

// Neighbor is an adjacent node in a given direction. Degree is the neighbour's
// own undirected degree, and File its source file, so callers can rank a
// high-degree node's connections and group the ones they cut.
type Neighbor struct {
	ID, Label, Relation, Direction, Location, File string
	Degree                                         int
}

// Explanation is a node plus its grouped neighbours.
type Explanation struct {
	Node      *Node
	Neighbors []Neighbor
}

// Explain returns a node and its neighbours, labelled by relation and direction
// (-> outgoing, <- incoming). id may be a full node ID or a label substring with
// a unique match.
func Explain(g *Graph, id string) (*Explanation, error) {
	n, err := g.resolve(id)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, fmt.Errorf("no node matching %q", id)
	}
	var nbrs []Neighbor
	for _, l := range g.Links {
		var other, dir string
		switch n.ID {
		case l.Source:
			other, dir = l.Target, "->"
		case l.Target:
			other, dir = l.Source, "<-"
		default:
			continue
		}
		o := g.byID[other]
		label, location, file := other, "", ""
		if o != nil {
			label, location, file = o.Label, loc(o), o.SourceFile
		}
		// Prefer the traversed edge's own call site over the neighbour's
		// definition line: "who calls this and where" wants the call site.
		if el := edgeLoc(l); el != "" {
			location = el
		}
		nbrs = append(nbrs, Neighbor{ID: other, Label: label, Relation: l.Relation,
			Direction: dir, Location: location, File: file, Degree: len(g.adj[other])})
	}
	// Most-connected neighbours first, so a caller showing only the head of the
	// list keeps the important ones; (relation, label, direction) breaks ties
	// deterministically.
	sort.Slice(nbrs, func(i, j int) bool {
		a, b := nbrs[i], nbrs[j]
		switch {
		case a.Degree != b.Degree:
			return a.Degree > b.Degree
		case a.Relation != b.Relation:
			return a.Relation < b.Relation
		case a.Label != b.Label:
			return a.Label < b.Label
		default:
			return a.Direction < b.Direction
		}
	})
	return &Explanation{Node: n, Neighbors: nbrs}, nil
}

// ErrNoDirectedPath is wrapped by PathEdges when a directed search finds
// no route; callers surface their own "retry undirected" hint.
var ErrNoDirectedPath = errors.New("no directed path")

// PathEdge annotates one step of a shortest path: the relation and confidence
// of the edge traversed to reach the step's node, and whether that edge is
// oriented forwards in the directed graph. In a PathResult, Edges[i] connects
// Nodes[i] to Nodes[i+1]; Forward is true when the recorded edge points
// Nodes[i] -> Nodes[i+1] and false when it points the other way.
type PathEdge struct {
	Relation   string
	Confidence string
	Forward    bool
}

// PathResult is a resolved shortest path: the ordered nodes plus, for every
// step after the first, the edge traversed to reach it (len(Edges) ==
// len(Nodes)-1).
type PathResult struct {
	Nodes []Node
	Edges []PathEdge
}

// SameNodeError is returned by PathEdges when source and target resolve to the
// same node, so the shortest path would be a meaningless zero hops.
type SameNodeError struct {
	From, To, ID string
}

func (e *SameNodeError) Error() string {
	return fmt.Sprintf("%q and %q both resolved to the same node %q; use a more specific label or the exact node ID", e.From, e.To, e.ID)
}

// MaxHopsError is returned by PathEdges when the shortest path is longer than
// the caller's maxHops limit.
type MaxHopsError struct {
	MaxHops, Hops int
}

func (e *MaxHopsError) Error() string {
	return fmt.Sprintf("path exceeds max_hops=%d (%d hops found)", e.MaxHops, e.Hops)
}

// PathEdges resolves from/to and returns the shortest path between them
// annotated with each traversed edge's relation and confidence, following edge
// direction unless undirected is set. When both queries resolve to the same
// node it returns a *SameNodeError; when the path is longer than maxHops (and
// maxHops > 0) it returns a *MaxHopsError.
func PathEdges(g *Graph, from, to string, maxHops int, undirected bool) (*PathResult, error) {
	a, err := g.resolve(from)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("no node matching %q", from)
	}
	b, err := g.resolve(to)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("no node matching %q", to)
	}
	if a.ID == b.ID {
		return nil, &SameNodeError{From: from, To: to, ID: a.ID}
	}
	ids, ok := g.bfsPath(a.ID, b.ID, undirected)
	if !ok {
		return nil, noPathErr(a, b, undirected)
	}
	if hops := len(ids) - 1; maxHops > 0 && hops > maxHops {
		return nil, &MaxHopsError{MaxHops: maxHops, Hops: hops}
	}
	res := &PathResult{
		Nodes: make([]Node, 0, len(ids)),
		Edges: make([]PathEdge, 0, len(ids)-1),
	}
	for _, id := range ids {
		if n := g.byID[id]; n != nil {
			res.Nodes = append(res.Nodes, *n)
		} else {
			res.Nodes = append(res.Nodes, Node{ID: id})
		}
	}
	for i := 0; i+1 < len(ids); i++ {
		u, v := ids[i], ids[i+1]
		if l := g.edge[[2]string{u, v}]; l != nil {
			res.Edges = append(res.Edges, PathEdge{Relation: l.Relation, Confidence: l.Confidence, Forward: true})
		} else if l := g.edge[[2]string{v, u}]; l != nil {
			res.Edges = append(res.Edges, PathEdge{Relation: l.Relation, Confidence: l.Confidence, Forward: false})
		} else {
			res.Edges = append(res.Edges, PathEdge{})
		}
	}
	return res, nil
}

// noPathErr reports a failed search, wrapping ErrNoDirectedPath when the search
// followed edge direction so callers can offer the undirected retry.
func noPathErr(a, b *Node, undirected bool) error {
	if undirected {
		return fmt.Errorf("no path between %q and %q", a.Label, b.Label)
	}
	return fmt.Errorf("%w between %q and %q", ErrNoDirectedPath, a.Label, b.Label)
}

// bfsPath returns the node IDs on a shortest path from aID to bID (inclusive,
// ordered from->to), or ok=false when none exists. It follows edge direction
// unless undirected is set.
func (g *Graph) bfsPath(aID, bID string, undirected bool) ([]string, bool) {
	prev := map[string]string{aID: ""}
	queue := []string{aID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == bID {
			break
		}
		nbrs := make([]string, 0, len(g.adj[cur]))
		for nb := range g.adj[cur] {
			nbrs = append(nbrs, nb)
		}
		sort.Strings(nbrs)
		for _, nb := range nbrs {
			if !undirected && g.edge[[2]string{cur, nb}] == nil {
				continue // edge points nb -> cur; not traversable directed
			}
			if _, seen := prev[nb]; !seen {
				prev[nb] = cur
				queue = append(queue, nb)
			}
		}
	}
	if _, ok := prev[bID]; !ok {
		return nil, false
	}
	var ids []string
	for cur := bID; cur != ""; cur = prev[cur] {
		ids = append(ids, cur)
	}
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 { // reverse to from->to order
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids, true
}

// AmbiguousError is returned by resolve when a query matches more than one
// node, so answering would mean guessing.
type AmbiguousError struct {
	Query      string
	Candidates []string // "label (file:line)" per match, capped
	More       int      // matches omitted from Candidates
}

func (e *AmbiguousError) Error() string {
	msg := fmt.Sprintf("%q is ambiguous, matches: %s", e.Query, strings.Join(e.Candidates, ", "))
	if e.More > 0 {
		msg += fmt.Sprintf(" (and %d more)", e.More)
	}
	return msg + "; disambiguate with path/to/file::Symbol or the exact node ID"
}

// maxCandidates caps how many matches an AmbiguousError lists.
const maxCandidates = 10

// ambiguous builds an *AmbiguousError from more than one matching node.
func ambiguous(s string, hits []*Node) *AmbiguousError {
	e := &AmbiguousError{Query: s}
	for _, n := range hits {
		if len(e.Candidates) == maxCandidates {
			e.More = len(hits) - maxCandidates
			break
		}
		c := n.Label
		if l := loc(n); l != "" {
			c += " (" + l + ")"
		}
		e.Candidates = append(e.Candidates, c)
	}
	return e
}

// resolve finds a node by exact ID, then by a path::Symbol qualifier, then by
// exact (case-insensitive) label, then by a case-insensitive label or ID
// substring. Matching more than one node at any tier is an *AmbiguousError
// rather than an arbitrary pick; matching none returns (nil, nil).
func (g *Graph) resolve(s string) (*Node, error) {
	if n, ok := g.byID[s]; ok {
		return n, nil
	}
	if i := strings.LastIndex(s, "::"); i > 0 && i+2 < len(s) {
		path, sym := strings.ToLower(s[:i]), strings.ToLower(s[i+2:])
		var hits []*Node
		for j := range g.Nodes {
			n := &g.Nodes[j]
			if strings.ToLower(n.Label) == sym && strings.HasSuffix(strings.ToLower(n.SourceFile), path) {
				hits = append(hits, n)
			}
		}
		if len(hits) > 0 {
			if len(hits) > 1 {
				return nil, ambiguous(s, hits)
			}
			return hits[0], nil
		}
	}
	low := strings.ToLower(s)
	var exact []*Node
	for i := range g.Nodes {
		if strings.ToLower(g.Nodes[i].Label) == low {
			exact = append(exact, &g.Nodes[i])
		}
	}
	if len(exact) > 0 {
		if len(exact) > 1 {
			return nil, ambiguous(s, exact)
		}
		return exact[0], nil
	}
	var subs []*Node
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if strings.Contains(strings.ToLower(n.Label), low) || strings.Contains(strings.ToLower(n.ID), low) {
			subs = append(subs, n)
		}
	}
	switch len(subs) {
	case 0:
		return nil, nil
	case 1:
		return subs[0], nil
	default:
		return nil, ambiguous(s, subs)
	}
}

// edgeLoc formats the call site an edge was extracted from, if it has one.
func edgeLoc(l Link) string {
	if l.SourceLocation == "" || l.SourceFile == "" {
		return ""
	}
	return l.SourceFile + ":" + strings.TrimPrefix(l.SourceLocation, "L")
}

func loc(n *Node) string {
	if n.SourceFile == "" {
		return ""
	}
	if n.SourceLocation == "" {
		return n.SourceFile
	}
	return n.SourceFile + ":" + strings.TrimPrefix(n.SourceLocation, "L")
}
