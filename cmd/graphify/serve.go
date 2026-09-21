package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/analyze"
	"github.com/dobbo-ca/graphify-go/internal/model"
	"github.com/dobbo-ca/graphify-go/internal/query"
	"github.com/dobbo-ca/graphify-go/internal/security"
)

// mcpProtocolVersion is the MCP revision this stdio server speaks.
const mcpProtocolVersion = "2024-11-05"

// cmdServe runs a minimal JSON-RPC-over-stdio MCP server. It loads graph.json
// once into a resident process so an agent can issue many structured queries
// without paying the load cost on every shell-out of query/explain/path.
//
// Only the stdio transport is implemented (no HTTP, api-key, or hot-reload): a
// per-agent process re-launches cheaply, so those are unnecessary for the first
// cut. The 7 tools mirror upstream graphify's serve.py over the existing Go
// query/analyze primitives.
func cmdServe(args []string) error {
	positionals, graphPath := parseGraphFlag(args)
	if len(positionals) > 1 {
		return fmt.Errorf("usage: graphify serve [graph.json] [--graph path]")
	}
	if len(positionals) == 1 {
		graphPath = positionals[0]
	}
	g, err := query.Load(graphPath)
	if err != nil {
		return err
	}
	return newMCPServer(g).run(os.Stdin, os.Stdout)
}

// graphCtx is everything a tool call needs about one project's graph.
type graphCtx struct {
	g           *query.Graph
	communities map[int][]string // community id -> node ids, from persisted node fields
	god         *model.Graph     // in-memory adapter so analyze.GodNodes can filter file/concept nodes
}

// maxContexts caps how many other projects stay resident alongside the one
// serve was launched in, bounding memory for a long-lived server.
const maxContexts = 8

// mcpServer holds the graph context in scope for the current tool call plus the
// graphs loaded for other projects via the project_path argument.
type mcpServer struct {
	graphCtx
	contexts map[string]*graphCtx // absolute project dir -> loaded graph
}

func newMCPServer(g *query.Graph) *mcpServer {
	return &mcpServer{graphCtx: newGraphCtx(g), contexts: map[string]*graphCtx{}}
}

func newGraphCtx(g *query.Graph) graphCtx {
	return graphCtx{g: g, communities: communitiesOf(g), god: modelOf(g)}
}

// contextFor loads (and caches) the graph of another project directory, so one
// running server can answer about any repo carrying graphify-out/graph.json.
// The fixed graphify-out/graph.json suffix is the guard: a project_path can
// only ever reach a graph file, never arbitrary JSON on disk.
func (s *mcpServer) contextFor(project string) (*graphCtx, error) {
	key, err := filepath.Abs(project)
	if err != nil {
		return nil, err
	}
	if c, ok := s.contexts[key]; ok {
		return c, nil
	}
	g, err := query.Load(filepath.Join(key, "graphify-out", "graph.json"))
	if err != nil {
		return nil, err
	}
	if len(s.contexts) >= maxContexts {
		clear(s.contexts) // ponytail: flush-all instead of LRU eviction; port an LRU if anyone really serves >8 projects
	}
	c := newGraphCtx(g)
	s.contexts[key] = &c
	return &c, nil
}

// communitiesOf reconstructs the community -> node-id map from the community
// field persisted on each node (mirrors upstream _communities_from_graph).
func communitiesOf(g *query.Graph) map[int][]string {
	out := map[int][]string{}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.Community != nil {
			out[*n.Community] = append(out[*n.Community], n.ID)
		}
	}
	return out
}

// modelOf rebuilds an in-memory model.Graph from the loaded graph.json (no
// re-extraction) so god_nodes can reuse analyze.GodNodes' file/concept filtering.
func modelOf(g *query.Graph) *model.Graph {
	m := model.New()
	for i := range g.Nodes {
		n := &g.Nodes[i]
		m.AddNode(model.Node{
			ID: n.ID, Label: n.Label, FileType: n.FileType,
			SourceFile: n.SourceFile, SourceLocation: n.SourceLocation,
		})
	}
	for i := range g.Links {
		l := &g.Links[i]
		m.AddEdge(model.Edge{Source: l.Source, Target: l.Target, Relation: l.Relation, Confidence: l.Confidence})
	}
	return m
}

// JSON-RPC 2.0 message shapes. id is kept as json.RawMessage so it round-trips
// unchanged (per spec a request id may be a string or a number).
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// run reads newline-delimited JSON-RPC requests from in and writes responses to
// out (the MCP stdio transport). Notifications (no id) are processed without a
// reply. Blank lines are skipped — some MCP clients emit them between messages.
func (s *mcpServer) run(in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	enc := json.NewEncoder(out)
	for {
		line, err := r.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			if resp, ok := s.handle(line); ok {
				if encErr := enc.Encode(resp); encErr != nil {
					return encErr
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// handle dispatches one JSON-RPC message, returning the response to write and
// whether a response is expected (false for notifications).
func (s *mcpServer) handle(line []byte) (rpcResponse, bool) {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}}, true
	}
	notification := len(req.ID) == 0
	switch req.Method {
	case "initialize":
		return s.reply(req, map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "graphify", "version": version},
		}), true
	case "tools/list":
		return s.reply(req, map[string]any{"tools": toolDefs()}), true
	case "tools/call":
		return s.callTool(req), true
	case "notifications/initialized", "notifications/cancelled":
		return rpcResponse{}, false
	default:
		if notification {
			return rpcResponse{}, false
		}
		return s.fail(req, -32601, "method not found: "+req.Method), true
	}
}

func (s *mcpServer) reply(req rpcRequest, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *mcpServer) fail(req rpcRequest, code int, msg string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: code, Message: msg}}
}

// textResult wraps a tool's text output in the MCP content envelope.
func (s *mcpServer) textResult(req rpcRequest, text string) rpcResponse {
	return s.reply(req, map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	})
}

// callTool routes a tools/call request to the matching handler.
func (s *mcpServer) callTool(req rpcRequest) rpcResponse {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.fail(req, -32602, "invalid params")
	}
	h, ok := toolHandlers[p.Name]
	if !ok {
		return s.fail(req, -32602, "unknown tool: "+p.Name)
	}
	args := map[string]any{}
	if len(p.Arguments) > 0 {
		if err := json.Unmarshal(p.Arguments, &args); err != nil {
			return s.fail(req, -32602, "invalid arguments")
		}
	}
	// project_path retargets this one call at another project's graph; it is
	// never a tool argument, so pop it before handing args to the handler.
	if project := argString(args, "project_path"); project != "" {
		c, err := s.contextFor(project)
		if err != nil {
			return s.textResult(req, "Error: "+err.Error())
		}
		defer func(prev graphCtx) { s.graphCtx = prev }(s.graphCtx)
		s.graphCtx = *c
	}
	delete(args, "project_path")
	return s.textResult(req, h(s, args))
}

// toolHandlers maps each MCP tool name to its handler.
var toolHandlers = map[string]func(*mcpServer, map[string]any) string{
	"query_graph":   (*mcpServer).toolQueryGraph,
	"get_node":      (*mcpServer).toolGetNode,
	"get_neighbors": (*mcpServer).toolGetNeighbors,
	"get_community": (*mcpServer).toolGetCommunity,
	"god_nodes":     (*mcpServer).toolGodNodes,
	"graph_stats":   (*mcpServer).toolGraphStats,
	"shortest_path": (*mcpServer).toolShortestPath,
}

func (s *mcpServer) toolQueryGraph(args map[string]any) string {
	question := argString(args, "question")
	if question == "" {
		return "Error: question is required."
	}
	dfs := argString(args, "mode") == "dfs"
	depth := argInt(args, "depth", 3)
	if depth > 6 {
		depth = 6
	}
	if depth < 1 {
		depth = 1
	}
	budget := argInt(args, "token_budget", 2000)
	if budget < 1 {
		budget = 2000
	}
	return query.Ask(s.g, question, dfs, depth, budget)
}

func (s *mcpServer) toolGetNode(args map[string]any) string {
	label := argString(args, "label")
	ex, err := query.Explain(s.g, label)
	if err != nil {
		return fmt.Sprintf("No node matching '%s' found.", label)
	}
	n := ex.Node
	comm := ""
	if n.Community != nil {
		comm = fmt.Sprintf("%d", *n.Community)
	}
	return strings.Join([]string{
		"Node: " + security.SanitizeLabel(labelOrID(n)),
		"  ID: " + security.SanitizeLabel(n.ID),
		"  Source: " + security.SanitizeLabel(n.SourceFile) + " " + security.SanitizeLabel(n.SourceLocation),
		"  Type: " + security.SanitizeLabel(n.FileType),
		"  Community: " + security.SanitizeLabel(comm),
		fmt.Sprintf("  Degree: %d", s.god.Degree(n.ID)),
	}, "\n")
}

func (s *mcpServer) toolGetNeighbors(args map[string]any) string {
	label := argString(args, "label")
	ex, err := query.Explain(s.g, label)
	if err != nil {
		return fmt.Sprintf("No node matching '%s' found.", label)
	}
	relFilter := strings.ToLower(argString(args, "relation_filter"))
	var items []string
	for _, nb := range ex.Neighbors {
		if relFilter != "" && !strings.Contains(strings.ToLower(nb.Relation), relFilter) {
			continue
		}
		arrow := "-->"
		if nb.Direction == "<-" {
			arrow = "<--"
		}
		items = append(items, fmt.Sprintf("  %s %s [%s]", arrow,
			security.SanitizeLabel(nb.Label), security.SanitizeLabel(nb.Relation)))
	}
	header := "Neighbors of " + security.SanitizeLabel(labelOrID(ex.Node)) + ":"
	return budgetLines(header, items, argTokenBudget(args))
}

func (s *mcpServer) toolGetCommunity(args map[string]any) string {
	cid := argInt(args, "community_id", -1)
	nodes := s.communities[cid]
	if len(nodes) == 0 {
		return fmt.Sprintf("Community %d not found.", cid)
	}
	sort.Strings(nodes)
	items := make([]string, 0, len(nodes))
	for _, id := range nodes {
		n := s.god.Nodes[id]
		label, src := id, ""
		if n != nil {
			label, src = n.Label, n.SourceFile
		}
		items = append(items, "  "+security.SanitizeLabel(label)+" ["+security.SanitizeLabel(src)+"]")
	}
	header := fmt.Sprintf("Community %d (%d nodes):", cid, len(nodes))
	return budgetLines(header, items, argTokenBudget(args))
}

// argTokenBudget reads token_budget, defaulting to 2000.
func argTokenBudget(args map[string]any) int {
	budget := argInt(args, "token_budget", 2000)
	if budget < 1 {
		budget = 2000
	}
	return budget
}

// budgetLines joins header + items, cutting items that do not fit in the
// ~3-chars-per-token budget and prepending a truncation notice so the caller
// sees it before reading the list.
func budgetLines(header string, items []string, tokenBudget int) string {
	charBudget := tokenBudget * 3
	used, shown := len(header), len(items)
	for i, l := range items {
		used += len(l) + 1
		if used > charBudget {
			shown = i
			break
		}
	}
	out := append([]string{header}, items[:shown]...)
	if shown < len(items) {
		out = append([]string{fmt.Sprintf(
			"Truncated: %d of %d shown (~%d-token budget; raise token_budget or narrow the query)",
			shown, len(items), tokenBudget)}, out...)
	}
	return strings.Join(out, "\n")
}

func (s *mcpServer) toolGodNodes(args map[string]any) string {
	nodes := analyze.GodNodes(s.god, argInt(args, "top_n", 10))
	lines := []string{"God nodes (most connected):"}
	for i, n := range nodes {
		lines = append(lines, fmt.Sprintf("  %d. %s - %d edges", i+1, security.SanitizeLabel(n.Label), n.Degree))
	}
	return strings.Join(lines, "\n")
}

func (s *mcpServer) toolGraphStats(map[string]any) string {
	total := len(s.g.Links)
	if total == 0 {
		total = 1
	}
	counts := map[string]int{}
	for i := range s.g.Links {
		c := s.g.Links[i].Confidence
		if c == "" {
			c = "EXTRACTED"
		}
		counts[c]++
	}
	pct := func(k string) int { return int(float64(counts[k])/float64(total)*100 + 0.5) }
	return fmt.Sprintf(
		"Nodes: %d\nEdges: %d\nCommunities: %d\nEXTRACTED: %d%%\nINFERRED: %d%%\nAMBIGUOUS: %d%%\n",
		len(s.g.Nodes), len(s.g.Links), len(s.communities),
		pct("EXTRACTED"), pct("INFERRED"), pct("AMBIGUOUS"))
}

func (s *mcpServer) toolShortestPath(args map[string]any) string {
	src, tgt := argString(args, "source"), argString(args, "target")
	undirected, _ := args["undirected"].(bool)
	res, err := query.PathEdges(s.g, src, tgt, argInt(args, "max_hops", 8), undirected)
	if err != nil {
		if errors.Is(err, query.ErrNoDirectedPath) {
			return err.Error() + "; retry with undirected=true"
		}
		var same *query.SameNodeError
		var over *query.MaxHopsError
		if errors.As(err, &same) || errors.As(err, &over) {
			return err.Error() // advisory, not a hard error
		}
		return "Error: " + err.Error()
	}
	return fmt.Sprintf("Shortest path (%d hops):\n  %s", len(res.Edges), renderPathChain(res))
}

// renderPathChain renders a resolved path as "a --calls [INFERRED]--> b", using
// "<--rel--" for hops whose stored edge points against the direction of travel
// so the arrow never asserts a direction the graph does not record.
func renderPathChain(res *query.PathResult) string {
	var b strings.Builder
	b.WriteString(security.SanitizeLabel(labelOrID(&res.Nodes[0])))
	for i, e := range res.Edges {
		next := security.SanitizeLabel(labelOrID(&res.Nodes[i+1]))
		conf := ""
		if e.Confidence != "" {
			conf = " [" + e.Confidence + "]"
		}
		if e.Forward {
			fmt.Fprintf(&b, " --%s%s--> %s", e.Relation, conf, next)
		} else {
			fmt.Fprintf(&b, " <--%s%s-- %s", e.Relation, conf, next)
		}
	}
	return b.String()
}

// labelOrID returns a node's label, falling back to its id.
func labelOrID(n *query.Node) string {
	if n.Label != "" {
		return n.Label
	}
	return n.ID
}

// argString extracts a string argument, returning "" when absent or not a string.
func argString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// argInt extracts an integer argument (JSON numbers decode to float64),
// returning def when absent or not a number.
func argInt(args map[string]any, key string, def int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	return def
}

// toolDefs returns the MCP tool definitions advertised by tools/list.
func toolDefs() []map[string]any {
	obj := func(props map[string]any, required ...string) map[string]any {
		// Every tool accepts project_path, so one server can answer about any
		// project on disk, not just the one it was launched in.
		props["project_path"] = map[string]any{"type": "string", "description": "Optional: path to another project containing graphify-out/graph.json (default: the project serve was launched with)"}
		schema := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	str := map[string]any{"type": "string"}
	return []map[string]any{
		{"name": "query_graph",
			"description": "Search the knowledge graph using BFS or DFS. Returns relevant nodes and edges as text context.",
			"inputSchema": obj(map[string]any{
				"question":     map[string]any{"type": "string", "description": "Natural language question or keyword search"},
				"mode":         map[string]any{"type": "string", "enum": []string{"bfs", "dfs"}, "description": "bfs=broad context, dfs=trace a specific path"},
				"depth":        map[string]any{"type": "integer", "description": "Traversal depth (1-6)"},
				"token_budget": map[string]any{"type": "integer", "description": "Max output tokens"},
			}, "question")},
		{"name": "get_node",
			"description": "Get full details for a specific node by label or ID.",
			"inputSchema": obj(map[string]any{"label": str}, "label")},
		{"name": "get_neighbors",
			"description": "Get all direct neighbors of a node with edge details.",
			"inputSchema": obj(map[string]any{
				"label":           str,
				"relation_filter": map[string]any{"type": "string", "description": "Optional: filter by relation type"},
				"token_budget":    map[string]any{"type": "integer", "description": "Max output tokens (default 2000)"},
			}, "label")},
		{"name": "get_community",
			"description": "Get all nodes in a community by community ID.",
			"inputSchema": obj(map[string]any{
				"community_id": map[string]any{"type": "integer", "description": "Community ID (0-indexed by size)"},
				"token_budget": map[string]any{"type": "integer", "description": "Max output tokens (default 2000)"},
			}, "community_id")},
		{"name": "god_nodes",
			"description": "Return the most connected nodes - the core abstractions of the knowledge graph.",
			"inputSchema": obj(map[string]any{"top_n": map[string]any{"type": "integer"}})},
		{"name": "graph_stats",
			"description": "Return summary statistics: node count, edge count, communities, confidence breakdown.",
			"inputSchema": obj(map[string]any{})},
		{"name": "shortest_path",
			"description": "Find the shortest path between two concepts in the knowledge graph, following edge direction. Each hop is annotated with its relation and confidence.",
			"inputSchema": obj(map[string]any{
				"source":     map[string]any{"type": "string", "description": "Source concept label or keyword"},
				"target":     map[string]any{"type": "string", "description": "Target concept label or keyword"},
				"max_hops":   map[string]any{"type": "integer", "description": "Reject paths longer than this many hops (default 8)"},
				"undirected": map[string]any{"type": "boolean", "description": "Ignore edge direction (default false: follow edges forwards only)"},
			}, "source", "target")},
	}
}
