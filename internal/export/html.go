package export

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/model"
	"github.com/dobbo-ca/graphify-go/internal/security"
)

// ToHTML writes a small self-contained force-directed viewer (vis-network from
// a CDN) so a human can eyeball the graph. Labels are sanitised before they are
// embedded in the page's JSON payload.
func ToHTML(g *model.Graph, communities map[int][]string, path string) error {
	nc := cluster.NodeCommunity(communities)
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
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		nodes = append(nodes, vnode{
			ID: id, Label: security.SanitizeLabel(n.Label), Group: nc[id],
			Title: security.SanitizeLabel(n.SourceFile + " " + n.SourceLocation),
		})
	}
	var edges []vedge
	for _, e := range g.Edges() {
		edges = append(edges, vedge{From: e.Source, To: e.Target, Label: e.Relation})
	}
	nodesJSON, _ := json.Marshal(nodes)
	edgesJSON, _ := json.Marshal(edges)

	page := htmlTemplate
	page = strings.Replace(page, "/*NODES*/", string(nodesJSON), 1)
	page = strings.Replace(page, "/*EDGES*/", string(edgesJSON), 1)
	return os.WriteFile(path, []byte(page), 0o644)
}

const htmlTemplate = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>graphify</title>
<script src="https://unpkg.com/vis-network/standalone/umd/vis-network.min.js"></script>
<style>html,body,#g{height:100%;margin:0}#g{background:#111}</style></head>
<body><div id="g"></div><script>
const nodes=new vis.DataSet(/*NODES*/);
const edges=new vis.DataSet(/*EDGES*/);
new vis.Network(document.getElementById("g"),{nodes,edges},{
 nodes:{shape:"dot",size:10,font:{color:"#eee"}},
 edges:{arrows:"to",color:{opacity:0.4},font:{size:8,color:"#aaa"}},
 physics:{stabilization:true,barnesHut:{gravitationalConstant:-3000}}});
</script></body></html>`
