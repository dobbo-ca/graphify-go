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
	".go":     true,
	".js":     true,
	".jsx":    true,
	".mjs":    true,
	".cjs":    true,
	".ts":     true,
	".tsx":    true,
	".tf":     true,
	".tfvars": true,
	".hcl":    true,
	".py":     true,
	".rs":     true,
}

var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, ".hg": true, ".svn": true,
	"dist": true, "build": true, "target": true, "out": true,
	"vendor": true, "coverage": true, "__snapshots__": true,
	".next": true, ".nuxt": true, ".turbo": true, ".svelte-kit": true,
	".cache": true, ".idea": true, ".vscode": true, ".worktrees": true,
	"graphify-out": true, ".graphify": true,
}

var skipFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"go.sum": true, "go.work.sum": true,
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
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry — skip, don't abort the whole walk
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if skipFiles[name] || !SupportedExtensions[strings.ToLower(filepath.Ext(name))] {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if isSensitive(rel) {
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
