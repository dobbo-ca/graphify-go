// Package report renders GRAPH_REPORT.md, the human-readable audit trail that
// accompanies graph.json: corpus summary, the core abstractions, surprising
// connections, import cycles, and the community breakdown.
package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/analyze"
	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

const minCommunitySize = 3

// Generate builds the GRAPH_REPORT.md body.
func Generate(g *model.Graph, communities map[int][]string, root, builtAtCommit string) string {
	scores := cluster.Scores(g, communities)
	gods := analyze.GodNodes(g, 10)
	surprises := analyze.Surprising(g, communities, 5)
	cycles := analyze.ImportCycles(g, 5, 20)

	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }

	w("# Graph Report - %s", root)
	w("")
	w("## Summary")
	w("- %d nodes · %d edges · %d communities", g.NumNodes(), g.NumEdges(), len(communities))
	w("- %s", confidenceBreakdown(g))
	if builtAtCommit != "" {
		w("")
		w("## Graph Freshness")
		w("- Built from commit: `%s`", short(builtAtCommit))
		w("- Run `git rev-parse HEAD` and compare to check if the graph is stale.")
		w("- Run `graphify build .` after code changes to rebuild.")
	}

	w("")
	w("## God Nodes (most connected - your core abstractions)")
	if len(gods) == 0 {
		w("- None detected.")
	}
	for i, n := range gods {
		w("%d. `%s` - %d edges", i+1, n.Label, n.Degree)
	}

	w("")
	w("## Surprising Connections (you probably didn't know these)")
	if len(surprises) == 0 {
		w("- None detected - all connections are within the same source files.")
	}
	for _, s := range surprises {
		note := ""
		if s.Note != "" {
			note = "  _" + s.Note + "_"
		}
		w("- `%s` --%s--> `%s`  [%s]", s.Source, s.Relation, s.Target, s.Confidence)
		w("  %s → %s%s", s.SourceFiles[0], s.SourceFiles[1], note)
	}

	w("")
	w("## Import Cycles")
	if len(cycles) == 0 {
		w("- None detected.")
	}
	for _, c := range cycles {
		path := append(append([]string(nil), c.Files...), c.Files[0])
		w("- %d-file cycle: `%s`", len(c.Files), strings.Join(path, " -> "))
	}

	w("")
	w("## Communities (%d total)", len(communities))
	for _, cid := range sortedKeys(communities) {
		nodes := communities[cid]
		display := realLabels(g, nodes)
		if len(display) < minCommunitySize {
			continue
		}
		suffix := ""
		if len(display) > 8 {
			suffix = fmt.Sprintf(" (+%d more)", len(display)-8)
			display = display[:8]
		}
		w("")
		w("### Community %d", cid)
		w("Cohesion: %.2f", scores[cid])
		w("Nodes (%d): %s%s", len(realLabels(g, nodes)), strings.Join(display, ", "), suffix)
	}

	var ambiguous []*model.Edge
	for _, e := range g.Edges() {
		if e.Confidence == "AMBIGUOUS" {
			ambiguous = append(ambiguous, e)
		}
	}
	if len(ambiguous) > 0 {
		w("")
		w("## Ambiguous Edges - Review These")
		for _, e := range ambiguous {
			w("- `%s` --%s--> `%s`  [AMBIGUOUS]", g.Nodes[e.Source].Label, e.Relation, g.Nodes[e.Target].Label)
			w("  %s · relation: %s", e.SourceFile, e.Relation)
		}
	}

	isolated := isolatedNodes(g)
	thin := 0
	for _, nodes := range communities {
		if n := len(realLabels(g, nodes)); n > 0 && n < minCommunitySize {
			thin++
		}
	}
	ambPct := len(ambiguous) * 100 / max(g.NumEdges(), 1)
	if len(isolated)+thin > 0 || ambPct > 20 {
		w("")
		w("## Knowledge Gaps")
		if len(isolated) > 0 {
			labels := make([]string, 0, len(isolated))
			for _, id := range isolated {
				labels = append(labels, "`"+g.Nodes[id].Label+"`")
			}
			suffix := ""
			if len(labels) > 5 {
				suffix = fmt.Sprintf(" (+%d more)", len(labels)-5)
				labels = labels[:5]
			}
			w("- **%d isolated node(s):** %s%s", len(isolated), strings.Join(labels, ", "), suffix)
			w("  These have <=1 connection - possible missing edges or undocumented components.")
		}
		if thin > 0 {
			w("- **%d thin communities (<%d nodes) omitted from report** - run `graphify query` to explore isolated nodes.", thin, minCommunitySize)
		}
		if ambPct > 20 {
			w("- **High ambiguity: %d%% of edges are AMBIGUOUS.** Review the Ambiguous Edges section above.", ambPct)
		}
	}
	return b.String()
}

// isolatedNodes returns the IDs of real entities (non-file, non-concept) with at
// most one connection - candidates for missing edges or undocumented components.
func isolatedNodes(g *model.Graph) []string {
	var out []string
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		if g.Degree(id) > 1 {
			continue
		}
		if n.SourceFile == "" || n.Label == base(n.SourceFile) { // concept node or file hub
			continue
		}
		if !strings.Contains(base(n.SourceFile), ".") { // concept node (no real source file)
			continue
		}
		out = append(out, id)
	}
	return out
}

func confidenceBreakdown(g *model.Graph) string {
	counts := map[string]int{}
	total := 0
	for _, e := range g.Edges() {
		counts[e.Confidence]++
		total++
	}
	if total == 0 {
		total = 1
	}
	pct := func(k string) int { return counts[k] * 100 / total }
	return fmt.Sprintf("Extraction: %d%% EXTRACTED · %d%% INFERRED · %d%% AMBIGUOUS",
		pct("EXTRACTED"), pct("INFERRED"), pct("AMBIGUOUS"))
}

// realLabels returns the labels of nodes that are real entities (not file hubs
// or method stubs), for display in a community listing.
func realLabels(g *model.Graph, nodes []string) []string {
	var out []string
	for _, id := range nodes {
		n := g.Nodes[id]
		if n.SourceFile != "" && n.Label == base(n.SourceFile) {
			continue // file-hub node
		}
		out = append(out, n.Label)
	}
	return out
}

func sortedKeys(m map[int][]string) []int {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}

func base(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
