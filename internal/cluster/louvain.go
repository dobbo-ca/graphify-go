package cluster

import (
	"math"

	"golang.org/x/exp/rand"
)

// deltaQTol is the smallest modularity gain that still counts as a move.
const deltaQTol = 1e-15

// lookAtAll turns the skip in localMove off, for the test that compares both.
var lookAtAll bool

// level is one step of the Louvain hierarchy: a weighted graph whose nodes are
// the communities of the level below.
type level struct {
	// Row u of the adjacency is nbr[off[u]:off[u+1]]: distinct neighbours,
	// never the node itself. Flat rows keep a sweep inside the cache.
	off    []int32
	nbr    []int32
	w      []float64 // edge weights, parallel to nbr; nil means every weight is 1
	self   []float64 // weight internal to each node
	comms  [][]int   // current communities, as node indices
	parent *level
}

// modularize runs Louvain over a deduplicated, self-loop-free undirected
// adjacency on nodes 0..n-1 and returns the communities as node-index groups.
//
// It reproduces gonum v0.15.1 community.Modularize(g, 1, rand.NewSource(seed))
// on an unweighted graph exactly, group and member order included: same visit
// order, random stream, candidate order and tie-breaks. Every weight is an
// integer held in a float64, so the sums are exact in any order, and the result
// does not depend on the order of a node's neighbours. Keeping a running weight
// per community makes one node move cost O(degree).
func modularize(adj [][]int) [][]int {
	n := len(adj)
	l := &level{off: make([]int32, n+1), self: make([]float64, n), comms: make([][]int, n)}
	for i, row := range adj {
		for _, v := range row {
			l.nbr = append(l.nbr, int32(v))
		}
		l.off[i+1] = int32(len(l.nbr))
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
	n := len(l.self)
	degree := make([]float64, n) // weighted, internal weight included
	var m2 float64
	for u := range degree {
		d := l.self[u]
		if l.w == nil {
			d += float64(l.off[u+1] - l.off[u])
		}
		for _, w := range l.weights(u) {
			d += w
		}
		degree[u] = d
		m2 += d
	}
	if m2 == 0 {
		return false
	}

	member := make([]int32, n) // community of each node
	pos := make([]int, n)      // index of each node within its community
	total := make([]float64, len(l.comms))
	for c, nodes := range l.comms {
		for i, u := range nodes {
			member[u], pos[u] = int32(c), i
			total[c] += degree[u]
		}
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	toComm := make([]float64, len(l.comms)) // weight from the node to each community
	seen := make([]bool, len(l.comms))
	var candidates []int32

	// A node that stayed put is skipped until its verdict could differ. The
	// verdict is a function of its neighbours' communities and of the totals
	// of those communities and its own, and every step of the arithmetic
	// below is monotone in a total: a larger one makes a community a worse
	// place to go and the node's own a better one to leave. So when a node
	// stays, the same test is run once more with its own total raised and
	// every other total lowered by a slack; if it still stays, it stays for
	// as long as no neighbour moves, its own total is within the slack, and
	// no more than the slack has left all communities together (which bounds
	// what any one of them lost). Looking at a node more often than needed
	// changes nothing.
	dirty := make([]bool, n)
	for u := range dirty {
		dirty[u] = true
	}
	ownLimit := make([]float64, n)  // look again when the node's own total passes this
	lostLimit := make([]float64, n) // or when lost does
	var lost float64                // weight that has left any community so far
	var candK []float64             // toComm of each candidate

	changed := false
	for {
		moved := false
		for i := range order[:n-1] {
			j := i + rnd(n-i)
			order[i], order[j] = order[j], order[i]
		}
		for _, u := range order {
			own := member[u]
			if !lookAtAll && !dirty[u] && total[own] <= ownLimit[u] && lost <= lostLimit[u] {
				continue
			}
			row, w := l.nbr[l.off[u]:l.off[u+1]], l.weights(u)
			dirty[u] = false
			candidates = append(candidates[:0], own)
			seen[own] = true
			for j, v := range row {
				c := member[v]
				if !seen[c] {
					seen[c] = true
					candidates = append(candidates, c)
				}
				if w == nil {
					toComm[c]++
				} else {
					toComm[c] += w[j]
				}
			}
			k := degree[u]
			var remove, kOwn float64
			add, dst := math.Inf(-1), int32(-1)
			candK = candK[:0]
			for _, c := range candidates {
				kc, tot := toComm[c], total[c]
				toComm[c], seen[c] = 0, false
				candK = append(candK, kc)
				if c == own {
					kOwn = kc
					remove = kc - k*(tot-k)/m2
				} else if dQ := kc - k*tot/m2; dQ > add || (dQ == add && c < dst) {
					// The lowest index wins a tie, as when gonum scans the
					// candidates in ascending order.
					add, dst = dQ, c
				}
			}
			if 2*(add-remove)/m2 <= deltaQTol {
				ownLimit[u], lostLimit[u] = total[own], lost
				if dst < 0 {
					// Nowhere to go, whatever the totals.
					ownLimit[u], lostLimit[u] = math.Inf(1), math.Inf(1)
				} else if slack := math.Floor((remove - add) * m2 / k / 2); slack >= 1 {
					// Half the room real arithmetic leaves to each side, whole
					// so the sums stay exact; kept only if the test agrees.
					worst := math.Inf(-1)
					for i, c := range candidates {
						if c != own {
							worst = max(worst, candK[i]-k*(total[c]-slack)/m2)
						}
					}
					if 2*(worst-(kOwn-k*(total[own]+slack-k)/m2))/m2 <= deltaQTol {
						ownLimit[u], lostLimit[u] = total[own]+slack, lost+slack
					}
				}
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
			lost += k
			dirty[u] = true
			for _, v := range row {
				dirty[v] = true
			}
		}
		if !moved {
			return changed
		}
	}
}

// weights returns the weights of row u, nil when every weight is 1.
func (l *level) weights(u int) []float64 {
	if l.w == nil {
		return nil
	}
	return l.w[l.off[u]:l.off[u+1]]
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
	r := &level{off: make([]int32, n+1), w: []float64{}, self: make([]float64, n), comms: make([][]int, n), parent: l}
	of := make([]int32, len(l.self))
	for c, nodes := range comms {
		r.comms[c] = []int{c}
		for _, u := range nodes {
			of[u] = int32(c)
		}
	}
	slot := make([]int32, n) // 1-based index into r.nbr, set only for the row being built
	for c, nodes := range comms {
		start := len(r.nbr)
		for _, u := range nodes {
			r.self[c] += l.self[u]
			w := l.weights(u)
			for j, v := range l.nbr[l.off[u]:l.off[u+1]] {
				x := 1.0
				if w != nil {
					x = w[j]
				}
				vc := of[v]
				if int(vc) == c {
					r.self[c] += x // seen from both ends, so 2w per pair
					continue
				}
				if slot[vc] == 0 {
					r.nbr = append(r.nbr, vc)
					r.w = append(r.w, 0)
					slot[vc] = int32(len(r.nbr))
				}
				r.w[slot[vc]-1] += x
			}
		}
		for _, vc := range r.nbr[start:] {
			slot[vc] = 0
		}
		r.off[c+1] = int32(len(r.nbr))
	}
	return r
}
