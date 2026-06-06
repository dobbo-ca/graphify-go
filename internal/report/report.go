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
	return b.String()
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
