package extract

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
	"github.com/dobbo-ca/graphify-go/internal/security"
)

// mcpConfigFilenames are the MCP config basenames this extractor recognises,
// matched case-sensitively on the file's basename (mirrors the upstream
// mcp_ingest.MCP_CONFIG_FILENAMES set).
var mcpConfigFilenames = map[string]bool{
	".mcp.json":                  true,
	"claude_desktop_config.json": true,
	"mcp.json":                   true,
	"mcp_servers.json":           true,
}

const mcpMaxServers = 200 // generous cap; flags pathological configs

// IsMCPConfigPath reports whether name is a recognised MCP config filename.
// name may be a full path; only the basename is matched.
func IsMCPConfigPath(name string) bool {
	return mcpConfigFilenames[filepath.Base(filepath.ToSlash(name))]
}

// extractMCPConfig turns an MCP config file's mcpServers map into graph nodes
// and edges. It reads env-var NAMES only, never their values. A malformed file
// or one with no mcpServers map yields an empty result (indistinguishable from
// "no MCP config here" for the caller).
//
// Nodes: the file, one mcp_server per entry, plus globally-scoped mcp_command,
// mcp_package, and env_var nodes (shared across configs so an agent can ask
// "which configs depend on this package/env var?"). Edges: file -> contains ->
// server, server -> references -> command/package, server -> requires_env ->
// env_var.
func extractMCPConfig(rel string, src []byte) Result {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(src, &doc); err != nil {
		return Result{}
	}

	servers := mcpServersMap(doc)
	if servers == nil {
		return Result{}
	}

	fileID := idutil.MakeID(rel)
	res := Result{Nodes: []model.Node{{
		ID: fileID, Label: filepath.Base(rel), FileType: "code", SourceFile: rel, SourceLocation: "L1",
	}}}
	seenNode := map[string]bool{fileID: true}
	seenEdge := map[string]bool{}
	stem := fileStem(rel)

	addNode := func(id, label string) {
		if id == "" || seenNode[id] {
			return
		}
		seenNode[id] = true
		res.Nodes = append(res.Nodes, model.Node{
			ID: id, Label: security.SanitizeLabel(label), FileType: "code", SourceFile: rel, SourceLocation: "L1",
		})
	}
	addEdge := func(srcID, tgtID, relation string) {
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
			Confidence: "EXTRACTED", SourceFile: rel, SourceLocation: "L1",
		})
	}

	// Stable iteration: JSON object order is non-deterministic, so sort names.
	serverCount := 0
	for _, name := range sortedKeys(servers) {
		if serverCount >= mcpMaxServers {
			break
		}
		var spec mcpServerSpec
		if err := json.Unmarshal(servers[name], &spec); err != nil {
			continue // skip non-object / malformed server entries silently
		}
		serverCount++
		serverID := idutil.MakeID(stem, "mcp_server", name)
		addNode(serverID, name)
		addEdge(fileID, serverID, "contains")

		if cmd := strings.TrimSpace(spec.Command); cmd != "" {
			cmdID := idutil.MakeID("mcp_command", cmd)
			addNode(cmdID, cmd)
			addEdge(serverID, cmdID, "references")
		}
		if pkg := detectPackageFromArgs(spec.Args); pkg != "" {
			pkgID := idutil.MakeID("mcp_package", pkg)
			addNode(pkgID, pkg)
			addEdge(serverID, pkgID, "references")
		}
		// ONLY env var KEYS — values may hold secrets and are never read.
		for envName := range spec.Env {
			if envName == "" {
				continue
			}
			envID := idutil.MakeID("env_var", envName)
			addNode(envID, envName)
			addEdge(serverID, envID, "requires_env")
		}
	}
	return res
}

// mcpServerSpec is the subset of an mcpServers entry this extractor reads. Env
// values are deliberately discarded (map to json.RawMessage would still read
// them); only the keys of env are used.
type mcpServerSpec struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// mcpServersMap returns the server map from a parsed config, trying the standard
// top-level "mcpServers" key and the well-known nested {"mcp": {"servers": {}}}
// shape. Returns nil when neither is present.
func mcpServersMap(doc map[string]json.RawMessage) map[string]json.RawMessage {
	if raw, ok := doc["mcpServers"]; ok {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) == nil && m != nil {
			return m
		}
	}
	if raw, ok := doc["mcp"]; ok {
		var nested struct {
			Servers map[string]json.RawMessage `json:"servers"`
		}
		if json.Unmarshal(raw, &nested) == nil && nested.Servers != nil {
			return nested.Servers
		}
	}
	return nil
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// Package-id patterns observed in real MCP configs: npm scoped names
// (@org/pkg, optionally @version) passed to npx, and python "mcp-" / "-mcp"
// package ids passed to uvx. Mirrors upstream _NPM_PKG_RE / _PY_MCP_PKG_RE.
var (
	npmPkgRe   = regexp.MustCompile(`^@[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*(?:@[\w.\-+]+)?$`)
	pyMCPPkgRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*-mcp(?:-[a-z0-9._-]+)?$|^mcp-[a-z0-9][a-z0-9._-]*$`)
	argFlagRe  = regexp.MustCompile(`^-{1,2}\w`)
)

// detectPackageFromArgs returns the first arg that looks like an npm or pypi
// package id, else "". Short flags (-y) and option args (--tz=UTC) are skipped.
func detectPackageFromArgs(args []string) string {
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		if arg == "" || argFlagRe.MatchString(arg) {
			continue
		}
		if npmPkgRe.MatchString(arg) {
			return stripVersion(arg)
		}
		if pyMCPPkgRe.MatchString(arg) {
			return arg
		}
	}
	return ""
}

// stripVersion drops the @version suffix from an npm package id, preserving a
// leading @scope.
func stripVersion(pkg string) string {
	if strings.HasPrefix(pkg, "@") {
		if i := strings.IndexByte(pkg[1:], '@'); i != -1 {
			return pkg[:i+1]
		}
		return pkg
	}
	if i := strings.IndexByte(pkg, '@'); i != -1 {
		return pkg[:i]
	}
	return pkg
}
