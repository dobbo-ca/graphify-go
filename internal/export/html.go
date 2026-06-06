package export

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
	"github.com/dobbo-ca/graphify-go/internal/security"
)

// htmlNodeCap bounds how many nodes the viewer renders. vis-network's physics
// simulation bogs down well before a few thousand nodes, so large graphs are
// reduced to their most-connected nodes (the ones worth seeing anyway).
const htmlNodeCap = 600

// ToHTML writes a small self-contained force-directed viewer (vis-network from
// a CDN) so a human can eyeball the graph. For graphs larger than htmlNodeCap,
// only the highest-degree nodes (and edges among them) are shown. Labels are
// sanitised before they are embedded in the page's JSON payload.
func ToHTML(g *model.Graph, communities map[int][]string, path string) error {
	nc := cluster.NodeCommunity(communities)
	shown := topNodes(g, htmlNodeCap)
	truncated := len(shown) < g.NumNodes()

	type vnode struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Group int    `json:"group"`
		Title string `json:"title"`
	}
	type vedge struct {
		From  string `json:"from"`
		To    string `json:"to"`
		Label string `json:"label"`
	}
	var nodes []vnode
	for _, id := range shown {
		n := g.Nodes[id]
		nodes = append(nodes, vnode{
			ID: id, Label: security.SanitizeLabel(n.Label), Group: nc[id],
			Title: security.SanitizeLabel(n.SourceFile + " " + n.SourceLocation),
		})
	}
	keep := make(map[string]bool, len(shown))
	for _, id := range shown {
		keep[id] = true
	}
	var edges []vedge
	for _, e := range g.Edges() {
		if keep[e.Source] && keep[e.Target] {
			edges = append(edges, vedge{From: e.Source, To: e.Target, Label: e.Relation})
		}
	}
	nodesJSON, _ := json.Marshal(nodes)
	edgesJSON, _ := json.Marshal(edges)

	banner := ""
	if truncated {
		banner = fmt.Sprintf("showing %d highest-degree of %d nodes — see GRAPH_REPORT.md or use `graphify query` for the rest", len(shown), g.NumNodes())
	}

	page := htmlTemplate
	page = strings.Replace(page, "/*NODES*/", string(nodesJSON), 1)
	page = strings.Replace(page, "/*EDGES*/", string(edgesJSON), 1)
	page = strings.Replace(page, "/*BANNER*/", security.SanitizeLabel(banner), 1)
	return os.WriteFile(path, []byte(page), 0o644)
}

// topNodes returns up to n node IDs with the highest degree (all of them if the
// graph is within the cap), in a deterministic order.
func topNodes(g *model.Graph, n int) []string {
	ids := append([]string(nil), g.NodeIDs()...)
	if len(ids) <= n {
		return ids
	}
	sort.SliceStable(ids, func(i, j int) bool {
		di, dj := g.Degree(ids[i]), g.Degree(ids[j])
		if di != dj {
			return di > dj
		}
		return ids[i] < ids[j]
	})
	return ids[:n]
}

const htmlTemplate = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>graphify</title>
<script src="https://unpkg.com/vis-network/standalone/umd/vis-network.min.js"></script>
<style>html,body,#g{height:100%;margin:0}#g{background:#111}
#b{position:fixed;top:0;left:0;right:0;padding:6px 10px;background:#222;color:#bbb;font:12px sans-serif;z-index:1}</style></head>
<body><div id="b">/*BANNER*/</div><div id="g"></div><script>
if(!document.getElementById("b").textContent.trim())document.getElementById("b").style.display="none";
const nodes=new vis.DataSet(/*NODES*/);
const edges=new vis.DataSet(/*EDGES*/);
new vis.Network(document.getElementById("g"),{nodes,edges},{
 nodes:{shape:"dot",size:10,font:{color:"#eee"}},
 edges:{arrows:"to",color:{opacity:0.4},font:{size:8,color:"#aaa"}},
 physics:{stabilization:true,barnesHut:{gravitationalConstant:-3000}}});
</script></body></html>`
