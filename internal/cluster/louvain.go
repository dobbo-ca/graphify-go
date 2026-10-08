package cluster

import (
	"math"
	"slices"

	"golang.org/x/exp/rand"
)

// deltaQTol is the smallest modularity gain that still counts as a move.
const deltaQTol = 1e-15

// level is one step of the Louvain hierarchy: a weighted graph whose nodes are
// the communities of the level below.
type level struct {
	adj    [][]int     // distinct neighbours, never the node itself
	w      [][]float64 // edge weights, parallel to adj
	self   []float64   // weight internal to each node
	comms  [][]int     // current communities, as node indices
	parent *level
}

// modularize runs Louvain over a deduplicated, self-loop-free undirected
// adjacency on nodes 0..n-1 and returns the communities as node-index groups.
//
// It reproduces gonum v0.15.1 community.Modularize(g, 1, rand.NewSource(seed))
// on an unweighted graph exactly, group and member order included: same visit
// order, random stream, candidate order and tie-breaks. Every weight is an
// integer held in a float64, so the sums are exact in any order. Keeping a
// running weight per community makes one node move cost O(degree).
func modularize(adj [][]int) [][]int {
	n := len(adj)
	l := &level{adj: adj, w: make([][]float64, n), self: make([]float64, n), comms: make([][]int, n)}
	for i := range adj {
		l.w[i] = make([]float64, len(adj[i]))
		for j := range l.w[i] {
			l.w[i][j] = 1
		}
		l.comms[i] = []int{i}
	}
	rnd := rand.New(rand.NewSource(seed)).Intn
	for l.localMove(rnd) {
		l = l.reduce()
	}
	return l.expand()
}

// expand maps the communities back to node indices of the bottom level.
func (l *level) expand() [][]int {
	if l.parent == nil {
		return l.comms
	}
	sub := l.parent.expand()
	out := make([][]int, len(l.comms))
	for i, members := range l.comms {
		for _, m := range members {
			out[i] = append(out[i], sub[m]...)
		}
	}
	return out
}

// localMove moves nodes between communities until no move improves
// modularity, and reports whether any node moved.
func (l *level) localMove(rnd func(int) int) bool {
	n := len(l.adj)
	degree := make([]float64, n) // weighted, internal weight included
	var m2 float64
	for u := range l.adj {
		d := l.self[u]
		for _, w := range l.w[u] {
			d += w
		}
		degree[u] = d
		m2 += d
	}
	if m2 == 0 {
		return false
	}

	member := make([]int, n) // community of each node
	pos := make([]int, n)    // index of each node within its community
	total := make([]float64, len(l.comms))
	for c, nodes := range l.comms {
		for i, u := range nodes {
			member[u], pos[u] = c, i
			total[c] += degree[u]
		}
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	toComm := make([]float64, len(l.comms)) // weight from the node to each community
	seen := make([]bool, len(l.comms))
	var candidates []int

	changed := false
	for {
		moved := false
		for i := range order[:n-1] {
			j := i + rnd(n-i)
			order[i], order[j] = order[j], order[i]
		}
		for _, u := range order {
			own := member[u]
			candidates = append(candidates[:0], own)
			seen[own] = true
			for j, v := range l.adj[u] {
				c := member[v]
				if !seen[c] {
					seen[c] = true
					candidates = append(candidates, c)
				}
				toComm[c] += l.w[u][j]
			}
			// Ascending index with a strict > below: the lowest index wins a tie.
			slices.Sort(candidates)

			k := degree[u]
			var remove float64
			add, dst := math.Inf(-1), -1
			for _, c := range candidates {
				kc, tot := toComm[c], total[c]
				toComm[c], seen[c] = 0, false
				if c == own {
					remove = kc - k*(tot-k)/m2
				} else if dQ := kc - k*tot/m2; dQ > add {
					add, dst = dQ, c
				}
			}
			if 2*(add-remove)/m2 <= deltaQTol {
				continue
			}
			moved, changed = true, true

			// Swap-remove, as gonum does: member order decides later numbering.
			src := l.comms[own]
			last := src[len(src)-1]
			src[pos[u]], pos[last] = last, pos[u]
			l.comms[own] = src[:len(src)-1]
			member[u], pos[u] = dst, len(l.comms[dst])
			l.comms[dst] = append(l.comms[dst], u)
			total[own] -= k
			total[dst] += k
		}
		if !moved {
			return changed
		}
	}
}

// reduce builds the next level, one node per non-empty community.
func (l *level) reduce() *level {
	// Fill each empty slot from the end, as gonum does: it fixes the numbering.
	comms := l.comms
	for i := 0; i < len(comms); {
		if len(comms[i]) == 0 {
			comms[i] = comms[len(comms)-1]
			comms = comms[:len(comms)-1]
		} else {
			i++
		}
	}
	l.comms = comms

	n := len(comms)
	r := &level{adj: make([][]int, n), w: make([][]float64, n), self: make([]float64, n), comms: make([][]int, n), parent: l}
	of := make([]int, len(l.adj))
	for c, nodes := range comms {
		r.comms[c] = []int{c}
		for _, u := range nodes {
			of[u] = c
		}
	}
	slot := make([]int, n) // 1-based index into the current r.adj[c]
	for c, nodes := range comms {
		for _, u := range nodes {
			r.self[c] += l.self[u]
			for j, v := range l.adj[u] {
				vc := of[v]
				if vc == c {
					r.self[c] += l.w[u][j] // seen from both ends, so 2w per pair
					continue
				}
				if slot[vc] == 0 {
					r.adj[c] = append(r.adj[c], vc)
					r.w[c] = append(r.w[c], 0)
					slot[vc] = len(r.adj[c])
				}
				r.w[c][slot[vc]-1] += l.w[u][j]
			}
		}
		for _, vc := range r.adj[c] {
			slot[vc] = 0
		}
	}
	return r
}
