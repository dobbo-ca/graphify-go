package export

import (
	"math"
	"math/rand"
	"sort"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

type xy struct{ X, Y float64 }

// layoutPositions computes a deterministic force-directed layout (Fruchterman-
// Reingold) at build time. Baking coordinates into graph.html lets the browser
// render with physics disabled — the layout is solved once here instead of on
// every page open, which is what made large graphs slow to load.
//
// It's deterministic (fixed seed) so the committed graph.html is stable across
// rebuilds. Communities seed the initial placement, so connected clusters start
// near each other and the layout settles in far fewer iterations.
func layoutPositions(g *model.Graph, nc map[string]int) map[string]xy {
	ids := g.NodeIDs()
	n := len(ids)
	out := make(map[string]xy, n)
	if n == 0 {
		return out
	}
	if n == 1 {
		out[ids[0]] = xy{}
		return out
	}
	rng := rand.New(rand.NewSource(1)) // deterministic layout

	idx := make(map[string]int, n)
	for i, id := range ids {
		idx[id] = i
	}

	// side sizes the drawing area so node density stays constant; k is the ideal
	// edge length (the classic FR k = sqrt(area/n)).
	side := 60.0 * math.Sqrt(float64(n))
	k := side / math.Sqrt(float64(n))

	// Stable community indices for seeding (one ring position per community).
	cset := map[int]bool{}
	for _, id := range ids {
		cset[nc[id]] = true
	}
	cids := make([]int, 0, len(cset))
	for c := range cset {
		cids = append(cids, c)
	}
	sort.Ints(cids)
	ring := make(map[int]int, len(cids))
	for i, c := range cids {
		ring[c] = i
	}
	nc2ring := float64(len(cids))

	P := make([]xy, n)
	for i, id := range ids {
		ang := 2 * math.Pi * float64(ring[nc[id]]) / nc2ring
		P[i] = xy{
			X: side*0.35*math.Cos(ang) + rng.NormFloat64()*side*0.03,
			Y: side*0.35*math.Sin(ang) + rng.NormFloat64()*side*0.03,
		}
	}

	// Edges as index pairs (slice access keeps the O(n^2) inner loop fast).
	type pair struct{ a, b int }
	edges := make([]pair, 0, g.NumEdges())
	for _, e := range g.Edges() {
		edges = append(edges, pair{idx[e.Source], idx[e.Target]})
	}

	disp := make([]xy, n)
	temp := side / 10
	iters := frIterations(n)
	for it := 0; it < iters; it++ {
		for i := range disp {
			disp[i] = xy{}
		}
		// Repulsion between every pair: f = k^2 / d.
		for i := 0; i < n; i++ {
			pi := P[i]
			for j := i + 1; j < n; j++ {
				dx := pi.X - P[j].X
				dy := pi.Y - P[j].Y
				d2 := dx*dx + dy*dy
				if d2 < 1e-4 {
					dx, dy = rng.Float64()+0.1, rng.Float64()+0.1
					d2 = dx*dx + dy*dy
				}
				d := math.Sqrt(d2)
				f := k * k / d / d // (k^2/d) split across the unit vector (dx/d, dy/d)
				disp[i].X += dx * f
				disp[i].Y += dy * f
				disp[j].X -= dx * f
				disp[j].Y -= dy * f
			}
		}
		// Attraction along edges: f = d^2 / k.
		for _, e := range edges {
			dx := P[e.a].X - P[e.b].X
			dy := P[e.a].Y - P[e.b].Y
			d := math.Hypot(dx, dy)
			if d < 1e-6 {
				continue
			}
			f := d / k // (d^2/k) split across the unit vector
			disp[e.a].X -= dx * f
			disp[e.a].Y -= dy * f
			disp[e.b].X += dx * f
			disp[e.b].Y += dy * f
		}
		// Apply displacement, capped by the cooling temperature.
		for i := range P {
			dx, dy := disp[i].X, disp[i].Y
			d := math.Hypot(dx, dy)
			if d < 1e-9 {
				continue
			}
			lim := math.Min(d, temp)
			P[i].X += dx / d * lim
			P[i].Y += dy / d * lim
		}
		temp *= 0.95
	}

	for i, id := range ids {
		out[id] = P[i]
	}
	return out
}

// frIterations bounds the O(n^2) solve so build time stays reasonable: fewer
// iterations as the graph grows (community seeding carries the rest).
func frIterations(n int) int {
	it := 400_000_000 / (n * n)
	if it > 300 {
		it = 300
	}
	if it < 40 {
		it = 40
	}
	return it
}
