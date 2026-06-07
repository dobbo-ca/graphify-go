package export

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
	"github.com/dobbo-ca/graphify-go/internal/security"
)

// metaThreshold is the node count above which the viewer opens on an aggregated
// community meta-graph instead of rendering every node. Below it the viewer is
// node-level (search + click-to-inspect + community legend filter), matching how
// the Python original navigates: it renders all nodes up to MAX_NODES_FOR_VIZ
// (5000) and digs into communities via the legend show/hide, never aggregating.
//
// We match that 5000 ceiling, so for essentially every real repo the experience
// is the Python node-level one. The difference: past 5000 the Python tool just
// errors out ("too large for HTML viz"); we instead degrade to a directory-named
// community overview that drills into a community's node-level subgraph on click,
// so the viewer stays useful rather than failing. Use `graphify query`/`explain`
// for raw detail at any size.
const metaThreshold = 5000

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
	name  string
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

// communityName labels a community by the directory most of its nodes live in
// (shortened to the last two path segments), falling back to its highest-degree
// member's label, then to "Community N". This is what makes the meta overview
// readable — bubbles say "internal/extract" instead of "Community 12".
func communityName(g *model.Graph, cid int, members []string) string {
	dirCount := map[string]int{}
	bestDir, bestN := "", 0
	topNode, topDeg := "", -1
	for _, id := range members {
		n := g.Nodes[id]
		if n == nil {
			continue
		}
		if d := path.Dir(n.SourceFile); d != "" && d != "." {
			dirCount[d]++
			if dirCount[d] > bestN || (dirCount[d] == bestN && d < bestDir) {
				bestDir, bestN = d, dirCount[d]
			}
		}
		if deg := g.Degree(id); deg > topDeg {
			topNode, topDeg = security.SanitizeLabel(n.Label), deg
		}
	}
	switch {
	case bestDir != "":
		return shortenPath(bestDir)
	case topNode != "":
		return topNode
	default:
		return fmt.Sprintf("Community %d", cid)
	}
}

// shortenPath keeps the last two segments of a path so labels stay compact.
func shortenPath(p string) string {
	p = strings.TrimPrefix(p, "./")
	segs := strings.Split(p, "/")
	if len(segs) > 2 {
		segs = segs[len(segs)-2:]
	}
	return security.SanitizeLabel(strings.Join(segs, "/"))
}

// ToHTML writes a self-contained vis-network viewer. The layout solves off-screen
// (avoidOverlap separates clusters) and is shown already settled and frozen — no
// spinning, no perpetual jitter. A search box and click-to-inspect panel (with
// neighbours grouped by relation) make the graph navigable, mirroring the Python
// original. Large graphs open on a named community overview and drill into a
// community's node-level subgraph on click.
func ToHTML(g *model.Graph, communities map[int][]string, outPath string) error {
	nc := cluster.NodeCommunity(communities)
	meta := g.NumNodes() > metaThreshold

	// Node-level nodes/edges are always built: they're the initial view for small
	// graphs and the drill-down target for large ones.
	subNodes, subEdges, legend := buildNodeLevel(g, communities, nc)

	var rawNodes []vnode
	var rawEdges []vedge
	if meta {
		rawNodes, rawEdges = buildMeta(g, communities, nc)
	} else {
		rawNodes, rawEdges = subNodes, subEdges
		subNodes, subEdges = []vnode{}, []vedge{} // not needed; keep payload small
	}

	rawNodesJSON, _ := json.Marshal(rawNodes)
	rawEdgesJSON, _ := json.Marshal(rawEdges)
	subNodesJSON, _ := json.Marshal(subNodes)
	subEdgesJSON, _ := json.Marshal(subEdges)
	namesJSON, _ := json.Marshal(communityNames(g, communities))

	banner := ""
	if meta {
		banner = fmt.Sprintf("community overview — %d communities across %d nodes; each circle is one community (size = members, link width = coupling). Click a community to open it.", len(communities), g.NumNodes())
	}
	stats := fmt.Sprintf("%d nodes · %d edges · %d communities", g.NumNodes(), g.NumEdges(), len(communities))

	page := strings.NewReplacer(
		"/*NODES*/", string(rawNodesJSON),
		"/*EDGES*/", string(rawEdgesJSON),
		"/*SUBNODES*/", string(subNodesJSON),
		"/*SUBEDGES*/", string(subEdgesJSON),
		"/*NAMES*/", string(namesJSON),
		"/*META*/", fmt.Sprintf("%t", meta),
		"<!--LEGEND-->", legendHTML(legend),
		"<!--BANNER-->", banner,
		"<!--STATS-->", stats,
	).Replace(htmlTemplate)
	return os.WriteFile(outPath, []byte(page), 0o644)
}

func communityNames(g *model.Graph, communities map[int][]string) map[int]string {
	names := make(map[int]string, len(communities))
	for cid, members := range communities {
		names[cid] = communityName(g, cid, members)
	}
	return names
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
	return nodes, edges, legendRows(g, communities)
}

// buildMeta renders one named node per community (sized by members) with edges
// weighted by the number of cross-community connections.
func buildMeta(g *model.Graph, communities map[int][]string, nc map[string]int) ([]vnode, []vedge) {
	maxN := 1
	for _, members := range communities {
		if len(members) > maxN {
			maxN = len(members)
		}
	}
	var nodes []vnode
	for cid, members := range communities {
		name := communityName(g, cid, members)
		nodes = append(nodes, vnode{
			ID:    fmt.Sprintf("c%d", cid),
			Label: name,
			Title: fmt.Sprintf("%s — %d nodes (click to open)", name, len(members)),
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
	return nodes, edges
}

func colorFor(community int) string {
	if community < 0 {
		return "#888888"
	}
	return palette[community%len(palette)]
}

func legendRows(g *model.Graph, communities map[int][]string) []legendRow {
	rows := make([]legendRow, 0, len(communities))
	for cid, members := range communities {
		rows = append(rows, legendRow{cid: cid, count: len(members), name: communityName(g, cid, members)})
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
		fmt.Fprintf(&b, `<label class="li"><input type="checkbox" checked data-cid="%d"><span class="dot" style="background:%s"></span><span class="ln" data-cid="%d" title="%s">%s</span><span class="ct">%d</span></label>`,
			r.cid, colorFor(r.cid), r.cid, html.EscapeString(r.name), html.EscapeString(r.name), r.count)
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
 #legend .li{display:flex;align-items:center;gap:7px;padding:2px 2px;font-size:12px;border-radius:4px}
 #legend .li:hover{background:#23233a}
 #legend .dot{width:11px;height:11px;border-radius:50%;flex:0 0 auto}
 #legend .ln{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
 #legend.drill .ln{cursor:pointer}
 #legend.drill .ln:hover{color:#fff;text-decoration:underline}
 #legend .ct{margin-left:auto;color:#666;font-size:11px;flex:0 0 auto}
 #legend input{accent-color:#4E79A7;margin:0;flex:0 0 auto}
 #stats{flex:0 0 auto;border-top:1px solid #2a2a4e;padding-top:8px;color:#8a8aa5;font-size:11px}
 #help{bottom:12px;left:12px;padding:8px 12px;color:#9a9ab0;font-size:12px;line-height:1.5;max-width:46vw}
 #help b{color:#cfcfe0}
 #banner{top:12px;left:12px;padding:6px 10px;color:#bbb;font-size:12px;max-width:46vw}
 #banner code{color:#7da9d8}
 #nav{top:12px;left:12px;padding:6px 10px;display:none;align-items:center;gap:8px}
 #nav button{background:#23233a;color:#cfcfe0;border:1px solid #2a2a4e;border-radius:5px;padding:4px 9px;cursor:pointer;font-size:12px}
 #nav button:hover{background:#2c2c46}
 #nav #vtitle{color:#cfcfe0;font-size:12px}
</style></head>
<body>
<div id="g"></div>
<div id="banner" class="panel"><!--BANNER--></div>
<div id="nav" class="panel"><button id="back">↑ Overview</button><span id="vtitle"></span></div>
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
const RAW=/*NODES*/;          // initial view: community circles (meta) or node-level
const EDG=/*EDGES*/;
const SUB=/*SUBNODES*/;       // node-level nodes (populated only in meta mode)
const SUBE=/*SUBEDGES*/;      // node-level edges
const NAME=/*NAMES*/;         // community id -> display name
const NODELEVEL=META?SUB:RAW; // the real nodes, for search/inspect
const nlById={};NODELEVEL.forEach(n=>nlById[n.id]=n);
const esc=s=>String(s).replace(/[&<>"]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c]));

const helpEl=document.getElementById("help"), bannerEl=document.getElementById("banner");
const navEl=document.getElementById("nav"), vtitle=document.getElementById("vtitle");
const info=document.getElementById("info"), legendEl=document.getElementById("legend");
const PLACEHOLDER='<div class="muted">Click a node to inspect it.</div>';
function setHelp(node){helpEl.innerHTML=node
 ? 'scroll <b>zoom</b> · drag <b>pan</b> · click a node to <b>inspect</b> it · click empty space to reset · search at right'
 : 'scroll <b>zoom</b> · drag <b>pan</b> · click a community to <b>open</b> it · search any node at right';}
setHelp(!META);
if(!bannerEl.textContent.trim())bannerEl.style.display="none";
if(META)legendEl.classList.add("drill");

let curNodes=new vis.DataSet(RAW), curEdges=new vis.DataSet(EDG);
let mode=META?"meta":"node"; // "meta" overview, a community id (drilled), or "node" (small graph)
let pending=null;            // node id to focus once a drilled subgraph settles

const net=new vis.Network(document.getElementById("g"),{nodes:curNodes,edges:curEdges},{
 nodes:{shape:"dot",borderWidth:1.5},
 edges:{color:{color:"#3a3a5e",opacity:0.55},arrows:{to:{enabled:true,scaleFactor:0.4}},smooth:{type:"continuous",roundness:0.2},selectionWidth:3},
 interaction:{hover:true,tooltipDelay:120,hideEdgesOnDrag:true},
 // Solve off-screen with strong overlap avoidance, then freeze — never spins.
 physics:{enabled:true,solver:"forceAtlas2Based",
  forceAtlas2Based:{gravitationalConstant:-60,centralGravity:0.005,springLength:120,springConstant:0.08,damping:0.4,avoidOverlap:0.8},
  stabilization:{iterations:300,fit:true}}});
net.on("stabilizationIterationsDone",()=>{
 net.setOptions({physics:{enabled:false}});
 if(pending){const id=pending;pending=null;focusLocal(id);}
});

function setView(nodes,edges){
 curNodes=new vis.DataSet(nodes); curEdges=new vis.DataSet(edges);
 net.setOptions({physics:{enabled:true}});
 net.setData({nodes:curNodes,edges:curEdges});
}
function openCommunity(cid,focusId){
 const ns=SUB.filter(n=>n.comm===cid), ids=new Set(ns.map(n=>n.id));
 const es=SUBE.filter(e=>ids.has(e.from)&&ids.has(e.to));
 mode=cid; pending=focusId||null;
 navEl.style.display="flex"; bannerEl.style.display="none";
 vtitle.textContent=(NAME[cid]||("Community "+cid))+" · "+ns.length+" nodes";
 info.innerHTML=PLACEHOLDER; setHelp(true);
 setView(ns,es);
}
function backToOverview(){
 mode="meta"; pending=null;
 navEl.style.display="none";
 if(bannerEl.textContent.trim())bannerEl.style.display="";
 info.innerHTML='<div class="muted">Click a community to open it.</div>'; setHelp(false);
 setView(RAW,EDG);
}
document.getElementById("back").onclick=backToOverview;

// Inspect panel: metadata + neighbours grouped by relation, each clickable.
function showNode(id){
 const n=nlById[id]; if(!n){info.innerHTML=PLACEHOLDER;return;}
 let h='<div class="nm">'+esc(n.label)+'</div>';
 if(n.ftype)h+='<div class="kv"><b>Type</b>'+esc(n.ftype)+'</div>';
 h+='<div class="kv"><b>Community</b>'+esc(NAME[n.comm]||n.comm)+'</div>';
 if(n.sfile)h+='<div class="kv"><b>Source</b>'+esc(n.sfile)+(n.sloc?':'+esc(String(n.sloc).replace(/^L/,'')):'')+'</div>';
 if(n.deg!=null)h+='<div class="kv"><b>Degree</b>'+n.deg+'</div>';
 let lastG=null;
 (n.nbrs||[]).forEach(nb=>{
  if(nb.g!==lastG){h+='<div class="grp">'+esc(nb.g)+'</div>';lastG=nb.g;}
  const o=nlById[nb.id], col=o?o.color:"#888", lbl=o?o.label:nb.id;
  h+='<div class="nbr" style="border-left-color:'+col+'" data-id="'+esc(nb.id)+'">'+esc(lbl)+'</div>';
 });
 info.innerHTML=h;
 info.querySelectorAll('.nbr').forEach(el=>el.onclick=()=>goTo(el.dataset.id));
}

// goTo navigates to any node-level node, crossing into its community subgraph if
// we're in the overview or a different community.
function goTo(id){
 const n=nlById[id]; if(!n)return;
 if(META && mode!==n.comm){openCommunity(n.comm,id);return;}
 focusLocal(id);
}
function focusLocal(id){
 net.selectNodes([id]); net.focus(id,{scale:1.4,animation:{duration:400}});
 showNode(id); highlight(id);
}

// Highlight the node's neighbourhood within the current view; dim the rest.
function highlight(id){
 const keep=new Set(net.getConnectedNodes(id)); keep.add(id);
 curNodes.update(curNodes.getIds().map(nid=>({id:nid,opacity:keep.has(nid)?1:0.12})));
 const ke=new Set(net.getConnectedEdges(id));
 curEdges.update(curEdges.getIds().map(e=>({id:e,hidden:!ke.has(e)})));
}
function resetHL(){
 curNodes.update(curNodes.getIds().map(nid=>({id:nid,opacity:1})));
 curEdges.update(curEdges.getIds().map(e=>({id:e,hidden:false})));
 info.innerHTML=PLACEHOLDER; net.unselectAll();
}

net.on("click",p=>{
 if(p.nodes.length){
  const id=p.nodes[0];
  if(META && mode==="meta"){openCommunity(curNodes.get(id).comm,null);return;}
  focusLocal(id);
 }else if(mode!=="meta"){resetHL();}
});

// Live search over all node-level nodes; jump (and drill, if needed) to a pick.
const search=document.getElementById("search"), results=document.getElementById("results");
search.addEventListener("input",()=>{
 const q=search.value.toLowerCase().trim(); results.innerHTML="";
 if(!q)return;
 NODELEVEL.filter(n=>(n.norm||n.label.toLowerCase()).includes(q)).slice(0,20).forEach(n=>{
  const r=document.createElement("div"); r.className="r";
  r.innerHTML='<span style="color:'+n.color+'">●</span> '+esc(n.label)+(META?' <span style="color:#666">· '+esc(NAME[n.comm]||n.comm)+'</span>':'');
  r.onclick=()=>{goTo(n.id);search.value="";results.innerHTML="";};
  results.appendChild(r);
 });
});

// Legend: checkbox toggles a community's visibility in the current view; in the
// overview, clicking a community name opens it.
const hidden=new Set();
legendEl.querySelectorAll('input').forEach(cb=>{
 cb.addEventListener("change",()=>{const c=+cb.dataset.cid;cb.checked?hidden.delete(c):hidden.add(c);
  curNodes.update(curNodes.getIds().map(id=>{const n=curNodes.get(id);return {id:id,hidden:hidden.has(n.comm)};}));});
});
if(META)legendEl.querySelectorAll('.ln').forEach(el=>el.onclick=()=>{if(mode==="meta")openCommunity(+el.dataset.cid,null);});
</script></body></html>`
