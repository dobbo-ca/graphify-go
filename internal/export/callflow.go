package export

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

// CallflowFromJSON writes an architecture page of Mermaid call-flow diagrams
// from a built graph.json. Louvain communities are the natural cohesive call
// clusters, so it renders one flowchart per community showing the `calls` edges
// inside it. The page is self-contained except for Mermaid, loaded from a CDN.
const (
	maxCallflowCommunities = 100 // cap rendered communities so the page stays usable
	maxCallflowEdges       = 150 // cap edges per diagram so Mermaid stays responsive
)

func CallflowFromJSON(jsonPath, outPath string) error {
	g, err := readGraphJSON(jsonPath)
	if err != nil {
		return err
	}

	label := map[string]string{}   // node id -> display label
	srcFile := map[string]string{} // node id -> source file
	community := map[string]int{}  // node id -> community (only nodes that have one)
	members := map[int][]string{}  // community -> node ids
	for _, n := range g.Nodes {
		label[n.ID] = n.Label
		srcFile[n.ID] = n.SourceFile
		if n.Community != nil {
			community[n.ID] = *n.Community
			members[*n.Community] = append(members[*n.Community], n.ID)
		}
	}

	// Bucket calls edges by the community shared by both endpoints.
	callsByCommunity := map[int][][2]string{}
	totalCalls := 0
	for _, e := range g.Links {
		if e.Relation != "calls" {
			continue
		}
		cs, okS := community[e.Source]
		ct, okT := community[e.Target]
		if okS && okT && cs == ct {
			callsByCommunity[cs] = append(callsByCommunity[cs], [2]string{e.Source, e.Target})
			totalCalls++
		}
	}

	// Communities with at least one internal call, ordered by call count desc
	// then id asc for a stable, useful page.
	var comms []int
	for c, edges := range callsByCommunity {
		if len(edges) > 0 {
			comms = append(comms, c)
		}
	}
	sort.Slice(comms, func(i, j int) bool {
		if len(callsByCommunity[comms[i]]) != len(callsByCommunity[comms[j]]) {
			return len(callsByCommunity[comms[i]]) > len(callsByCommunity[comms[j]])
		}
		return comms[i] < comms[j]
	})

	var b strings.Builder
	b.WriteString(callflowHead)
	fmt.Fprintf(&b, "<h1>Call-flow</h1>\n<p class=\"meta\">%d communities with internal calls · %d call edges</p>\n",
		len(comms), totalCalls)

	shown := comms
	if len(shown) > maxCallflowCommunities {
		shown = shown[:maxCallflowCommunities]
		fmt.Fprintf(&b, "<p class=\"meta\">showing the %d largest of %d call-flow communities</p>\n",
			maxCallflowCommunities, len(comms))
	}

	for _, c := range shown {
		edges := callsByCommunity[c]
		name := communityLabel(members[c], srcFile)
		truncated := ""
		if len(edges) > maxCallflowEdges {
			edges = edges[:maxCallflowEdges]
			truncated = fmt.Sprintf(", showing %d", maxCallflowEdges)
		}
		fmt.Fprintf(&b, "<h2>%s <span class=\"meta\">(%d nodes, %d calls%s)</span></h2>\n",
			htmlEscape(name), len(members[c]), len(callsByCommunity[c]), truncated)
		b.WriteString(mermaidFlowchart(edges, label))
	}

	b.WriteString(callflowFoot)
	return os.WriteFile(outPath, []byte(b.String()), 0o644)
}

// mermaidFlowchart renders the call edges as a Mermaid flowchart, assigning each
// referenced node a stable short id and emitting nodes in sorted order so the
// output is deterministic.
func mermaidFlowchart(edges [][2]string, label map[string]string) string {
	id := map[string]string{}
	var order []string
	idFor := func(node string) string {
		if s, ok := id[node]; ok {
			return s
		}
		s := fmt.Sprintf("n%d", len(id))
		id[node] = s
		order = append(order, node)
		return s
	}
	// Sort edges for deterministic id assignment.
	sort.Slice(edges, func(i, j int) bool {
		if edges[i][0] != edges[j][0] {
			return edges[i][0] < edges[j][0]
		}
		return edges[i][1] < edges[j][1]
	})

	var lines []string
	for _, e := range edges {
		s, t := idFor(e[0]), idFor(e[1])
		lines = append(lines, "  "+s+" --> "+t)
	}

	var b strings.Builder
	b.WriteString("<pre class=\"mermaid\">\nflowchart LR\n")
	for _, node := range order {
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", id[node], mermaidLabel(label[node]))
	}
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString("</pre>\n")
	return b.String()
}

// communityLabel names a community after the most common top-level directory of
// its members' source files, falling back to a generic label.
func communityLabel(memberIDs []string, srcFile map[string]string) string {
	counts := map[string]int{}
	for _, id := range memberIDs {
		f := srcFile[id]
		if f == "" {
			continue
		}
		dir := path.Dir(f)
		if dir == "." || dir == "/" {
			dir = path.Base(f)
		}
		counts[dir]++
	}
	best, bestN := "", 0
	for dir, n := range counts {
		if n > bestN || (n == bestN && dir < best) {
			best, bestN = dir, n
		}
	}
	if best == "" {
		return "community"
	}
	return best
}

// mermaidLabel sanitises a node label for use inside a Mermaid node. Characters
// that would break the Mermaid or surrounding HTML parse are replaced rather
// than escaped, which is good enough for a diagram.
func mermaidLabel(s string) string {
	r := strings.NewReplacer(
		`"`, `'`, "`", "'", "<", "(", ">", ")", "|", "/", "{", "(", "}", ")",
		"\n", " ", "\r", "", "&", "+", "[", "(", "]", ")",
	)
	return r.Replace(s)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

const callflowHead = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>graphify · call-flow</title>
<style>
  body { font-family: system-ui, -apple-system, sans-serif; margin: 2rem; max-width: 1100px; }
  h1 { margin-bottom: 0.2rem; }
  h2 { margin-top: 2.2rem; border-bottom: 1px solid #eee; padding-bottom: 0.3rem; }
  .meta { color: #777; font-weight: normal; font-size: 0.9em; }
  pre.mermaid { background: #fafafa; border: 1px solid #eee; border-radius: 6px; padding: 0.5rem; overflow-x: auto; }
</style>
<script type="module">
  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs';
  mermaid.initialize({ startOnLoad: true, securityLevel: 'loose', maxEdges: 5000, flowchart: { useMaxWidth: true } });
</script>
</head>
<body>
`

const callflowFoot = `</body>
</html>
`
