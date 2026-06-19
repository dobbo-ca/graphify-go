package extract

import (
	"path/filepath"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsjson "github.com/tree-sitter/tree-sitter-json/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
	"github.com/dobbo-ca/graphify-go/internal/security"
)

// jsonMaxBytes skips large fixture dumps / GeoJSON blobs (mirrors upstream
// _JSON_MAX_BYTES). Data JSON that slips past the config gate is bounded anyway.
const jsonMaxBytes = 1 << 20 // 1 MiB

// jsonMaxPairs / jsonMaxDepth bound the walk so a deeply nested or huge config
// can't explode the graph (mirror upstream's pair_count cap and depth>6 guard).
const (
	jsonMaxPairs = 500
	jsonMaxDepth = 6
)

// configJSONNames are config/manifest basenames worth AST-extracting, matched
// case-insensitively (mirrors upstream _CONFIG_JSON_NAMES). MCP-config basenames
// (.mcp.json, mcp.json, …) are deliberately absent: FileFromBytes routes those
// to extractMCPConfig before this extractor is reached.
var configJSONNames = map[string]bool{
	"package.json": true, "tsconfig.json": true, "jsconfig.json": true,
	"composer.json": true, "deno.json": true, "deno.jsonc": true,
	"bower.json": true, "manifest.json": true, "app.json": true,
	"now.json": true, "vercel.json": true, "angular.json": true,
	"nest-cli.json": true, "biome.json": true, "biome.jsonc": true,
	"renovate.json": true, ".babelrc": true, ".babelrc.json": true,
	".eslintrc.json": true, ".prettierrc.json": true, ".prettierrc": true,
	"babel.config.json": true,
}

// configJSONKeys are top-level keys that prove an object is a config/manifest the
// extractor can draw cross-file edges from (mirrors upstream _CONFIG_JSON_KEYS).
var configJSONKeys = map[string]bool{
	"dependencies": true, "devDependencies": true, "peerDependencies": true,
	"optionalDependencies": true, "bundleDependencies": true, "bundledDependencies": true,
	"extends": true, "$ref": true, "$schema": true, "compilerOptions": true,
}

// depKeys are blocks whose string values become imports (package.json dep blocks).
var depKeys = map[string]bool{
	"dependencies": true, "devDependencies": true, "peerDependencies": true,
	"optionalDependencies": true, "bundleDependencies": true, "bundledDependencies": true,
}

// extractJSON turns a config/manifest .json file into config-key nodes plus the
// cross-file edges it can see (extends chains, $ref schema refs, dependency
// imports). It mirrors upstream extract_json: data-shaped JSON (fixtures,
// datasets, GeoJSON, API dumps) is deliberately skipped so it doesn't flood the
// graph with orphan key-nodes; recognition is by filename or a top-level key
// probe. A malformed, oversized, or non-config file yields an empty result.
func extractJSON(rel string, src []byte) Result {
	if len(src) > jsonMaxBytes {
		return Result{}
	}
	root, done := parseRoot(src, tsjson.Language())
	defer done()

	doc := root
	if doc.Kind() == "document" && doc.ChildCount() > 0 {
		doc = doc.Child(0)
	}
	if doc == nil || doc.Kind() != "object" {
		return Result{} // top-level array/scalar => data JSON, never a config
	}
	if !isConfigJSON(rel, doc, src) {
		return Result{}
	}

	fileID := idutil.MakeID(rel)
	res := Result{Nodes: []model.Node{{
		ID: fileID, Label: filepath.Base(rel), FileType: "code", SourceFile: rel, SourceLocation: "L1",
	}}}
	seenNode := map[string]bool{fileID: true}
	seenEdge := map[string]bool{}
	stem := fileStem(rel)

	addNode := func(id, label, loc string) {
		if id == "" || seenNode[id] {
			return
		}
		seenNode[id] = true
		res.Nodes = append(res.Nodes, model.Node{
			ID: id, Label: security.SanitizeLabel(label), FileType: "code", SourceFile: rel, SourceLocation: loc,
		})
	}
	addEdge := func(srcID, tgtID, relation, loc string) {
		if srcID == "" || tgtID == "" || srcID == tgtID {
			return
		}
		key := srcID + "\x00" + tgtID + "\x00" + relation
		if seenEdge[key] {
			return
		}
		seenEdge[key] = true
		res.Edges = append(res.Edges, model.Edge{
			Source: srcID, Target: tgtID, Relation: relation,
			Confidence: "EXTRACTED", SourceFile: rel, SourceLocation: loc,
		})
	}

	pairCount := 0
	var walkObject func(obj *ts.Node, parentID, parentKey string, depth int)
	walkObject = func(obj *ts.Node, parentID, parentKey string, depth int) {
		if depth > jsonMaxDepth {
			return
		}
		for i := uint(0); i < obj.ChildCount(); i++ {
			child := obj.Child(i)
			if child == nil || child.Kind() != "pair" {
				continue
			}
			if pairCount >= jsonMaxPairs {
				return
			}
			pairCount++
			key := jsonKeyText(child, src)
			if key == "" {
				continue
			}
			loc := line(child)
			keyParts := []string{stem}
			if parentKey != "" {
				keyParts = append(keyParts, parentKey)
			}
			keyParts = append(keyParts, key)
			keyID := idutil.MakeID(keyParts...)
			if keyID == "" {
				continue
			}
			addNode(keyID, key, loc)
			addEdge(parentID, keyID, "contains", loc)

			val := child.ChildByFieldName("value")
			if val == nil {
				continue
			}
			switch val.Kind() {
			case "object":
				walkObject(val, keyID, key, depth+1)
			case "array":
				// "extends" arrays (tsconfig, eslint): each string element is a ref.
				// Namespace with "ref" so external refs don't collide with real
				// code/file node IDs that share a collapsed _make_id.
				for j := uint(0); j < val.ChildCount(); j++ {
					item := val.Child(j)
					if item == nil || item.Kind() != "string" {
						continue
					}
					if ref := jsonStringContent(item, src); ref != "" {
						if refID := idutil.MakeID("ref", ref); refID != "" {
							addEdge(keyID, refID, "extends", loc)
						}
					}
				}
			case "string":
				valText := jsonStringContent(val, src)
				switch {
				case key == "extends" && valText != "":
					if refID := idutil.MakeID("ref", valText); refID != "" {
						addEdge(fileID, refID, "extends", loc)
					}
				case key == "$ref" && valText != "":
					if refID := idutil.MakeID("ref", valText); refID != "" {
						addEdge(parentID, refID, "references", loc)
					}
				case depKeys[parentKey] && valText != "":
					if depID := idutil.MakeID(key); depID != "" {
						addEdge(keyID, depID, "imports", loc)
					}
				}
			}
		}
	}
	walkObject(doc, fileID, "", 0)
	return res
}

// isConfigJSON reports whether a .json file is a recognized config/manifest worth
// AST-extracting. It matches by basename first (cheap), then probes the root
// object's immediate keys (mirrors upstream _is_config_json). Returns false for
// data JSON so it is skipped by the structural pass.
func isConfigJSON(rel string, obj *ts.Node, src []byte) bool {
	name := strings.ToLower(filepath.Base(rel))
	if configJSONNames[name] {
		return true
	}
	for _, suffix := range []string{".eslintrc.json", ".prettierrc.json", ".babelrc.json", "tsconfig.json", "jsconfig.json"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	for i := uint(0); i < obj.ChildCount(); i++ {
		child := obj.Child(i)
		if child == nil || child.Kind() != "pair" {
			continue
		}
		if configJSONKeys[jsonKeyText(child, src)] {
			return true
		}
	}
	return false
}

// jsonKeyText returns the string content of a pair's key, or "" if absent.
func jsonKeyText(pair *ts.Node, src []byte) string {
	key := pair.ChildByFieldName("key")
	if key == nil {
		return ""
	}
	if key.Kind() == "string" {
		return jsonStringContent(key, src)
	}
	return key.Utf8Text(src)
}

// jsonStringContent returns the unquoted content of a string node, preferring the
// grammar's string_content child and falling back to stripping surrounding quotes.
func jsonStringContent(s *ts.Node, src []byte) string {
	for i := uint(0); i < s.ChildCount(); i++ {
		c := s.Child(i)
		if c != nil && c.Kind() == "string_content" {
			return c.Utf8Text(src)
		}
	}
	return strings.Trim(s.Utf8Text(src), `"'`)
}
