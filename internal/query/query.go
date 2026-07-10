// Package query answers questions against a built graph.json without rebuilding:
// find nodes by name, explain a node and its neighbours, and find the shortest
// dependency path between two nodes. These back the CLI commands the Claude
// skill uses instead of grepping the source tree.
package query

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/security"
)

// Graph is a loaded graph.json.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Links []Link `json:"links"`

	byID map[string]*Node
	adj  map[string]map[string]bool
	edge map[[2]string]*Link // directed (source,target) -> link, for relation lookup
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
	Source     string `json:"source"`
	Target     string `json:"target"`
	Relation   string `json:"relation"`
	Confidence string `json:"confidence"`
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

// Neighbor is an adjacent node in a given direction.
type Neighbor struct {
	ID, Label, Relation, Direction, Location string
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
	n := g.resolve(id)
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
		label, location := other, ""
		if o != nil {
			label, location = o.Label, loc(o)
		}
		nbrs = append(nbrs, Neighbor{ID: other, Label: label, Relation: l.Relation, Direction: dir, Location: location})
	}
	sort.Slice(nbrs, func(i, j int) bool {
		if nbrs[i].Relation != nbrs[j].Relation {
			return nbrs[i].Relation < nbrs[j].Relation
		}
		return nbrs[i].Label < nbrs[j].Label
	})
	return &Explanation{Node: n, Neighbors: nbrs}, nil
}

// Path returns the shortest path (by node labels/ids) between two nodes via BFS
// on the undirected graph, or an error if no path exists.
func Path(g *Graph, from, to string) ([]Node, error) {
	a, b := g.resolve(from), g.resolve(to)
	if a == nil {
		return nil, fmt.Errorf("no node matching %q", from)
	}
	if b == nil {
		return nil, fmt.Errorf("no node matching %q", to)
	}
	ids, ok := g.bfsPath(a.ID, b.ID)
	if !ok {
		return nil, fmt.Errorf("no path between %q and %q", a.Label, b.Label)
	}
	out := make([]Node, 0, len(ids))
	for _, id := range ids {
		if n := g.byID[id]; n != nil {
			out = append(out, *n)
		}
	}
	return out, nil
}

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

// PathEdges resolves from/to and returns the shortest undirected path between
// them annotated with each traversed edge's relation and confidence. It uses
// the same resolve() semantics as Path. When both queries resolve to the same
// node it returns a *SameNodeError; when the path is longer than maxHops (and
// maxHops > 0) it returns a *MaxHopsError.
func PathEdges(g *Graph, from, to string, maxHops int) (*PathResult, error) {
	a, b := g.resolve(from), g.resolve(to)
	if a == nil {
		return nil, fmt.Errorf("no node matching %q", from)
	}
	if b == nil {
		return nil, fmt.Errorf("no node matching %q", to)
	}
	if a.ID == b.ID {
		return nil, &SameNodeError{From: from, To: to, ID: a.ID}
	}
	ids, ok := g.bfsPath(a.ID, b.ID)
	if !ok {
		return nil, fmt.Errorf("no path between %q and %q", a.Label, b.Label)
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

// bfsPath returns the node IDs on a shortest undirected path from aID to bID
// (inclusive, ordered from->to), or ok=false when none exists.
func (g *Graph) bfsPath(aID, bID string) ([]string, bool) {
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

// resolve finds a node by exact ID, then exact (case-insensitive) label, then a
// unique case-insensitive label or ID substring.
func (g *Graph) resolve(s string) *Node {
	if n, ok := g.byID[s]; ok {
		return n
	}
	low := strings.ToLower(s)
	for i := range g.Nodes {
		if strings.ToLower(g.Nodes[i].Label) == low {
			return &g.Nodes[i]
		}
	}
	var hit *Node
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if strings.Contains(strings.ToLower(n.Label), low) || strings.Contains(strings.ToLower(n.ID), low) {
			if hit != nil {
				return nil // ambiguous
			}
			hit = n
		}
	}
	return hit
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
