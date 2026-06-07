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
	// Inspector fields (node-level view only; omitted in the meta view).
	FileType  string      `json:"ftype,omitempty"`
	Source    string      `json:"sfile,omitempty"`
	Loc       string      `json:"sloc,omitempty"`
	Degree    int         `json:"deg,omitempty"`
	Norm      string      `json:"norm,omitempty"`
	Neighbors []vneighbor `json:"nbrs,omitempty"`
}

type vfont struct {
	Size  int    `json:"size"`
	Color string `json:"color"`
}

// vneighbor is one adjacent node, already grouped by relation+direction
// ("Calls", "Called by", "Imports", …) so the panel mirrors `graphify explain`.
type vneighbor struct {
	ID  string `json:"id"`
	Grp string `json:"g"`
}

type vedge struct {
	From   string   `json:"from"`
	To     string   `json:"to"`
	Width  float64  `json:"width"`
	Dashes bool     `json:"dashes,omitempty"`
	Title  string   `json:"title,omitempty"`
	Color  *vecolor `json:"color,omitempty"`
}

type vecolor struct {
	Color   string  `json:"color"`
	Opacity float64 `json:"opacity"`
}

type legendRow struct {
	cid   int
	count int
}

// groupOrder fixes the display order of neighbour groups in the inspect panel.
var groupOrder = []string{
	"Calls", "Called by", "Imports", "Imported by",
	"Contains", "Contained by", "References", "Referenced by",
	"Depends on", "Depended on by",
}

func groupPrio(g string) int {
	for i, s := range groupOrder {
		if s == g {
			return i
		}
	}
	return len(groupOrder)
}

// neighborGroup names a neighbour bucket from the edge relation and whether the
// inspected node is the edge's source (outgoing) or target (incoming).
func neighborGroup(relation string, outgoing bool) string {
	switch relation {
	case "calls":
		if outgoing {
			return "Calls"
		}
		return "Called by"
	case "imports", "imports_from":
		if outgoing {
			return "Imports"
		}
		return "Imported by"
	case "contains":
		if outgoing {
			return "Contains"
		}
		return "Contained by"
	case "references":
		if outgoing {
			return "References"
		}
		return "Referenced by"
	case "depends_on":
		if outgoing {
			return "Depends on"
		}
		return "Depended on by"
	default:
		if outgoing {
			return relation + " →"
		}
		return "← " + relation
	}
}

// ToHTML writes a self-contained vis-network viewer. The layout solves off-screen
// (avoidOverlap separates clusters) and is shown already settled and frozen — no
// spinning, no perpetual jitter. Communities are coloured with a legend whose
// checkboxes show/hide each one; a search box and a click-to-inspect panel (with
// clickable neighbours grouped by relation) make the graph navigable, mirroring
// the Python original.
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
	stats := fmt.Sprintf("%d nodes · %d edges · %d communities", g.NumNodes(), g.NumEdges(), len(communities))

	page := strings.NewReplacer(
		"/*NODES*/", string(nodesJSON),
		"/*EDGES*/", string(edgesJSON),
		"/*META*/", fmt.Sprintf("%t", meta),
		"<!--LEGEND-->", legendHTML(legend),
		"<!--BANNER-->", banner,
		"<!--STATS-->", stats,
	).Replace(htmlTemplate)
	return os.WriteFile(path, []byte(page), 0o644)
}

// buildNodeLevel renders every node, sized by degree, with labels only on hubs.
// Each node carries its inspect metadata and a relation-grouped neighbour list;
// edges encode confidence (solid/opaque when EXTRACTED, dashed/faint otherwise).
func buildNodeLevel(g *model.Graph, communities map[int][]string, nc map[string]int) ([]vnode, []vedge, []legendRow) {
	maxDeg := 1
	for _, id := range g.NodeIDs() {
		if d := g.Degree(id); d > maxDeg {
			maxDeg = d
		}
	}

	// Bucket each node's neighbours by relation+direction (mirrors explain).
	type tmpNbr struct {
		id, group string
		prio      int
	}
	nbrs := map[string][]tmpNbr{}
	add := func(owner, other, group string) {
		nbrs[owner] = append(nbrs[owner], tmpNbr{id: other, group: group, prio: groupPrio(group)})
	}
	for _, e := range g.Edges() {
		add(e.Source, e.Target, neighborGroup(e.Relation, true))
		add(e.Target, e.Source, neighborGroup(e.Relation, false))
	}

	var nodes []vnode
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		deg := g.Degree(id)
		fontSize := 12
		if float64(deg) < 0.15*float64(maxDeg) {
			fontSize = 0 // hide labels on low-degree nodes to cut clutter; hover still shows them
		}
		label := security.SanitizeLabel(n.Label)
		list := nbrs[id]
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].prio != list[j].prio {
				return list[i].prio < list[j].prio
			}
			return security.SanitizeLabel(g.Nodes[list[i].id].Label) < security.SanitizeLabel(g.Nodes[list[j].id].Label)
		})
		vn := make([]vneighbor, 0, len(list))
		for _, t := range list {
			vn = append(vn, vneighbor{ID: t.id, Grp: t.group})
		}
		nodes = append(nodes, vnode{
			ID: id, Label: label,
			Title:     security.SanitizeLabel(strings.TrimSpace(n.SourceFile + " " + n.SourceLocation)),
			Color:     colorFor(nc[id]),
			Size:      10 + 30*float64(deg)/float64(maxDeg),
			Font:      vfont{Size: fontSize, Color: "#e0e0e0"},
			Comm:      nc[id],
			FileType:  security.SanitizeLabel(n.FileType),
			Source:    security.SanitizeLabel(n.SourceFile),
			Loc:       security.SanitizeLabel(n.SourceLocation),
			Degree:    deg,
			Norm:      strings.ToLower(label),
			Neighbors: vn,
		})
	}

	var edges []vedge
	for _, e := range g.Edges() {
		extracted := e.Confidence == "EXTRACTED"
		width, opacity := 1.0, 0.35
		if extracted {
			width, opacity = 2.0, 0.7
		}
		edges = append(edges, vedge{
			From: e.Source, To: e.Target, Width: width, Dashes: !extracted,
			Title: security.SanitizeLabel(strings.TrimSpace(e.Relation + " · " + e.Confidence)),
			Color: &vecolor{Color: "#3a3a5e", Opacity: opacity},
		})
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
 #sidebar{top:12px;right:12px;bottom:12px;width:280px;display:flex;flex-direction:column;padding:10px;box-sizing:border-box;gap:8px}
 #search{width:100%;box-sizing:border-box;background:#0f0f1a;border:1px solid #2a2a4e;color:#e0e0e0;border-radius:6px;padding:7px 9px;font-size:13px;outline:none}
 #search:focus{border-color:#4E79A7}
 #results{max-height:170px;overflow:auto}
 #results .r{padding:5px 7px;border-radius:4px;cursor:pointer;font-size:12px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
 #results .r:hover{background:#23233a}
 #info{flex:0 0 auto;max-height:40vh;overflow:auto;border-top:1px solid #2a2a4e;padding-top:8px}
 #info .nm{font-size:14px;color:#fff;font-weight:600;margin-bottom:4px;word-break:break-word}
 #info .kv{font-size:12px;color:#aaa;margin:2px 0;word-break:break-word}
 #info .kv b{color:#cfcfe0;font-weight:600;margin-right:4px}
 #info .grp{margin-top:9px;font-size:10px;letter-spacing:.05em;text-transform:uppercase;color:#8a8aa5}
 .nbr{display:block;padding:3px 7px;margin:3px 0;border-left:3px solid #888;background:#23233a;border-radius:3px;cursor:pointer;font-size:12px;color:#ddd;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
 .nbr:hover{background:#2c2c46}
 .muted{color:#6a6a85;font-size:12px}
 #legend{flex:1 1 auto;overflow:auto;border-top:1px solid #2a2a4e;padding-top:8px}
 #legend h4{margin:0 0 6px;font-size:11px;letter-spacing:.05em;text-transform:uppercase;color:#8a8aa5}
 #legend .li{display:flex;align-items:center;gap:7px;padding:2px 2px;font-size:12px;cursor:pointer;border-radius:4px}
 #legend .li:hover{background:#23233a}
 #legend .dot{width:11px;height:11px;border-radius:50%;flex:0 0 auto}
 #legend .ct{margin-left:auto;color:#666;font-size:11px}
 #legend input{accent-color:#4E79A7;margin:0}
 #stats{flex:0 0 auto;border-top:1px solid #2a2a4e;padding-top:8px;color:#8a8aa5;font-size:11px}
 #help{bottom:12px;left:12px;padding:8px 12px;color:#9a9ab0;font-size:12px;line-height:1.5;max-width:46vw}
 #help b{color:#cfcfe0}
 #banner{top:12px;left:12px;padding:6px 10px;color:#bbb;font-size:12px;max-width:46vw}
 #banner code{color:#7da9d8}
</style></head>
<body>
<div id="g"></div>
<div id="banner" class="panel"><!--BANNER--></div>
<div id="sidebar" class="panel">
 <input id="search" placeholder="Search nodes…" autocomplete="off" spellcheck="false">
 <div id="results"></div>
 <div id="info"><div class="muted">Click a node to inspect it.</div></div>
 <div id="legend"><h4>Communities</h4><!--LEGEND--></div>
 <div id="stats"><!--STATS--></div>
</div>
<div id="help" class="panel"></div>
<script>
const META=/*META*/;
const RAW=/*NODES*/;
const nodesDS=new vis.DataSet(RAW);
const edgesDS=new vis.DataSet(/*EDGES*/);
const byId={};RAW.forEach(n=>byId[n.id]=n);
const esc=s=>String(s).replace(/[&<>"]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c]));

document.getElementById("help").innerHTML = META
 ? 'scroll <b>zoom</b> · drag <b>pan</b> · click a community to <b>focus</b> it · search &amp; toggle communities at right'
 : 'scroll <b>zoom</b> · drag <b>pan</b> · click a node to <b>inspect</b> it · click empty space to reset · search at right';
if(!document.getElementById("banner").textContent.trim())document.getElementById("banner").style.display="none";

const net=new vis.Network(document.getElementById("g"),{nodes:nodesDS,edges:edgesDS},{
 nodes:{shape:"dot",borderWidth:1.5},
 edges:{color:{color:"#3a3a5e",opacity:0.55},arrows:{to:{enabled:true,scaleFactor:0.4}},smooth:{type:"continuous",roundness:0.2},selectionWidth:3},
 interaction:{hover:true,tooltipDelay:120,hideEdgesOnDrag:true},
 // Solve the layout off-screen with strong overlap avoidance, then freeze it.
 // The graph is shown already settled and static — it never spins or jitters.
 physics:{enabled:true,solver:"forceAtlas2Based",
  forceAtlas2Based:{gravitationalConstant:-60,centralGravity:0.005,springLength:120,springConstant:0.08,damping:0.4,avoidOverlap:0.8},
  stabilization:{iterations:300,fit:true}}});
net.once("stabilizationIterationsDone",()=>net.setOptions({physics:{enabled:false}}));
const EDGEIDS=edgesDS.getIds();

// Legend checkboxes: show/hide whole communities.
const hidden=new Set();
document.querySelectorAll('#legend input').forEach(cb=>{
 cb.addEventListener('change',()=>{const c=+cb.dataset.cid;cb.checked?hidden.delete(c):hidden.add(c);
  nodesDS.update(RAW.map(n=>({id:n.id,hidden:hidden.has(n.comm)})));});
});

// Live search: filter by label, show top 20, click to focus.
const search=document.getElementById("search"), results=document.getElementById("results");
search.addEventListener('input',()=>{
 const q=search.value.toLowerCase().trim(); results.innerHTML="";
 if(!q)return;
 RAW.filter(n=>(n.norm||n.label.toLowerCase()).includes(q)).slice(0,20).forEach(n=>{
  const r=document.createElement("div"); r.className="r";
  r.innerHTML='<span style="color:'+n.color+'">●</span> '+esc(n.label);
  r.onclick=()=>{focusNode(n.id);search.value="";results.innerHTML="";};
  results.appendChild(r);
 });
});

// Inspect panel: metadata + neighbours grouped by relation, each clickable.
const info=document.getElementById("info");
function showNode(id){
 const n=byId[id]; if(!n){info.innerHTML="";return;}
 let h='<div class="nm">'+esc(n.label)+'</div>';
 if(n.ftype)h+='<div class="kv"><b>Type</b>'+esc(n.ftype)+'</div>';
 h+='<div class="kv"><b>Community</b>'+n.comm+'</div>';
 if(n.sfile)h+='<div class="kv"><b>Source</b>'+esc(n.sfile)+(n.sloc?':'+esc(String(n.sloc).replace(/^L/,'')):'')+'</div>';
 if(n.deg!=null)h+='<div class="kv"><b>Degree</b>'+n.deg+'</div>';
 let lastG=null;
 (n.nbrs||[]).forEach(nb=>{
  if(nb.g!==lastG){h+='<div class="grp">'+esc(nb.g)+'</div>';lastG=nb.g;}
  const o=byId[nb.id], col=o?o.color:"#888", lbl=o?o.label:nb.id;
  h+='<div class="nbr" style="border-left-color:'+col+'" data-id="'+esc(nb.id)+'">'+esc(lbl)+'</div>';
 });
 info.innerHTML=h;
 info.querySelectorAll('.nbr').forEach(el=>el.onclick=()=>focusNode(el.dataset.id));
}

function focusNode(id){
 net.selectNodes([id]);
 net.focus(id,{scale:1.4,animation:{duration:400}});
 showNode(id);
 if(!META)highlight(id);
}

// Highlight the clicked node's neighbourhood: dim everything else, hide
// unrelated edges. Click empty space to reset.
function highlight(id){
 const keep=new Set(net.getConnectedNodes(id)); keep.add(id);
 nodesDS.update(RAW.map(n=>({id:n.id,opacity:keep.has(n.id)?1:0.12})));
 const ke=new Set(net.getConnectedEdges(id));
 edgesDS.update(EDGEIDS.map(e=>({id:e,hidden:!ke.has(e)})));
}
function reset(){
 nodesDS.update(RAW.map(n=>({id:n.id,opacity:1})));
 edgesDS.update(EDGEIDS.map(e=>({id:e,hidden:false})));
 info.innerHTML='<div class="muted">Click a node to inspect it.</div>';
 net.unselectAll();
}

net.on("click",p=>{
 if(p.nodes.length){
  if(META){net.focus(p.nodes[0],{scale:1.3,animation:true});showNode(p.nodes[0]);return;}
  focusNode(p.nodes[0]);
 }else if(!META){reset();}
});
</script></body></html>`
