// Package detect walks a project tree and returns the source files worth
// graphing. It skips dependency/build/cache directories, generated lock files,
// and anything that looks like it holds secrets (mirrors the Python original's
// detect.py skip lists and sensitive-file heuristics).
package detect

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

// SupportedExtensions are the file suffixes this port can extract.
var SupportedExtensions = map[string]bool{
	".go":       true,
	".js":       true,
	".jsx":      true,
	".mjs":      true,
	".cjs":      true,
	".ts":       true,
	".tsx":      true,
	".tf":       true,
	".tfvars":   true,
	".json":     true,
	".hcl":      true,
	".py":       true,
	".rs":       true,
	".c":        true,
	".h":        true,
	".cpp":      true,
	".cc":       true,
	".cxx":      true,
	".hpp":      true,
	".hh":       true,
	".hxx":      true,
	".java":     true,
	".cs":       true,
	".rb":       true,
	".php":      true,
	".phtml":    true,
	".sh":       true,
	".bash":     true,
	".scala":    true,
	".sc":       true,
	".jl":       true,
	".v":        true,
	".sv":       true,
	".svh":      true,
	".vh":       true,
	".kt":       true,
	".kts":      true,
	".lua":      true,
	".zig":      true,
	".md":       true,
	".mdx":      true,
	".markdown": true,
}

var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, ".hg": true, ".svn": true,
	"dist": true, "build": true, "target": true, "out": true,
	"vendor": true, "coverage": true, "__snapshots__": true,
	".next": true, ".nuxt": true, ".turbo": true, ".svelte-kit": true,
	".cache": true, ".idea": true, ".vscode": true, ".worktrees": true,
	"graphify-out": true, ".graphify": true,
	".terraform": true, // provider/module cache from `terraform init` — vendored, never source
}

var skipFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"go.sum": true, "go.work.sum": true,
}

// mcpConfigFiles are indexed by basename: their .json extension is not in
// SupportedExtensions, but they wire up MCP servers an agent can query. The
// extractor for them lives in internal/extract (mcpconfig.go).
var mcpConfigFiles = map[string]bool{
	".mcp.json":                  true,
	"claude_desktop_config.json": true,
	"mcp.json":                   true,
	"mcp_servers.json":           true,
}

// sensitiveDirs hold secrets; any file under one is skipped.
var sensitiveDirs = map[string]bool{
	".ssh": true, ".gnupg": true, ".aws": true, ".gcloud": true,
	"secrets": true, ".secrets": true, "credentials": true,
}

// sensitivePatterns match filenames likely to contain secrets.
var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(^|[\\/])\.(env|envrc)(\.|$)`),
	regexp.MustCompile(`(?i)\.(pem|key|p12|pfx|cert|crt|der|p8)$`),
	regexp.MustCompile(`(?i)(^|[^a-zA-Z0-9])(credential|secret|passwd|password|private_key)s?($|[^a-zA-Z])`),
	regexp.MustCompile(`(id_rsa|id_dsa|id_ecdsa|id_ed25519)(\.pub)?$`),
	regexp.MustCompile(`(?i)(\.netrc|\.pgpass|\.htpasswd)$`),
}

// CollectFiles returns the supported source files under root, relative to root,
// in sorted order for deterministic output.
func CollectFiles(root string) ([]string, error) {
	var files []string
	ign := newIgnorer(root)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry — skip, don't abort the whole walk
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		slashRel := filepath.ToSlash(rel)
		if d.IsDir() {
			if path == root {
				return nil
			}
			if skipDirs[d.Name()] || ign.ignored(slashRel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if skipFiles[name] {
			return nil
		}
		if !SupportedExtensions[strings.ToLower(filepath.Ext(name))] && !mcpConfigFiles[name] {
			return nil
		}
		if isSensitive(rel) || ign.ignored(slashRel, false) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

func isSensitive(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, p := range parts[:len(parts)-1] { // parents only — a root file named "credentials" is handled by name patterns
		if sensitiveDirs[p] {
			return true
		}
	}
	name := parts[len(parts)-1]
	for _, p := range sensitivePatterns {
		if p.MatchString(name) {
			return true
		}
	}
	return false
}
