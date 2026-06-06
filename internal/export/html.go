package export

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
	"github.com/dobbo-ca/graphify-go/internal/security"
)

// metaThreshold is the node count above which the viewer renders an aggregated
// community meta-graph (one node per community) instead of every node. Drawing
// thousands of individual nodes is an unreadable hairball; the macro view of how
// communities connect is what's actually useful at that scale (matches the
// Python original's aggregated view). Use `graphify query`/`explain` for detail.
const metaThreshold = 500

// palette colours communities by index.
var palette = []string{
	"#4E79A7", "#F28E2B", "#E15759", "#76B7B2", "#59A14F",
	"#EDC948", "#B07AA1", "#FF9DA7", "#9C755F", "#BAB0AC",
	"#86BCB6", "#D37295", "#FABFD2", "#B6992D", "#499894",
	"#D7B5A6", "#79706E", "#8CD17D", "#F1CE63", "#A0CBE8",
}

type vnode struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Title string  `json:"title"`
	Color string  `json:"color"`
	Size  float64 `json:"size"`
	Font  vfont   `json:"font"`
	Comm  int     `json:"comm"`
}

type vfont struct {
	Size  int    `json:"size"`
	Color string `json:"color"`
}

type vedge struct {
	From  string  `json:"from"`
	To    string  `json:"to"`
	Width float64 `json:"width"`
}

type legendRow struct {
	cid   int
	count int
}

// ToHTML writes a self-contained vis-network viewer. The layout solves off-screen
// (avoidOverlap separates clusters) and is shown already settled and frozen — no
// spinning, no perpetual jitter. Communities are coloured with a legend whose
// checkboxes show/hide each one; clicking a node isolates its community.
func ToHTML(g *model.Graph, communities map[int][]string, path string) error {
	nc := cluster.NodeCommunity(communities)
	var nodes []vnode
	var edges []vedge
	var legend []legendRow
	meta := g.NumNodes() > metaThreshold

	if meta {
		nodes, edges, legend = buildMeta(g, communities, nc)
	} else {
		nodes, edges, legend = buildNodeLevel(g, communities, nc)
	}

	nodesJSON, _ := json.Marshal(nodes)
	edgesJSON, _ := json.Marshal(edges)
	banner := ""
	if meta {
		banner = fmt.Sprintf("community overview — %d communities across %d nodes; each circle is one community (size = members, link width = coupling). Use <code>graphify query</code>/<code>explain</code> for node-level detail.", len(communities), g.NumNodes())
	}

	page := strings.NewReplacer(
		"/*NODES*/", string(nodesJSON),
		"/*EDGES*/", string(edgesJSON),
		"/*META*/", fmt.Sprintf("%t", meta),
		"<!--LEGEND-->", legendHTML(legend),
		"<!--BANNER-->", banner,
	).Replace(htmlTemplate)
	return os.WriteFile(path, []byte(page), 0o644)
}

// buildNodeLevel renders every node, sized by degree, with labels only on hubs.
func buildNodeLevel(g *model.Graph, communities map[int][]string, nc map[string]int) ([]vnode, []vedge, []legendRow) {
	maxDeg := 1
	for _, id := range g.NodeIDs() {
		if d := g.Degree(id); d > maxDeg {
			maxDeg = d
		}
	}
	var nodes []vnode
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		deg := g.Degree(id)
		fontSize := 12
		if float64(deg) < 0.15*float64(maxDeg) {
			fontSize = 0 // hide labels on low-degree nodes to cut clutter; hover still shows them
		}
		nodes = append(nodes, vnode{
			ID: id, Label: security.SanitizeLabel(n.Label),
			Title: security.SanitizeLabel(strings.TrimSpace(n.SourceFile + " " + n.SourceLocation)),
			Color: colorFor(nc[id]),
			Size:  10 + 30*float64(deg)/float64(maxDeg),
			Font:  vfont{Size: fontSize, Color: "#e0e0e0"},
			Comm:  nc[id],
		})
	}
	var edges []vedge
	for _, e := range g.Edges() {
		edges = append(edges, vedge{From: e.Source, To: e.Target, Width: 1})
	}
	return nodes, edges, legendRows(communities)
}

// buildMeta renders one node per community (sized by members) with edges weighted
// by the number of cross-community connections.
func buildMeta(g *model.Graph, communities map[int][]string, nc map[string]int) ([]vnode, []vedge, []legendRow) {
	maxN := 1
	for _, members := range communities {
		if len(members) > maxN {
			maxN = len(members)
		}
	}
	var nodes []vnode
	for cid, members := range communities {
		nodes = append(nodes, vnode{
			ID:    fmt.Sprintf("c%d", cid),
			Label: fmt.Sprintf("Community %d", cid),
			Title: fmt.Sprintf("%d nodes", len(members)),
			Color: colorFor(cid),
			Size:  12 + 38*float64(len(members))/float64(maxN),
			Font:  vfont{Size: 13, Color: "#e0e0e0"},
			Comm:  cid,
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	counts := map[[2]int]int{}
	for _, e := range g.Edges() {
		a, b := nc[e.Source], nc[e.Target]
		if a == b {
			continue
		}
		if a > b {
			a, b = b, a
		}
		counts[[2]int{a, b}]++
	}
	var edges []vedge
	for pair, w := range counts {
		edges = append(edges, vedge{
			From:  fmt.Sprintf("c%d", pair[0]),
			To:    fmt.Sprintf("c%d", pair[1]),
			Width: 1 + math.Log2(float64(w+1)),
		})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	return nodes, edges, legendRows(communities)
}

func colorFor(community int) string {
	if community < 0 {
		return "#888888"
	}
	return palette[community%len(palette)]
}

func legendRows(communities map[int][]string) []legendRow {
	rows := make([]legendRow, 0, len(communities))
	for cid, members := range communities {
		rows = append(rows, legendRow{cid: cid, count: len(members)})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count != rows[j].count {
			return rows[i].count > rows[j].count
		}
		return rows[i].cid < rows[j].cid
	})
	return rows
}

func legendHTML(rows []legendRow) string {
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, `<label class="li"><input type="checkbox" checked data-cid="%d"><span class="dot" style="background:%s"></span>Community %d<span class="ct">%d</span></label>`,
			r.cid, colorFor(r.cid), r.cid, r.count)
	}
	return b.String()
}

const htmlTemplate = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>graphify</title>
<script src="https://unpkg.com/vis-network/standalone/umd/vis-network.min.js"></script>
<style>
 html,body{height:100%;margin:0;font:13px -apple-system,Segoe UI,sans-serif}
 #g{position:absolute;inset:0;background:#0f0f1a}
 .panel{position:fixed;background:#1a1a2e;color:#ccc;border:1px solid #2a2a4e;border-radius:8px;z-index:2}
 #legend{top:12px;right:12px;max-height:78vh;overflow:auto;padding:8px 10px;width:210px}
 #legend h4{margin:0 0 6px;font-size:11px;letter-spacing:.05em;text-transform:uppercase;color:#8a8aa5}
 #legend .li{display:flex;align-items:center;gap:7px;padding:2px 2px;font-size:12px;cursor:pointer;border-radius:4px}
 #legend .li:hover{background:#23233a}
 #legend .dot{width:11px;height:11px;border-radius:50%;flex:0 0 auto}
 #legend .ct{margin-left:auto;color:#666;font-size:11px}
 #legend input{accent-color:#4E79A7;margin:0}
 #help{bottom:12px;left:12px;padding:8px 12px;color:#9a9ab0;font-size:12px;line-height:1.5;max-width:52vw}
 #help b{color:#cfcfe0}
 #banner{top:12px;left:12px;padding:6px 10px;color:#bbb;font-size:12px;max-width:52vw}
 #banner code{color:#7da9d8}
</style></head>
<body>
<div id="g"></div>
<div id="banner" class="panel"><!--BANNER--></div>
<div id="legend" class="panel"><h4>Communities</h4><!--LEGEND--></div>
<div id="help" class="panel"></div>
<script>
const META=/*META*/;
const RAW=/*NODES*/;
const nodes=new vis.DataSet(RAW);
const edges=new vis.DataSet(/*EDGES*/);
document.getElementById("help").innerHTML = META
 ? 'scroll <b>zoom</b> · drag <b>pan</b> · click a community to <b>focus</b> it · toggle communities in the legend'
 : 'scroll <b>zoom</b> · drag <b>pan</b> · click a node to <b>isolate its community</b> · click empty space to show all · toggle communities in the legend';
if(!document.getElementById("banner").textContent.trim())document.getElementById("banner").style.display="none";

const net=new vis.Network(document.getElementById("g"),{nodes,edges},{
 nodes:{shape:"dot",borderWidth:1.5},
 edges:{color:{color:"#3a3a5e",opacity:0.55},arrows:{to:{enabled:true,scaleFactor:0.4}},smooth:{type:"continuous",roundness:0.2},selectionWidth:3},
 interaction:{hover:true,tooltipDelay:120,hideEdgesOnDrag:true},
 // Solve the layout off-screen with strong overlap avoidance, then freeze it.
 // The graph is shown already settled and static — it never spins or jitters.
 physics:{enabled:true,solver:"forceAtlas2Based",
  forceAtlas2Based:{gravitationalConstant:-60,centralGravity:0.005,springLength:120,springConstant:0.08,damping:0.4,avoidOverlap:0.8},
  stabilization:{iterations:300,fit:true}}});
net.once("stabilizationIterationsDone",()=>net.setOptions({physics:{enabled:false}}));

// Legend checkboxes: show/hide whole communities.
const hidden=new Set();
function applyHidden(){nodes.update(RAW.map(n=>({id:n.id,hidden:hidden.has(n.comm)})));}
document.querySelectorAll('#legend input').forEach(cb=>{
 cb.addEventListener('change',()=>{const c=+cb.dataset.cid;cb.checked?hidden.delete(c):hidden.add(c);applyHidden();});
});

// Click: isolate the clicked node's community (node view) or focus a community
// circle (meta view). Click empty space to show everything again.
net.on("click",p=>{
 if(META){if(p.nodes.length)net.focus(p.nodes[0],{scale:1.3,animation:true});return;}
 if(p.nodes.length){
  const c=nodes.get(p.nodes[0]).comm;
  nodes.update(RAW.map(n=>({id:n.id,hidden:n.comm!==c})));
  net.fit({nodes:RAW.filter(n=>n.comm===c).map(n=>n.id),animation:true});
 }else{
  hidden.clear();
  document.querySelectorAll('#legend input').forEach(cb=>cb.checked=true);
  nodes.update(RAW.map(n=>({id:n.id,hidden:false})));
  net.fit({animation:true});
 }
});
</script></body></html>`
