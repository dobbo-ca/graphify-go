package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/query"
)

// serveGraph wires three connected functions across two communities plus an
// isolated noise node, so traversal, community grouping, and god-node ranking
// all have something to chew on.
const serveGraph = `{
  "directed": true, "multigraph": false, "graph": {},
  "nodes": [
    {"id":"auth_validate","label":"authValidate()","file_type":"code","source_file":"auth.go","source_location":"L10","community":0,"norm_label":"authvalidate()"},
    {"id":"auth_check","label":"checkToken()","file_type":"code","source_file":"auth.go","source_location":"L20","community":0,"norm_label":"checktoken()"},
    {"id":"util_log","label":"log()","file_type":"code","source_file":"util.go","source_location":"L1","community":1,"norm_label":"log()"}
  ],
  "links": [
    {"source":"auth_validate","target":"auth_check","relation":"calls","confidence":"INFERRED"},
    {"source":"auth_check","target":"util_log","relation":"calls","confidence":"EXTRACTED"}
  ]
}`

func newServer(t *testing.T) *mcpServer {
	t.Helper()
	p := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(p, []byte(serveGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := query.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return &mcpServer{g: g, communities: communitiesOf(g), god: modelOf(g)}
}

func TestToolQueryGraph(t *testing.T) {
	s := newServer(t)
	out := s.toolQueryGraph(map[string]any{"question": "authValidate"})
	if !strings.Contains(out, "Traversal: BFS depth=3") {
		t.Errorf("missing BFS header: %q", out)
	}
	if !strings.Contains(out, "NODE authValidate()") {
		t.Errorf("seed node missing:\n%s", out)
	}
}

func TestToolQueryGraphDFSAndDepthCap(t *testing.T) {
	s := newServer(t)
	// depth above 6 is clamped to 6; mode dfs flips the traversal.
	out := s.toolQueryGraph(map[string]any{"question": "authValidate", "mode": "dfs", "depth": float64(99)})
	if !strings.Contains(out, "Traversal: DFS depth=6") {
		t.Errorf("expected DFS depth clamped to 6:\n%s", out)
	}
}

func TestToolQueryGraphMissingQuestion(t *testing.T) {
	s := newServer(t)
	if out := s.toolQueryGraph(map[string]any{}); !strings.Contains(out, "question is required") {
		t.Errorf("got %q, want required-question error", out)
	}
}

func TestToolGetNode(t *testing.T) {
	s := newServer(t)
	out := s.toolGetNode(map[string]any{"label": "authValidate"})
	for _, want := range []string{"Node: authValidate()", "ID: auth_validate", "auth.go", "Community: 0", "Degree: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("get_node output missing %q:\n%s", want, out)
		}
	}
}

func TestToolGetNodeNotFound(t *testing.T) {
	s := newServer(t)
	if out := s.toolGetNode(map[string]any{"label": "nope"}); !strings.Contains(out, "No node matching") {
		t.Errorf("got %q, want not-found message", out)
	}
}

func TestToolGetNeighbors(t *testing.T) {
	s := newServer(t)
	out := s.toolGetNeighbors(map[string]any{"label": "checkToken"})
	// checkToken is called by authValidate (incoming) and calls log (outgoing).
	if !strings.Contains(out, "<-- authValidate() [calls]") {
		t.Errorf("missing incoming neighbor:\n%s", out)
	}
	if !strings.Contains(out, "--> log() [calls]") {
		t.Errorf("missing outgoing neighbor:\n%s", out)
	}
}

func TestToolGetNeighborsRelationFilter(t *testing.T) {
	s := newServer(t)
	out := s.toolGetNeighbors(map[string]any{"label": "checkToken", "relation_filter": "imports"})
	// No neighbor has an "imports" relation, so only the header line remains.
	if strings.Contains(out, "-->") || strings.Contains(out, "<--") {
		t.Errorf("relation_filter should have excluded all neighbors:\n%s", out)
	}
}

func TestToolGetCommunity(t *testing.T) {
	s := newServer(t)
	out := s.toolGetCommunity(map[string]any{"community_id": float64(0)})
	if !strings.Contains(out, "Community 0 (2 nodes):") {
		t.Errorf("wrong community header:\n%s", out)
	}
	if !strings.Contains(out, "authValidate() [auth.go]") {
		t.Errorf("missing community member:\n%s", out)
	}
}

func TestToolGetCommunityNotFound(t *testing.T) {
	s := newServer(t)
	if out := s.toolGetCommunity(map[string]any{"community_id": float64(99)}); !strings.Contains(out, "not found") {
		t.Errorf("got %q, want not-found message", out)
	}
}

func TestToolGodNodes(t *testing.T) {
	s := newServer(t)
	out := s.toolGodNodes(map[string]any{"top_n": float64(5)})
	if !strings.HasPrefix(out, "God nodes (most connected):") {
		t.Errorf("wrong header:\n%s", out)
	}
	// checkToken has degree 2 (the highest) and must rank first.
	if !strings.Contains(out, "1. checkToken() - 2 edges") {
		t.Errorf("expected checkToken ranked first:\n%s", out)
	}
}

func TestToolGraphStats(t *testing.T) {
	s := newServer(t)
	out := s.toolGraphStats(nil)
	for _, want := range []string{"Nodes: 3", "Edges: 2", "Communities: 2", "INFERRED: 50%", "EXTRACTED: 50%"} {
		if !strings.Contains(out, want) {
			t.Errorf("graph_stats missing %q:\n%s", want, out)
		}
	}
}

func TestToolShortestPath(t *testing.T) {
	s := newServer(t)
	out := s.toolShortestPath(map[string]any{"source": "authValidate", "target": "log"})
	if !strings.Contains(out, "Shortest path (2 hops):") {
		t.Errorf("wrong hop count:\n%s", out)
	}
	if !strings.Contains(out, "authValidate() -> checkToken() -> log()") {
		t.Errorf("wrong path rendering:\n%s", out)
	}
}

func TestToolShortestPathNoNode(t *testing.T) {
	s := newServer(t)
	if out := s.toolShortestPath(map[string]any{"source": "nope", "target": "log"}); !strings.Contains(out, "Error:") {
		t.Errorf("got %q, want error for unknown source", out)
	}
}

// drive runs the server over a sequence of newline-delimited request lines and
// returns the decoded responses (notifications produce no response line).
func drive(t *testing.T, s *mcpServer, lines ...string) []rpcResponse {
	t.Helper()
	var out bytes.Buffer
	if err := s.run(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	var resps []rpcResponse
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var r rpcResponse
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("decode response %q: %v", l, err)
		}
		resps = append(resps, r)
	}
	return resps
}

func TestServeProtocolHandshake(t *testing.T) {
	s := newServer(t)
	resps := drive(t,
		s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`, // notification: no reply
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		``, // blank line is skipped
	)
	if len(resps) != 2 {
		t.Fatalf("got %d responses, want 2 (notification must be silent): %+v", len(resps), resps)
	}
	init, ok := resps[0].Result.(map[string]any)
	if !ok || init["protocolVersion"] != mcpProtocolVersion {
		t.Errorf("initialize result wrong: %+v", resps[0].Result)
	}
	list, ok := resps[1].Result.(map[string]any)
	if !ok {
		t.Fatalf("tools/list result not an object: %+v", resps[1].Result)
	}
	tools, ok := list["tools"].([]any)
	if !ok || len(tools) != 7 {
		t.Errorf("expected 7 tools advertised, got %+v", list["tools"])
	}
}

func TestServeToolsCall(t *testing.T) {
	s := newServer(t)
	resps := drive(t, s,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"graph_stats","arguments":{}}}`)
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	res := resps[0].Result.(map[string]any)
	content := res["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "Nodes: 3") {
		t.Errorf("tools/call did not return graph_stats text:\n%s", text)
	}
}

func TestServeErrors(t *testing.T) {
	s := newServer(t)
	resps := drive(t, s,
		`not json`,
		`{"jsonrpc":"2.0","id":4,"method":"no/such/method"}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"bogus","arguments":{}}}`,
	)
	if len(resps) != 3 {
		t.Fatalf("want 3 error responses, got %d: %+v", len(resps), resps)
	}
	if resps[0].Error == nil || resps[0].Error.Code != -32700 {
		t.Errorf("parse error not reported: %+v", resps[0].Error)
	}
	if resps[1].Error == nil || resps[1].Error.Code != -32601 {
		t.Errorf("method-not-found not reported: %+v", resps[1].Error)
	}
	if resps[2].Error == nil || !strings.Contains(resps[2].Error.Message, "unknown tool") {
		t.Errorf("unknown-tool not reported: %+v", resps[2].Error)
	}
}

func TestServeToolCallBadArguments(t *testing.T) {
	s := newServer(t)
	resps := drive(t, s,
		// params is not an object → invalid params.
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":[1,2]}`,
		// arguments is a string, not an object → invalid arguments.
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"graph_stats","arguments":"oops"}}`,
	)
	if len(resps) != 2 {
		t.Fatalf("want 2 responses, got %d", len(resps))
	}
	for i, r := range resps {
		if r.Error == nil || r.Error.Code != -32602 {
			t.Errorf("response %d: want -32602 invalid-params, got %+v", i, r.Error)
		}
	}
}

func TestCmdServeLoadError(t *testing.T) {
	// A missing graph.json must surface a load error, not panic.
	dir := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := cmdServe(defaultGraphPath); err == nil {
		t.Error("cmdServe with no graph.json = nil, want load error")
	}
}
