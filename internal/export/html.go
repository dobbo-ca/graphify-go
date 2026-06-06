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

// palette is a categorical colour set; communities are coloured by index into it.
var palette = []string{
	"#4E79A7", "#F28E2B", "#E15759", "#76B7B2", "#59A14F",
	"#EDC948", "#B07AA1", "#FF9DA7", "#9C755F", "#BAB0AC",
	"#86BCB6", "#D37295", "#FABFD2", "#B6992D", "#499894",
	"#D7B5A6", "#79706E", "#8CD17D", "#F1CE63", "#A0CBE8",
}

const dimColor = "#23232f"

// ToHTML writes a self-contained force-directed viewer (vis-network from a CDN).
// The layout stabilises once and then freezes — it does not jitter forever.
// Nodes are coloured by community with a legend; clicking a node highlights its
// neighbourhood. For graphs larger than htmlNodeCap, only the highest-degree
// nodes (and edges among them) are shown.
func ToHTML(g *model.Graph, communities map[int][]string, path string) error {
	nc := cluster.NodeCommunity(communities)
	shown := topNodes(g, htmlNodeCap)
	truncated := len(shown) < g.NumNodes()
	keep := make(map[string]bool, len(shown))
	for _, id := range shown {
		keep[id] = true
	}

	type vnode struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Title string `json:"title"`
		Color string `json:"color"`
		C     string `json:"c"` // original colour, for restoring after a highlight
	}
	type vedge struct {
		From string `json:"from"`
		To   string `json:"to"`
	}

	var nodes []vnode
	shownPerCommunity := map[int]int{}
	for _, id := range shown {
		n := g.Nodes[id]
		col := colorFor(nc[id])
		shownPerCommunity[nc[id]]++
		nodes = append(nodes, vnode{
			ID: id, Label: security.SanitizeLabel(n.Label),
			Title: security.SanitizeLabel(strings.TrimSpace(n.SourceFile + " " + n.SourceLocation)),
			Color: col, C: col,
		})
	}
	var edges []vedge
	for _, e := range g.Edges() {
		if keep[e.Source] && keep[e.Target] {
			edges = append(edges, vedge{From: e.Source, To: e.Target})
		}
	}
	nodesJSON, _ := json.Marshal(nodes)
	edgesJSON, _ := json.Marshal(edges)

	banner := ""
	if truncated {
		banner = fmt.Sprintf("showing the %d highest-degree of %d nodes — use <code>graphify query</code> for the rest", len(shown), g.NumNodes())
	}

	page := htmlTemplate
	page = strings.NewReplacer(
		"/*NODES*/", string(nodesJSON),
		"/*EDGES*/", string(edgesJSON),
		"<!--LEGEND-->", legendHTML(shownPerCommunity),
		"<!--BANNER-->", banner,
	).Replace(page)
	return os.WriteFile(path, []byte(page), 0o644)
}

func colorFor(community int) string {
	if community < 0 {
		return "#888888"
	}
	return palette[community%len(palette)]
}

// legendHTML renders the community colour key, largest first.
func legendHTML(perCommunity map[int]int) string {
	cids := make([]int, 0, len(perCommunity))
	for cid := range perCommunity {
		cids = append(cids, cid)
	}
	sort.Slice(cids, func(i, j int) bool {
		if perCommunity[cids[i]] != perCommunity[cids[j]] {
			return perCommunity[cids[i]] > perCommunity[cids[j]]
		}
		return cids[i] < cids[j]
	})
	var b strings.Builder
	for _, cid := range cids {
		fmt.Fprintf(&b, `<div class="li"><span class="dot" style="background:%s"></span>Community %d <span class="ct">%d</span></div>`,
			colorFor(cid), cid, perCommunity[cid])
	}
	return b.String()
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
<style>
 html,body{height:100%;margin:0;font:13px -apple-system,Segoe UI,sans-serif}
 #g{position:absolute;inset:0;background:#0f0f1a}
 .panel{position:fixed;background:#1a1a2e;color:#ccc;border:1px solid #2a2a4e;border-radius:8px;z-index:2}
 #legend{top:12px;right:12px;max-height:70vh;overflow:auto;padding:10px 12px;width:190px}
 #legend h4{margin:0 0 8px;font-size:11px;letter-spacing:.05em;text-transform:uppercase;color:#8a8aa5}
 #legend .li{display:flex;align-items:center;gap:8px;padding:2px 0;font-size:12px}
 #legend .dot{width:11px;height:11px;border-radius:50%;flex:0 0 auto}
 #legend .ct{margin-left:auto;color:#666;font-size:11px}
 #help{bottom:12px;left:12px;padding:8px 12px;color:#9a9ab0;font-size:12px;line-height:1.5}
 #help b{color:#cfcfe0}
 #banner{top:12px;left:12px;padding:6px 10px;color:#bbb;font-size:12px}
 #banner code{color:#7da9d8}
</style></head>
<body>
<div id="g"></div>
<div id="banner" class="panel"><!--BANNER--></div>
<div id="legend" class="panel"><h4>Communities</h4><!--LEGEND--></div>
<div id="help" class="panel">scroll <b>zoom</b> · drag <b>pan</b> · click a node to <b>highlight its neighbours</b> · click empty space to reset</div>
<script>
if(!document.getElementById("banner").textContent.trim())document.getElementById("banner").style.display="none";
const RAW=/*NODES*/;
const nodes=new vis.DataSet(RAW);
const edges=new vis.DataSet(/*EDGES*/);
const net=new vis.Network(document.getElementById("g"),{nodes,edges},{
 nodes:{shape:"dot",size:11,font:{color:"#e0e0e0",size:12}},
 edges:{color:{color:"#3a3a5e",opacity:0.5},arrows:{to:{enabled:true,scaleFactor:0.4}},smooth:false},
 interaction:{hover:true,tooltipDelay:120},
 physics:{enabled:true,solver:"forceAtlas2Based",
  forceAtlas2Based:{gravitationalConstant:-50,centralGravity:0.01,springLength:120,springConstant:0.08},
  stabilization:{iterations:250,fit:true}}});
// Freeze the layout once it settles so the graph stays static instead of drifting.
net.once("stabilizationIterationsDone",()=>net.setOptions({physics:{enabled:false}}));
// Click a node: keep it and its neighbours coloured, dim everything else.
net.on("click",p=>{
 if(p.nodes.length){
  const id=p.nodes[0], keep=new Set([id]);
  net.getConnectedNodes(id).forEach(n=>keep.add(n));
  nodes.update(RAW.map(n=>({id:n.id,color:keep.has(n.id)?n.c:"` + dimColor + `"})));
 }else{
  nodes.update(RAW.map(n=>({id:n.id,color:n.c})));
 }
});
</script></body></html>`
