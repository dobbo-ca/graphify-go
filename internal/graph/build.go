// Package graph assembles extraction output into the undirected knowledge graph
// the rest of the pipeline operates on. It mirrors the Python original's
// build_from_json: deterministic edge ordering, ID-normalization remap for edge
// endpoints, dropping of dangling and phantom cross-language inferred calls, and
// first-seen-direction-wins for parallel edges.
package graph

import (
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/langfamily"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// Build assembles an extraction into a graph.
func Build(ext model.Extraction) *model.Graph {
	return BuildParts([][]model.Node{ext.Nodes}, [][]model.Edge{ext.Edges})
}

// BuildParts is Build for an extraction that was not joined into one slice
// (extract.ResolveParts). It only reads the parts, and the graph keeps the
// edges in them (model.AddEdgesAt): the caller must not write to those.
func BuildParts(nodes [][]model.Node, parts [][]model.Edge) *model.Graph {
	nNodes, nEdges := 0, 0
	for _, p := range nodes {
		nNodes += len(p)
	}
	for _, p := range parts {
		nEdges += len(p)
	}
	edges := make([]*model.Edge, 0, nEdges)
	for _, p := range parts {
		for i := range p {
			edges = append(edges, &p[i])
		}
	}
	// The sort only reads the edges, so it runs beside the node pass.
	order := make(chan []int32, 1)
	go func() { order <- edgeOrder(edges) }()

	g := model.New()
	g.Reserve(nNodes)
	for _, p := range nodes {
		for _, n := range p {
			addNodeKeepingProvenance(g, n)
		}
	}

	// Map a normalized form of every node ID back to the real ID, so an edge
	// endpoint that differs only in punctuation/casing still connects.
	ids := g.NodeIDs()
	norms := make([]string, len(ids))
	eachRun(len(ids), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			norms[i] = idutil.NormalizeID(ids[i])
		}
	})
	normToID := make(map[string]string, len(ids))
	for i, id := range ids {
		normToID[norms[i]] = id
	}

	// node finds an endpoint, falling back to its normalized form.
	node := func(id string) *model.Node {
		if n := g.Nodes[id]; n != nil {
			return n
		}
		if real, ok := normToID[idutil.NormalizeID(id)]; ok {
			return g.Nodes[real]
		}
		return nil
	}
	// Endpoints are looked up on every CPU; only the insert is serial.
	src, tgt := make([]uint32, len(edges)), make([]uint32, len(edges))
	eachRun(len(edges), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			src[i] = model.NoNode
			e := edges[i]
			s, t := node(e.Source), node(e.Target)
			if s == nil || t == nil {
				continue // dangling edge to an external/stdlib node — expected
			}
			cross := func() bool { return langfamily.Cross(s.SourceFile, t.SourceFile) }
			switch e.Relation {
			case model.RelCalls:
				if e.Confidence == model.ConfInferred && cross() {
					continue
				}
			case model.RelImports, model.RelImportsFrom, model.RelReferences:
				// Same phantom-collision guard as calls, but EXTRACTED: an
				// unresolved `import time` must not bind by bare stem onto a
				// `time.ts` across a language boundary (#1749). crossLanguage is
				// false when either endpoint is a non-code node (empty family),
				// so config/manifest/md -> code references survive.
				if cross() {
					continue
				}
			}
			src[i], _ = g.Index(s.ID)
			tgt[i], _ = g.Index(t.ID)
		}
	})
	g.AddEdgesAt(edges, <-order, src, tgt)
	return g
}

// eachRun calls fn on the runs of 0..n, one per CPU, side by side.
func eachRun(n int, fn func(lo, hi int)) {
	var wg sync.WaitGroup
	per := n/runtime.GOMAXPROCS(0) + 1
	for lo := 0; lo < n; lo += per {
		wg.Go(func() { fn(lo, min(lo+per, n)) })
	}
	wg.Wait()
}

// edgeOrder returns the indexes of edges in the order a stable sort by (source,
// target, relation) would leave them. Sorting indexes with the index as the
// last key gives that order without moving the 120-byte edges or going through
// reflection. The order is total, so runs sorted one per CPU and merged in
// pairs come out as one sort would leave them.
func edgeOrder(edges []*model.Edge) []int32 {
	order := make([]int32, len(edges))
	for i := range order {
		order[i] = int32(i)
	}
	cmp := func(i, j int32) int {
		a, b := edges[i], edges[j]
		if c := strings.Compare(a.Source, b.Source); c != 0 {
			return c
		}
		if c := strings.Compare(a.Target, b.Target); c != 0 {
			return c
		}
		if c := strings.Compare(a.Relation.String(), b.Relation.String()); c != 0 {
			return c
		}
		return int(i - j)
	}
	var wg sync.WaitGroup
	per := len(order)/runtime.GOMAXPROCS(0) + 1
	ends := []int{0} // run k is order[ends[k]:ends[k+1]]
	for lo := 0; lo < len(order); lo += per {
		hi := min(lo+per, len(order))
		ends = append(ends, hi)
		wg.Add(1)
		go func() {
			defer wg.Done()
			slices.SortFunc(order[lo:hi], cmp)
		}()
	}
	wg.Wait()
	buf := make([]int32, len(order))
	for len(ends) > 2 {
		next := []int{0}
		for k := 0; k+1 < len(ends); k += 2 {
			lo, mid, hi := ends[k], ends[k+1], ends[min(k+2, len(ends)-1)]
			next = append(next, hi)
			wg.Add(1)
			go func() {
				defer wg.Done()
				merge(buf[lo:hi], order[lo:mid], order[mid:hi], cmp)
			}()
		}
		wg.Wait()
		order, buf, ends = buf, order, next
	}
	return order
}

// merge fills dst with the sorted runs a and b in order.
func merge(dst, a, b []int32, cmp func(i, j int32) int) {
	for len(a) > 0 && len(b) > 0 {
		if cmp(a[0], b[0]) <= 0 {
			dst[0], a = a[0], a[1:]
		} else {
			dst[0], b = b[0], b[1:]
		}
		dst = dst[1:]
	}
	copy(dst[copy(dst, a):], b)
}

// addNodeKeepingProvenance inserts n, but on an ID collision it keeps whichever
// record actually has provenance instead of plain last-write-wins: an external
// import node (empty SourceFile) must not erase the real file:line node it
// happens to collide with. When both records name the same source file, the
// incoming one inherits any attribute it left empty.
func addNodeKeepingProvenance(g *model.Graph, n model.Node) {
	prev := g.Nodes[n.ID]
	if prev == nil {
		g.AddNode(n)
		return
	}
	if n.SourceFile == "" && prev.SourceFile != "" {
		return
	}
	if n.SourceFile == prev.SourceFile {
		if n.Label == "" {
			n.Label = prev.Label
		}
		if n.FileType == "" {
			n.FileType = prev.FileType
		}
		if n.SourceLocation == "" {
			n.SourceLocation = prev.SourceLocation
		}
		if n.ComputedName == "" {
			n.ComputedName = prev.ComputedName
		}
	}
	g.AddNode(n)
}
