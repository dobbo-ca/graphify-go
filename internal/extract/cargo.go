package extract

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// crate bundles a discovered package with the manifest it came from, so edge
// emission can re-read its [dependencies] table and attribute edges to the file.
type crate struct {
	id       string
	manifest string // path relative to root, posix-slashed
	data     map[string]any
}

// IntrospectCargo parses a Rust workspace rooted at root/Cargo.toml plus its
// member manifests and returns crate:<name> nodes and crate_depends_on edges for
// workspace-internal path dependencies only (registry deps like serde are
// excluded). It mirrors upstream graphify/cargo_introspect.py: a manifest with a
// [package].name becomes a node, and a [dependencies] entry whose name matches
// another workspace crate becomes an edge. A genuine TOML syntax error in any
// manifest is returned; degenerate-but-parseable manifests yield no nodes/edges
// rather than an error.
func IntrospectCargo(root string) (Result, error) {
	rootManifest := filepath.Join(root, "Cargo.toml")
	rootData, err := loadTOML(rootManifest)
	if err != nil {
		return Result{}, err
	}

	manifests, err := memberManifestPaths(root, rootData)
	if err != nil {
		return Result{}, err
	}

	crates := map[string]crate{}
	for _, manifest := range manifests {
		data := rootData
		if manifest != rootManifest {
			data, err = loadTOML(manifest)
			if err != nil {
				return Result{}, err
			}
		}
		name, ok := packageName(data)
		if !ok {
			continue
		}
		rel, err := filepath.Rel(root, manifest)
		if err != nil {
			return Result{}, err
		}
		crates[name] = crate{id: "crate:" + name, manifest: filepath.ToSlash(rel), data: data}
	}

	names := make([]string, 0, len(crates))
	for name := range crates {
		names = append(names, name)
	}
	sort.Strings(names)

	var res Result
	for _, name := range names {
		c := crates[name]
		res.Nodes = append(res.Nodes, model.Node{
			ID: c.id, Label: name, FileType: "code", SourceFile: c.manifest, SourceLocation: "L1",
		})
	}
	for _, name := range names {
		c := crates[name]
		deps, ok := c.data["dependencies"].(map[string]any)
		if !ok {
			continue
		}
		depNames := make([]string, 0, len(deps))
		for depName := range deps {
			depNames = append(depNames, depName)
		}
		sort.Strings(depNames)
		for _, depName := range depNames {
			// Honor a dep-table rename: `db = { path = "...", package = "real" }`
			// binds to the workspace crate named "real", not the dep key "db"
			// (#1858). A rename pointing at a registry/external crate simply misses
			// `crates` and stays a no-op, like any other non-internal dep.
			lookup := depName
			if spec, ok := deps[depName].(map[string]any); ok {
				if pkg, ok := spec["package"].(string); ok && pkg != "" {
					lookup = pkg
				}
			}
			target, ok := crates[lookup]
			if !ok {
				continue // registry dep or unknown crate — not an internal edge
			}
			res.Edges = append(res.Edges, model.Edge{
				Source: c.id, Target: target.id, Relation: "crate_depends_on",
				Confidence: "EXTRACTED", Weight: 1.0,
				SourceFile: c.manifest, SourceLocation: "L1",
			})
		}
	}
	return res, nil
}

// loadTOML decodes a manifest into a generic map (mirroring tomllib.load), so
// degenerate values like `dependencies = "not-a-table"` survive parsing and are
// type-checked later rather than rejected at decode time.
func loadTOML(path string) (map[string]any, error) {
	var data map[string]any
	if _, err := toml.DecodeFile(path, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// memberManifestPaths lists the manifests to inspect: the root manifest when the
// root itself is a package, plus every Cargo.toml under an expanded
// [workspace].members glob. Returned paths are filesystem paths (joined on root).
func memberManifestPaths(root string, rootData map[string]any) ([]string, error) {
	var paths []string
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}

	if _, ok := rootData["package"].(map[string]any); ok {
		add(filepath.Join(root, "Cargo.toml"))
	}

	workspace, ok := rootData["workspace"].(map[string]any)
	if !ok {
		return paths, nil
	}
	members, ok := workspace["members"].([]any)
	if !ok {
		return paths, nil
	}
	for _, raw := range members {
		pattern, ok := raw.(string)
		if !ok {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, member := range matches {
			manifest := filepath.Join(member, "Cargo.toml")
			if info, err := os.Stat(manifest); err == nil && info.Mode().IsRegular() {
				add(manifest)
			}
		}
	}
	return paths, nil
}

// packageName returns the [package].name string, or ok=false when the manifest
// has no package table or no string name (mirroring the upstream isinstance guards).
func packageName(data map[string]any) (string, bool) {
	pkg, ok := data["package"].(map[string]any)
	if !ok {
		return "", false
	}
	name, ok := pkg["name"].(string)
	if !ok {
		return "", false
	}
	return name, true
}
