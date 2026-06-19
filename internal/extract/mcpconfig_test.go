package extract

import "testing"

func TestExtractMCPConfig(t *testing.T) {
	src := []byte(`{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem@1.2.3", "/data"],
      "env": {"FS_TOKEN": "sk-secret-value"}
    },
    "fetch": {
      "command": "uvx",
      "args": ["mcp-server-fetch"]
    }
  }
}`)
	res := FileFromBytes(".mcp.json", src)

	labels := map[string]bool{}
	id2label := map[string]string{}
	for _, n := range res.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	for _, want := range []string{
		".mcp.json", "filesystem", "fetch", "npx", "uvx",
		"@modelcontextprotocol/server-filesystem", // version stripped
		"mcp-server-fetch", "FS_TOKEN",
	} {
		if !labels[want] {
			t.Errorf("missing node %q", want)
		}
	}

	// Env VALUE must never appear as a node label.
	if labels["sk-secret-value"] {
		t.Error("env value leaked into a node label")
	}

	has := func(srcLabel, rel, tgtLabel string) bool {
		for _, e := range res.Edges {
			if e.Relation == rel && id2label[e.Source] == srcLabel && id2label[e.Target] == tgtLabel {
				return true
			}
		}
		return false
	}
	if !has(".mcp.json", "contains", "filesystem") {
		t.Error("expected .mcp.json --contains--> filesystem")
	}
	if !has("filesystem", "references", "npx") {
		t.Error("expected filesystem --references--> npx")
	}
	if !has("filesystem", "references", "@modelcontextprotocol/server-filesystem") {
		t.Error("expected filesystem --references--> package")
	}
	if !has("filesystem", "requires_env", "FS_TOKEN") {
		t.Error("expected filesystem --requires_env--> FS_TOKEN")
	}
	if !has("fetch", "references", "mcp-server-fetch") {
		t.Error("expected fetch --references--> mcp-server-fetch")
	}
}

// Two configs declaring the same command/package/env var share those global
// nodes (so an agent can ask "which configs use this package?"), while their
// server nodes stay stem-scoped and distinct.
func TestExtractMCPConfigGlobalNodesShared(t *testing.T) {
	cfg := []byte(`{"mcpServers": {"fs": {"command": "npx", "args": ["@scope/pkg"], "env": {"TOK": "x"}}}}`)
	a := FileFromBytes("a/.mcp.json", cfg)
	b := FileFromBytes("b/.mcp.json", cfg)

	idOf := func(res Result, label string) string {
		for _, n := range res.Nodes {
			if n.Label == label {
				return n.ID
			}
		}
		return ""
	}
	if idOf(a, "npx") != idOf(b, "npx") || idOf(a, "npx") == "" {
		t.Error("mcp_command node should be global (shared across configs)")
	}
	if idOf(a, "@scope/pkg") != idOf(b, "@scope/pkg") {
		t.Error("mcp_package node should be global")
	}
	if idOf(a, "TOK") != idOf(b, "TOK") {
		t.Error("env_var node should be global")
	}
	if idOf(a, "fs") == idOf(b, "fs") || idOf(a, "fs") == "" {
		t.Error("mcp_server node should be stem-scoped (distinct per config)")
	}
}

// The nested {"mcp": {"servers": {...}}} shape some tools emit is also accepted.
func TestExtractMCPConfigNestedShape(t *testing.T) {
	src := []byte(`{"mcp": {"servers": {"time": {"command": "uvx", "args": ["mcp-server-time", "--local-timezone=UTC"]}}}}`)
	res := FileFromBytes("mcp.json", src)
	labels := map[string]bool{}
	for _, n := range res.Nodes {
		labels[n.Label] = true
	}
	if !labels["time"] || !labels["uvx"] || !labels["mcp-server-time"] {
		t.Errorf("nested mcp.servers shape not extracted: %v", labels)
	}
	// The option arg must not be mistaken for a package.
	if labels["--local-timezone=UTC"] {
		t.Error("option arg leaked as a package node")
	}
}

func TestExtractMCPConfigEmptyResults(t *testing.T) {
	cases := map[string]string{
		"not json":           `{not valid json`,
		"no mcpServers":      `{"other": {}}`,
		"mcpServers not map": `{"mcpServers": []}`,
		"root not object":    `[1, 2, 3]`,
	}
	for name, body := range cases {
		res := FileFromBytes("mcp.json", []byte(body))
		if len(res.Nodes) != 0 || len(res.Edges) != 0 {
			t.Errorf("%s: expected empty result, got %d nodes / %d edges", name, len(res.Nodes), len(res.Edges))
		}
	}
}

func TestDetectPackageFromArgs(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-y", "@org/pkg@1.2.3"}, "@org/pkg"}, // scoped npm, version stripped
		{[]string{"some-mcp@2.0.0"}, ""},               // py id pattern has no @version branch: no match
		{[]string{"server-mcp"}, "server-mcp"},         // py "-mcp" suffix
		{[]string{"mcp-server-git"}, "mcp-server-git"}, // py "mcp-" prefix
		{[]string{"-y", "--yes", "/abs/path"}, ""},     // flags + bare path: no package
		{[]string{"@scope/name"}, "@scope/name"},       // scoped, no version
		{nil, ""},                                      // no args
	}
	for _, c := range cases {
		if got := detectPackageFromArgs(c.args); got != c.want {
			t.Errorf("detectPackageFromArgs(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestStripVersion(t *testing.T) {
	cases := map[string]string{
		"@scope/name":       "@scope/name",
		"@scope/name@1.2.3": "@scope/name",
		"name":              "name",
		"name@1.2.3":        "name",
	}
	for in, want := range cases {
		if got := stripVersion(in); got != want {
			t.Errorf("stripVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsMCPConfigPath(t *testing.T) {
	yes := []string{
		".mcp.json", "claude_desktop_config.json", "mcp.json", "mcp_servers.json",
		"sub/dir/.mcp.json", "/abs/path/mcp_servers.json",
	}
	for _, p := range yes {
		if !IsMCPConfigPath(p) {
			t.Errorf("IsMCPConfigPath(%q) = false, want true", p)
		}
	}
	no := []string{"config.json", "package.json", "mcp.yaml", "mymcp.json", "main.go"}
	for _, p := range no {
		if IsMCPConfigPath(p) {
			t.Errorf("IsMCPConfigPath(%q) = true, want false", p)
		}
	}
}
