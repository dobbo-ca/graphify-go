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

// sensitivePatterns match filenames that specifically name a secret store
// (extensions, exact credential-store names). These are specific and always
// apply, regardless of extension.
var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(^|[\\/])\.(env|envrc)(\.|$)`),
	regexp.MustCompile(`(?i)\.(pem|key|p12|pfx|cert|crt|der|p8)$`),
	regexp.MustCompile(`(id_rsa|id_dsa|id_ecdsa|id_ed25519)(\.pub)?$`),
	regexp.MustCompile(`(?i)(\.netrc|\.pgpass|\.htpasswd)$`),
}

// secretProneDataExts are data/serialization extensions that commonly ARE secret
// stores when their name hits a generic keyword (credentials.json, secrets.yaml).
// They stay subject to the generic-keyword drop even though .json routes through
// the code path for manifest parsing — only real programming-language source is
// exempt. Mirrors the upstream _SECRET_PRONE_DATA_EXTS set.
var secretProneDataExts = map[string]bool{
	".json": true, ".yaml": true, ".yml": true, ".toml": true, ".ini": true,
	".cfg": true, ".conf": true, ".config": true, ".xml": true,
	".properties": true, ".env": true, ".txt": true,
}

// genericKeywordPattern matches a generic secret keyword in a filename. Unlike
// sensitivePatterns it does NOT unconditionally drop the file: a genuine source
// file whose name merely contains the keyword (password_reset.go,
// passwords_controller.rb) is a module, not a secret store, so it is exempt in
// isSensitive Stage 3. RE2 has no lookbehind, so the word boundaries are emulated
// with (^|[^a-zA-Z0-9]) and ($|[^a-zA-Z]); group 1 captures the keyword plus its
// optional plural "s" so its end position can be tested against the stem end.
var genericKeywordPattern = regexp.MustCompile(`(?i)(?:^|[^a-zA-Z0-9])((?:credential|secret|passwd|password|private_key)s?)(?:$|[^a-zA-Z])`)

// wordSplit separates a filename stem into words for the load-bearing check
// (mirrors the upstream _WORD_SPLIT of [-_\s]+).
var wordSplit = regexp.MustCompile(`[-_\s]+`)

// genericKeywordHit reports whether a generic secret keyword appears load-bearing
// in the filename. Secret-store files name their contents, and in English
// compounds the content noun comes last: "api_token", "oauth_password". A keyword
// that neither ends the stem nor sits in a short (<=2 word) name is a topic word
// in a descriptive slug ("password-policy-discussion.md") and must not silently
// drop the file. Mirrors upstream _generic_keyword_hit.
func genericKeywordHit(name string) bool {
	// Stem = name up to the first dot, ignoring leading dots so dotfiles like
	// ".secret" keep their keyword.
	stem := strings.SplitN(strings.TrimLeft(name, "."), ".", 2)[0]
	matches := genericKeywordPattern.FindAllStringSubmatchIndex(stem, -1)
	if len(matches) == 0 {
		return false
	}
	for _, m := range matches {
		if m[3] == len(stem) { // keyword+s ends the stem -> names the contents
			return true
		}
	}
	// Short name like secret_store / password_reset (<=2 words): still load-bearing.
	words := 0
	for _, w := range wordSplit.Split(stem, -1) {
		if w != "" {
			words++
		}
	}
	return words <= 2
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

// IsSensitive reports whether rel names a file likely to hold secrets (a key,
// credential, or a file under a sensitive directory). Exposed so later stages
// (e.g. the semantic enrichment pass) can re-apply the same skip heuristic
// before any file content leaves the process.
func IsSensitive(rel string) bool { return isSensitive(rel) }

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
	// Stage 3: generic secret keywords, only when load-bearing in the name. Do NOT
	// let a bare keyword silently drop a genuine programming-language source file: a
	// .rb/.py named passwords_controller or secret_store is a module, not a secret
	// store. Data/config formats (.json, .yaml, ...) are deliberately NOT exempt,
	// because credentials.json / secrets.yaml are exactly the secret stores this
	// stage must catch. The specific Stage 2 patterns (.env, .pem, id_rsa) still
	// apply to everything regardless of extension.
	if genericKeywordHit(name) {
		ext := strings.ToLower(filepath.Ext(name))
		isSourceCode := SupportedExtensions[ext] && !secretProneDataExts[ext]
		return !isSourceCode
	}
	return false
}
