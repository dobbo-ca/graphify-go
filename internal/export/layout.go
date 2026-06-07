package export

import (
	"math"
	"math/rand"
	"sort"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

type xy struct{ X, Y float64 }

// Force-model constants derive from the vis forceAtlas2Based config that produced
// the in-browser layout this replaced (springLength 120, springConstant 0.08,
// gravitationalConstant -60, centralGravity 0.005, damping 0.4), tuned a little
// tighter (shorter edges, lighter repulsion) for a more compact result.
const (
	repK      = 42.0  // repulsion strength
	springLen = 92.0  // ideal edge length
	springK   = 0.08  // spring stiffness
	centralG  = 0.02  // pull toward centre (contains stragglers)
	damping   = 0.4   // velocity lost per step
	dt        = 0.5   // integration timestep
	maxStep   = 120.0 // cap per-step movement for stability
	bhTheta   = 0.9   // Barnes-Hut opening angle (larger = faster, coarser)
)

// layoutPositions computes a deterministic force-directed layout at build time.
// Baking coordinates into graph.html lets the browser render with physics off —
// solved once here instead of on every page open, which is what made large
// graphs slow to load.
//
// The force model mirrors vis's forceAtlas2Based (linear spring attraction +
// degree-weighted repulsion + central gravity), so dense regions spread and hubs
// get room instead of collapsing into a ball. Repulsion uses a Barnes-Hut
// quadtree (O(n log n)) so even ~5000-node graphs get hundreds of iterations and
// actually expand. A final radial clamp keeps disconnected stragglers from
// flying off. Deterministic (fixed seed) so the committed graph.html is stable.
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
	for it := 0; it < layoutIters(n); it++ {
		for i := range Fx {
			Fx[i], Fy[i] = 0, 0
		}
		// Degree-weighted repulsion via a Barnes-Hut quadtree (O(n log n)).
		root := buildQuad(Px, Py, mass)
		for i := 0; i < n; i++ {
			fx, fy := root.force(Px[i], Py[i], mass[i], bhTheta)
			Fx[i] += fx
			Fy[i] += fy
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
		// Central gravity (grows with distance) keeps stragglers contained.
		for i := 0; i < n; i++ {
			Fx[i] -= centralG * mass[i] * Px[i]
			Fy[i] -= centralG * mass[i] * Py[i]
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

	radialClamp(Px, Py)
	for i, id := range ids {
		out[id] = xy{Px[i], Py[i]}
	}
	return out
}

// layoutIters scales iterations down as the graph grows (Barnes-Hut keeps each
// cheap, so even the large end gets enough passes to expand).
func layoutIters(n int) int {
	switch {
	case n > 3000:
		return 300
	case n > 800:
		return 400
	default:
		return 500
	}
}

// radialClamp pulls any node sitting far outside the bulk back to the edge of it,
// so a handful of disconnected/peripheral nodes can't stretch the view. Operates
// around the centroid using the 90th-percentile radius as the reference.
func radialClamp(Px, Py []float64) {
	n := len(Px)
	if n < 12 {
		return
	}
	var cx, cy float64
	for i := range Px {
		cx += Px[i]
		cy += Py[i]
	}
	cx /= float64(n)
	cy /= float64(n)
	rad := make([]float64, n)
	for i := range Px {
		rad[i] = math.Hypot(Px[i]-cx, Py[i]-cy)
	}
	sorted := append([]float64(nil), rad...)
	sort.Float64s(sorted)
	cap := sorted[int(float64(n)*0.9)] * 1.8
	if cap <= 0 {
		return
	}
	for i := range Px {
		if rad[i] > cap {
			s := cap / rad[i]
			Px[i] = cx + (Px[i]-cx)*s
			Py[i] = cy + (Py[i]-cy)*s
		}
	}
}

// quad is a Barnes-Hut quadtree node accumulating a centre of mass.
type quad struct {
	cx, cy, hs       float64 // centre and half-size of this cell
	comX, comY, mass float64 // centre of mass
	n                int     // bodies contained
	bx, by, bm       float64 // the single body, when n==1 and not divided
	children         [4]*quad
	divided          bool
}

// buildQuad builds a quadtree over the points, sized to their bounding box.
func buildQuad(Px, Py, mass []float64) *quad {
	minX, minY := Px[0], Py[0]
	maxX, maxY := Px[0], Py[0]
	for i := range Px {
		minX, maxX = math.Min(minX, Px[i]), math.Max(maxX, Px[i])
		minY, maxY = math.Min(minY, Py[i]), math.Max(maxY, Py[i])
	}
	hs := math.Max(maxX-minX, maxY-minY)/2 + 1
	root := &quad{cx: (minX + maxX) / 2, cy: (minY + maxY) / 2, hs: hs}
	for i := range Px {
		root.insert(Px[i], Py[i], mass[i])
	}
	return root
}

func (q *quad) insert(x, y, m float64) {
	tm := q.mass + m
	q.comX = (q.comX*q.mass + x*m) / tm
	q.comY = (q.comY*q.mass + y*m) / tm
	q.mass = tm
	if q.n == 0 {
		q.bx, q.by, q.bm, q.n = x, y, m, 1
		return
	}
	if q.hs < 0.5 { // too small to split; bucket bodies together
		q.n++
		return
	}
	if !q.divided {
		q.subdivide()
		q.child(q.bx, q.by).insert(q.bx, q.by, q.bm)
		q.divided = true
	}
	q.child(x, y).insert(x, y, m)
	q.n++
}

func (q *quad) subdivide() {
	h := q.hs / 2
	q.children[0] = &quad{cx: q.cx - h, cy: q.cy - h, hs: h} // SW
	q.children[1] = &quad{cx: q.cx + h, cy: q.cy - h, hs: h} // SE
	q.children[2] = &quad{cx: q.cx - h, cy: q.cy + h, hs: h} // NW
	q.children[3] = &quad{cx: q.cx + h, cy: q.cy + h, hs: h} // NE
}

func (q *quad) child(x, y float64) *quad {
	i := 0
	if x >= q.cx {
		i |= 1
	}
	if y >= q.cy {
		i |= 2
	}
	return q.children[i]
}

// force returns the repulsion on a unit-mass body at (x,y); the caller scales by
// the body's own mass. Aggregates distant cells by their centre of mass.
func (q *quad) force(x, y, m, theta float64) (fx, fy float64) {
	if q == nil || q.n == 0 {
		return 0, 0
	}
	dx := x - q.comX
	dy := y - q.comY
	d2 := dx*dx + dy*dy
	if !q.divided {
		if d2 < 1e-4 { // self / coincident
			return 0, 0
		}
		f := m * repK * q.mass / d2
		return dx * f, dy * f
	}
	if (2*q.hs)*(2*q.hs) < theta*theta*d2 { // cell is far enough to aggregate
		f := m * repK * q.mass / d2
		return dx * f, dy * f
	}
	for _, c := range q.children {
		cx, cy := c.force(x, y, m, theta)
		fx += cx
		fy += cy
	}
	return fx, fy
}
