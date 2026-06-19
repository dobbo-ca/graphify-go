package extract

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
)

// helpers shared by the JSON extractor tests.
func jsonNodes(res Result) (labels map[string]bool, id2label map[string]string) {
	labels = map[string]bool{}
	id2label = map[string]string{}
	for _, n := range res.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	return
}

// jsonHasEdge matches by source label and target ID. Ref/dep targets (extends,
// $ref, imports) are edge-only — the extractor emits no node for them (mirrors
// upstream), so the target is matched by its computed ID, not a node label.
func jsonHasEdge(res Result, id2label map[string]string, srcLabel, rel, tgtID string) bool {
	for _, e := range res.Edges {
		if e.Relation == rel && id2label[e.Source] == srcLabel && e.Target == tgtID {
			return true
		}
	}
	return false
}

func TestExtractJSONPackageManifest(t *testing.T) {
	src := []byte(`{
  "name": "demo",
  "dependencies": {
    "left-pad": "^1.0.0"
  },
  "devDependencies": {
    "jest": "^29.0.0"
  }
}`)
	res := FileFromBytes("package.json", src)

	labels, id2label := jsonNodes(res)
	for _, want := range []string{"package.json", "name", "dependencies", "left-pad", "devDependencies", "jest"} {
		if !labels[want] {
			t.Errorf("missing node %q", want)
		}
	}
	if !jsonHasEdge(res, id2label, "package.json", "contains", idutil.MakeID("package", "dependencies")) {
		t.Error("expected package.json --contains--> dependencies")
	}
	if !jsonHasEdge(res, id2label, "dependencies", "contains", idutil.MakeID("package", "dependencies", "left-pad")) {
		t.Error("expected dependencies --contains--> left-pad")
	}
	if !jsonHasEdge(res, id2label, "left-pad", "imports", idutil.MakeID("left-pad")) {
		t.Error("expected dep key --imports--> dep node")
	}
}

func TestExtractJSONTsconfigExtendsAndRef(t *testing.T) {
	src := []byte(`{
  "extends": "./base.json",
  "compilerOptions": {
    "strict": true
  }
}`)
	res := FileFromBytes("tsconfig.json", src)

	_, id2label := jsonNodes(res)
	if !jsonHasEdge(res, id2label, "tsconfig.json", "extends", idutil.MakeID("ref", "./base.json")) {
		t.Error("expected tsconfig.json --extends--> ./base.json ref")
	}
	if !jsonHasEdge(res, id2label, "tsconfig.json", "contains", idutil.MakeID("tsconfig", "compilerOptions")) {
		t.Error("expected tsconfig.json --contains--> compilerOptions")
	}
}

func TestExtractJSONExtendsArray(t *testing.T) {
	src := []byte(`{
  "extends": ["eslint:recommended", "plugin:react/recommended"]
}`)
	res := FileFromBytes(".eslintrc.json", src)

	_, id2label := jsonNodes(res)
	if !jsonHasEdge(res, id2label, "extends", "extends", idutil.MakeID("ref", "eslint:recommended")) {
		t.Error("expected extends key --extends--> eslint:recommended ref")
	}
	if !jsonHasEdge(res, id2label, "extends", "extends", idutil.MakeID("ref", "plugin:react/recommended")) {
		t.Error("expected extends key --extends--> plugin:react/recommended ref")
	}
}

func TestExtractJSONSchemaRef(t *testing.T) {
	// Recognized via the top-level $schema key probe (arbitrary basename).
	src := []byte(`{
  "$schema": "https://example.com/schema.json",
  "thing": {
    "$ref": "#/defs/widget"
  }
}`)
	res := FileFromBytes("config/whatever.json", src)

	_, id2label := jsonNodes(res)
	if !jsonHasEdge(res, id2label, "thing", "references", idutil.MakeID("ref", "#/defs/widget")) {
		t.Error("expected $ref parent --references--> ref node")
	}
}

func TestExtractJSONSkipsDataJSON(t *testing.T) {
	// No recognized basename, no config key probe hit => data JSON, skipped.
	src := []byte(`{"users": [{"id": 1, "name": "a"}, {"id": 2, "name": "b"}]}`)
	res := FileFromBytes("fixtures/users.json", src)
	if len(res.Nodes) != 0 || len(res.Edges) != 0 {
		t.Errorf("data json should be skipped, got %d nodes / %d edges", len(res.Nodes), len(res.Edges))
	}
}

func TestExtractJSONNonObjectRootSkipped(t *testing.T) {
	res := FileFromBytes("data.json", []byte(`[1, 2, 3]`))
	if len(res.Nodes) != 0 || len(res.Edges) != 0 {
		t.Errorf("top-level array should be skipped, got %d nodes", len(res.Nodes))
	}
}

func TestExtractJSONMalformedSkipped(t *testing.T) {
	res := FileFromBytes("package.json", []byte(`{not valid json`))
	// tree-sitter is error-tolerant; a malformed config still must not panic and
	// must not emit a non-file node from garbage. At minimum it stays bounded.
	if len(res.Nodes) > 1 && res.Nodes[0].Label != "package.json" {
		t.Errorf("unexpected nodes from malformed json: %d", len(res.Nodes))
	}
}

func TestExtractJSONMCPConfigStillRoutedToMCPExtractor(t *testing.T) {
	// .mcp.json must reach extractMCPConfig (server nodes), NOT extractJSON
	// (config-key nodes). Guards against the general extractor stealing it.
	src := []byte(`{"mcpServers": {"fs": {"command": "npx", "args": ["-y", "@scope/server-fs"]}}}`)
	res := FileFromBytes(".mcp.json", src)
	labels, _ := jsonNodes(res)
	if !labels["fs"] {
		t.Error("expected mcp_server node 'fs' from extractMCPConfig")
	}
	if labels["mcpServers"] {
		t.Error(".mcp.json was AST-walked by extractJSON instead of routed to extractMCPConfig")
	}
}
