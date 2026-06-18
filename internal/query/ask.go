package query

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/security"
)

// Ask answers a natural-language question against the graph using deterministic
// TF-IDF retrieval (no LLM, no network): it scores nodes against the question,
// picks seed nodes, runs a bounded BFS (or DFS when dfs is true) to the given
// depth, and renders the resulting subgraph as a token-budgeted text block.
//
// This mirrors upstream graphify's flagship `query` command (serve.py's
// _query_graph_text). The Go graph carries no edge `context` data, so upstream's
// context-filter machinery is intentionally omitted.
func Ask(g *Graph, question string, dfs bool, depth, tokenBudget int) string {
	terms := queryTerms(question)
	scored := scoreNodes(g, terms)
	seeds := pickSeeds(scored, 3, 0.2)
	if len(seeds) == 0 {
		return "No matching nodes found."
	}
	var nodes map[string]bool
	var edges [][2]string
	mode := "BFS"
	if dfs {
		mode = "DFS"
		nodes, edges = dfsTraverse(g, seeds, depth)
	} else {
		nodes, edges = bfsTraverse(g, seeds, depth)
	}
	seedLabels := make([]string, len(seeds))
	for i, id := range seeds {
		seedLabels[i] = nodeLabel(g, id)
	}
	header := fmt.Sprintf("Traversal: %s depth=%d | Start: [%s] | %d nodes found\n\n",
		mode, depth, strings.Join(seedLabels, ", "), len(nodes))
	return header + subgraphToText(g, nodes, edges, tokenBudget, seeds)
}

var wordRe = regexp.MustCompile(`\w+`)

// queryTerms splits a question into searchable lowercase tokens, dropping
// English words of two characters or fewer (mirrors upstream _query_terms +
// _is_searchable for the Latin-script case).
func queryTerms(question string) []string {
	var terms []string
	for _, raw := range strings.Fields(question) {
		for _, tok := range wordRe.FindAllString(strings.ToLower(raw), -1) {
			if isSearchable(tok) {
				terms = append(terms, tok)
			}
		}
	}
	return terms
}

// isSearchable reports whether a term is worth scoring: an all-ASCII-letter word
// must be longer than two chars; anything else is kept.
func isSearchable(term string) bool {
	for _, ch := range term {
		if ch < 'a' || ch > 'z' {
			return true
		}
	}
	return len(term) > 2
}

const (
	exactMatchBonus     = 1000.0
	prefixMatchBonus    = 100.0
	substringMatchBonus = 1.0
	sourceMatchBonus    = 0.5
)

// computeIDF returns IDF weights for the query terms. Common terms that appear
// in many node labels get low weights; rare identifiers get high weights.
func computeIDF(g *Graph, terms []string) map[string]float64 {
	n := float64(len(g.Nodes))
	if n == 0 {
		n = 1
	}
	df := map[string]int{}
	for _, t := range terms {
		if _, ok := df[t]; !ok {
			df[t] = 0
		}
	}
	for i := range g.Nodes {
		norm := normLabel(&g.Nodes[i])
		for t := range df {
			if strings.Contains(norm, t) {
				df[t]++
			}
		}
	}
	idf := make(map[string]float64, len(df))
	for t, d := range df {
		idf[t] = math.Log(1 + n/float64(1+d))
	}
	return idf
}

// scoreNodes ranks nodes by relevance to the query terms, returning
// (score, id) pairs sorted by score descending (ties broken toward the shorter
// label, then node id). Mirrors upstream _score_nodes.
func scoreNodes(g *Graph, terms []string) []scoredNode {
	idf := computeIDF(g, terms)
	joined := strings.Join(terms, " ")
	joinedW := 1.0
	for _, t := range terms {
		if w := idf[t]; w > joinedW {
			joinedW = w
		}
	}
	var out []scoredNode
	for i := range g.Nodes {
		nd := &g.Nodes[i]
		norm := normLabel(nd)
		bare := strings.TrimRight(norm, "()")
		source := strings.ToLower(nd.SourceFile)
		score := 0.0
		if joined != "" {
			nidLower := strings.ToLower(nd.ID)
			switch {
			case joined == norm || joined == bare || joined == nidLower:
				score += exactMatchBonus * 10 * joinedW
			case strings.HasPrefix(norm, joined) || strings.HasPrefix(bare, joined):
				score += prefixMatchBonus * 10 * joinedW
			}
		}
		for _, t := range terms {
			w := idf[t]
			switch {
			case t == norm || t == bare:
				score += exactMatchBonus * w
			case strings.HasPrefix(norm, t) || strings.HasPrefix(bare, t):
				score += prefixMatchBonus * w
			case strings.Contains(norm, t):
				score += substringMatchBonus * w
			}
			if strings.Contains(source, t) {
				score += sourceMatchBonus * w
			}
		}
		if score > 0 {
			out = append(out, scoredNode{score, nd.ID, len(labelOf(nd))})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		if out[i].labelLen != out[j].labelLen {
			return out[i].labelLen < out[j].labelLen
		}
		return out[i].id < out[j].id
	})
	return out
}

type scoredNode struct {
	score    float64
	id       string
	labelLen int
}

// pickSeeds selects up to maxK seed nodes, stopping once a candidate's score
// drops below gapRatio of the top score so high-frequency noise terms cannot
// steal seed slots from a dominant identifier match.
func pickSeeds(scored []scoredNode, maxK int, gapRatio float64) []string {
	if len(scored) == 0 {
		return nil
	}
	top := scored[0].score
	var seeds []string
	for i, s := range scored {
		if i >= maxK {
			break
		}
		if len(seeds) > 0 && s.score < top*gapRatio {
			break
		}
		seeds = append(seeds, s.id)
	}
	return seeds
}

// hubThreshold is the degree above which non-seed nodes are treated as hubs and
// not expanded as transit: the p99 of the degree distribution, floored at 50.
func hubThreshold(g *Graph) int {
	degrees := make([]int, 0, len(g.Nodes))
	for id := range g.adj {
		degrees = append(degrees, len(g.adj[id]))
	}
	if len(degrees) == 0 {
		return 50
	}
	sort.Ints(degrees)
	p99 := int(float64(len(degrees)) * 0.99)
	if p99 >= len(degrees) {
		p99 = len(degrees) - 1
	}
	if degrees[p99] > 50 {
		return degrees[p99]
	}
	return 50
}

// bfsTraverse expands outward from the seeds up to depth hops, skipping
// expansion through high-degree hubs (except seeds). It returns the visited node
// set and the edges traversed, in discovery order.
func bfsTraverse(g *Graph, seeds []string, depth int) (map[string]bool, [][2]string) {
	hub := hubThreshold(g)
	seedSet := toSet(seeds)
	visited := toSet(seeds)
	frontier := append([]string(nil), seeds...)
	var edges [][2]string
	for d := 0; d < depth; d++ {
		var next []string
		nextSet := map[string]bool{}
		for _, n := range frontier {
			if !seedSet[n] && len(g.adj[n]) >= hub {
				continue
			}
			for _, nb := range g.neighbors(n) {
				if !visited[nb] {
					if !nextSet[nb] {
						nextSet[nb] = true
						next = append(next, nb)
					}
					edges = append(edges, [2]string{n, nb})
				}
			}
		}
		for _, nb := range next {
			visited[nb] = true
		}
		frontier = next
	}
	return visited, edges
}

// dfsTraverse explores depth-first from the seeds up to depth hops, with the
// same hub-skipping rule as bfsTraverse.
func dfsTraverse(g *Graph, seeds []string, depth int) (map[string]bool, [][2]string) {
	hub := hubThreshold(g)
	seedSet := toSet(seeds)
	visited := map[string]bool{}
	var edges [][2]string
	type frame struct {
		node string
		d    int
	}
	var stack []frame
	for i := len(seeds) - 1; i >= 0; i-- {
		stack = append(stack, frame{seeds[i], 0})
	}
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[top.node] || top.d > depth {
			continue
		}
		visited[top.node] = true
		if !seedSet[top.node] && len(g.adj[top.node]) >= hub {
			continue
		}
		for _, nb := range g.neighbors(top.node) {
			if !visited[nb] {
				stack = append(stack, frame{nb, top.d + 1})
				edges = append(edges, [2]string{top.node, nb})
			}
		}
	}
	return visited, edges
}

// subgraphToText renders the traversed subgraph as NODE/EDGE lines, seeds first
// then the rest ordered by degree, cutting at the token budget (~3 chars/token).
func subgraphToText(g *Graph, nodes map[string]bool, edges [][2]string, tokenBudget int, seeds []string) string {
	charBudget := tokenBudget * 3
	seedSet := toSet(seeds)
	var ordered []string
	for _, s := range seeds {
		if nodes[s] {
			ordered = append(ordered, s)
		}
	}
	var rest []string
	for id := range nodes {
		if !seedSet[id] {
			rest = append(rest, id)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		di, dj := len(g.adj[rest[i]]), len(g.adj[rest[j]])
		if di != dj {
			return di > dj
		}
		return rest[i] < rest[j]
	})
	ordered = append(ordered, rest...)

	var lines []string
	for _, id := range ordered {
		nd := g.byID[id]
		label, src, loc, comm := id, "", "", ""
		if nd != nil {
			label = labelOf(nd)
			src, loc = nd.SourceFile, nd.SourceLocation
			if nd.Community != nil {
				comm = fmt.Sprintf("%d", *nd.Community)
			}
		}
		lines = append(lines, fmt.Sprintf("NODE %s [src=%s loc=%s community=%s]",
			security.SanitizeLabel(label), security.SanitizeLabel(src),
			security.SanitizeLabel(loc), security.SanitizeLabel(comm)))
	}
	for _, e := range edges {
		u, v := e[0], e[1]
		if !nodes[u] || !nodes[v] {
			continue
		}
		var relation, confidence string
		if l := g.edge[[2]string{u, v}]; l != nil {
			relation, confidence = l.Relation, l.Confidence
		} else if l := g.edge[[2]string{v, u}]; l != nil {
			relation, confidence = l.Relation, l.Confidence
		}
		lines = append(lines, fmt.Sprintf("EDGE %s --%s [%s]--> %s",
			security.SanitizeLabel(nodeLabel(g, u)), security.SanitizeLabel(relation),
			security.SanitizeLabel(confidence), security.SanitizeLabel(nodeLabel(g, v))))
	}
	output := strings.Join(lines, "\n")
	if charBudget > 0 && len(output) > charBudget {
		cut := strings.LastIndex(output[:charBudget], "\n")
		if cut <= 0 {
			cut = charBudget
		}
		total := 0
		for _, l := range lines {
			if strings.HasPrefix(l, "NODE ") {
				total++
			}
		}
		shown := strings.Count(output[:cut], "\nNODE ")
		if strings.HasPrefix(output, "NODE ") {
			shown++
		}
		output = output[:cut] + fmt.Sprintf(
			"\n... (truncated — %d more nodes cut by ~%d-token budget. Narrow the question or use `graphify explain` for a specific symbol)",
			total-shown, tokenBudget)
	}
	return output
}

// normLabel returns the lowercase norm_label of a node, falling back to a
// lowercased label.
func normLabel(n *Node) string {
	if n.NormLabel != "" {
		return strings.ToLower(n.NormLabel)
	}
	return strings.ToLower(n.Label)
}

// labelOf returns a node's label, falling back to its id.
func labelOf(n *Node) string {
	if n.Label != "" {
		return n.Label
	}
	return n.ID
}

// nodeLabel returns the label for a node id, falling back to the id itself.
func nodeLabel(g *Graph, id string) string {
	if n := g.byID[id]; n != nil {
		return labelOf(n)
	}
	return id
}

// neighbors returns a node's neighbour ids in sorted order, for deterministic
// traversal.
func (g *Graph) neighbors(id string) []string {
	ns := make([]string, 0, len(g.adj[id]))
	for n := range g.adj[id] {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return ns
}

func toSet(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}
