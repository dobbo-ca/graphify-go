package export

import (
	"math"
	"math/rand"
	"sort"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

type xy struct{ X, Y float64 }

// layoutPositions computes a deterministic force-directed layout at build time.
// Baking coordinates into graph.html lets the browser render with physics off —
// the layout is solved once here instead of on every page open, which is what
// made large graphs slow to load.
//
// The force model mirrors vis's forceAtlas2Based (the in-browser layout this
// replaced): linear spring attraction + degree-weighted repulsion + weak central
// gravity, integrated with velocity and damping. That spreads dense regions and
// gives hubs room, instead of Fruchterman-Reingold's d^2 attraction which packs
// connected nodes into a tight ball. Deterministic (fixed seed) so the committed
// graph.html is stable; communities seed the start so it settles quickly.
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

	// Constants mirror the vis forceAtlas2Based config that produced the layout
	// this replaced (gravitationalConstant -60, springLength 120, springConstant
	// 0.08, centralGravity 0.005, damping 0.4).
	const (
		repK      = 60.0  // repulsion strength
		springLen = 120.0 // ideal edge length
		springK   = 0.08  // spring stiffness
		centralG  = 0.005 // pull toward centre (contains stragglers)
		damping   = 0.4   // velocity lost per step
		dt        = 0.5   // integration timestep
		maxStep   = 120.0 // cap per-step movement for stability
	)

	idx := make(map[string]int, n)
	mass := make([]float64, n)
	for i, id := range ids {
		idx[id] = i
		mass[i] = 1 + 0.3*float64(g.Degree(id)) // hubs are heavier → more space, stay central
	}

	// Seed by community so connected clusters start together (faster settle).
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
	scale := springLen * math.Sqrt(float64(n))
	Px := make([]float64, n)
	Py := make([]float64, n)
	for i, id := range ids {
		ang := 2 * math.Pi * float64(ring[nc[id]]) / float64(len(cids))
		Px[i] = scale*0.3*math.Cos(ang) + rng.NormFloat64()*scale*0.05
		Py[i] = scale*0.3*math.Sin(ang) + rng.NormFloat64()*scale*0.05
	}

	type pair struct{ a, b int }
	edges := make([]pair, 0, g.NumEdges())
	for _, e := range g.Edges() {
		edges = append(edges, pair{idx[e.Source], idx[e.Target]})
	}

	Vx := make([]float64, n)
	Vy := make([]float64, n)
	Fx := make([]float64, n)
	Fy := make([]float64, n)
	iters := frIterations(n)
	for it := 0; it < iters; it++ {
		for i := range Fx {
			Fx[i], Fy[i] = 0, 0
		}
		// Degree-weighted repulsion between every pair (magnitude ~ repK·mi·mj/d).
		for i := 0; i < n; i++ {
			xi, yi, mi := Px[i], Py[i], mass[i]
			for j := i + 1; j < n; j++ {
				dx := xi - Px[j]
				dy := yi - Py[j]
				d2 := dx*dx + dy*dy
				if d2 < 1e-4 {
					dx, dy = rng.Float64()+0.1, rng.Float64()+0.1
					d2 = dx*dx + dy*dy
				}
				f := repK * mi * mass[j] / d2 // raw dx carries the extra 1/d for the unit vector
				Fx[i] += dx * f
				Fy[i] += dy * f
				Fx[j] -= dx * f
				Fy[j] -= dy * f
			}
		}
		// Linear spring attraction along edges (Hooke: springK·(d-springLen)).
		for _, e := range edges {
			dx := Px[e.a] - Px[e.b]
			dy := Py[e.a] - Py[e.b]
			d := math.Hypot(dx, dy)
			if d < 1e-6 {
				continue
			}
			f := springK * (d - springLen) / d
			Fx[e.a] -= dx * f
			Fy[e.a] -= dy * f
			Fx[e.b] += dx * f
			Fy[e.b] += dy * f
		}
		// Weak central gravity keeps disconnected/peripheral nodes from drifting off.
		for i := 0; i < n; i++ {
			d := math.Hypot(Px[i], Py[i])
			if d < 1e-6 {
				continue
			}
			f := centralG * mass[i] / d
			Fx[i] -= Px[i] * f
			Fy[i] -= Py[i] * f
		}
		// Integrate with damping; cap the step for stability.
		for i := 0; i < n; i++ {
			Vx[i] = (Vx[i] + Fx[i]/mass[i]*dt) * (1 - damping)
			Vy[i] = (Vy[i] + Fy[i]/mass[i]*dt) * (1 - damping)
			sx, sy := Vx[i]*dt, Vy[i]*dt
			if s := math.Hypot(sx, sy); s > maxStep {
				sx, sy = sx/s*maxStep, sy/s*maxStep
			}
			Px[i] += sx
			Py[i] += sy
		}
	}

	for i, id := range ids {
		out[id] = xy{Px[i], Py[i]}
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
	if it < 60 {
		it = 60
	}
	return it
}
