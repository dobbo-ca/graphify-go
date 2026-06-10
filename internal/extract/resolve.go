package extract

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// jsResolveExts are tried in order when resolving a relative JS/TS import to a
// file in the corpus.
var jsResolveExts = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".d.ts"}

// Resolve stitches per-file results into one extraction. It adds the call edges
// and import edges that need a whole-corpus view: calls resolve to definitions
// by name, and relative imports resolve to the file they point at.
func Resolve(results []Result, files []string) model.Extraction {
	corpus := make(map[string]bool, len(files))
	for _, f := range files {
		corpus[filepath.ToSlash(f)] = true
	}

	// Index definitions by name (global) and by file+name (local-first calls),
	// and remember each definition's file for disambiguation.
	global := map[string][]string{}
	local := map[string]string{}  // file\x00name -> id
	idFile := map[string]string{} // def id -> defining file
	for _, r := range results {
		for _, d := range r.Defs {
			global[d.Name] = append(global[d.Name], d.ID)
			local[d.File+"\x00"+d.Name] = d.ID
			idFile[d.ID] = d.File
		}
	}

	// For each file, the corpus files it imports — used to pick the right target
	// when a called name is defined in more than one file.
	importedFiles := map[string]map[string]bool{}
	for _, r := range results {
		for _, im := range r.Imps {
			target := resolveRelImport(im.File, im.Spec, corpus)
			if target == "" {
				continue
			}
			if importedFiles[im.File] == nil {
				importedFiles[im.File] = map[string]bool{}
			}
			importedFiles[im.File][target] = true
		}
	}

	var out model.Extraction
	for _, r := range results {
		out.Nodes = append(out.Nodes, r.Nodes...)
		out.Edges = append(out.Edges, r.Edges...)
	}

	// Calls: prefer a definition in the same file, else disambiguate among the
	// definitions sharing the called name (unique global, imported file, or same
	// package) rather than guessing.
	for _, r := range results {
		for _, c := range r.Calls {
			tgt := local[c.File+"\x00"+c.Callee]
			if tgt == "" {
				tgt = disambiguate(global[c.Callee], c.File, idFile, importedFiles[c.File])
			}
			if tgt == "" || tgt == c.CallerID {
				continue
			}
			out.Edges = append(out.Edges, model.Edge{
				Source: c.CallerID, Target: tgt, Relation: "calls",
				Confidence: "INFERRED", SourceFile: c.File, SourceLocation: c.Loc,
			})
		}
	}

	// Imports: relative specifiers resolve to a corpus file (imports_from, used
	// for cycle detection); bare specifiers become external dependency nodes.
	extSeen := map[string]bool{}
	for _, r := range results {
		for _, im := range r.Imps {
			if target := resolveRelImport(im.File, im.Spec, corpus); target != "" {
				out.Edges = append(out.Edges, model.Edge{
					Source: im.FileID, Target: idutil.MakeID(target), Relation: "imports_from",
					Confidence: "EXTRACTED", SourceFile: im.File, SourceLocation: im.Loc,
				})
				continue
			}
			depID := idutil.MakeID(im.Spec)
			if !extSeen[depID] {
				extSeen[depID] = true
				out.Nodes = append(out.Nodes, model.Node{ID: depID, Label: im.Spec, FileType: "concept"})
			}
			out.Edges = append(out.Edges, model.Edge{
				Source: im.FileID, Target: depID, Relation: "imports",
				Confidence: "EXTRACTED", SourceFile: im.File, SourceLocation: im.Loc,
			})
		}
	}

	// Terraform module sources: a local source resolves to the target directory
	// node (created once, with contains edges to that directory's files so the
	// module call is navigable into its implementation); a registry or
	// private-registry source becomes an external concept node.
	dirFiles := map[string][]string{}
	for _, f := range files {
		sf := filepath.ToSlash(f)
		dirFiles[path.Dir(sf)] = append(dirFiles[path.Dir(sf)], sf)
	}
	modSeen := map[string]bool{}
	for _, r := range results {
		for _, m := range r.ModRefs {
			target, isLocal := m.Source, isLocalSource(m.Source)
			if isLocal {
				target = path.Clean(path.Join(path.Dir(filepath.ToSlash(m.File)), m.Source))
				if filesIn, ok := dirFiles[target]; ok {
					dirID := idutil.MakeID("tfmodule", target)
					if !modSeen[dirID] {
						modSeen[dirID] = true
						out.Nodes = append(out.Nodes, model.Node{ID: dirID, Label: target, FileType: "code", SourceFile: target, SourceLocation: "L1"})
						for _, ff := range filesIn {
							out.Edges = append(out.Edges, model.Edge{
								Source: dirID, Target: idutil.MakeID(ff), Relation: "contains",
								Confidence: "EXTRACTED", SourceFile: target,
							})
						}
					}
					out.Edges = append(out.Edges, model.Edge{
						Source: m.FromID, Target: dirID, Relation: "references",
						Confidence: "EXTRACTED", SourceFile: m.File, SourceLocation: m.Loc,
					})
					continue
				}
				// local source pointing outside the corpus — keep it visible as a
				// concept node keyed by the cleaned target path.
			}
			extID := idutil.MakeID("tfmodule", target)
			if !modSeen[extID] {
				modSeen[extID] = true
				out.Nodes = append(out.Nodes, model.Node{ID: extID, Label: target, FileType: "concept"})
			}
			out.Edges = append(out.Edges, model.Edge{
				Source: m.FromID, Target: extID, Relation: "references",
				Confidence: "EXTRACTED", SourceFile: m.File, SourceLocation: m.Loc,
			})
		}
	}

	// Stage C: complete partial cloudposse null-label ids across local wrapper
	// chains, using the module-source edges and invocation args captured above.
	resolveNullLabels(results, &out)
	return out
}

// isLocalSource reports whether a Terraform module source is a local filesystem
// path (resolvable within the corpus) rather than a registry/git/private source.
func isLocalSource(s string) bool {
	return s == "." || s == ".." || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.HasPrefix(s, "/")
}

// disambiguate picks the call target among definitions sharing the called
// name. One candidate wins outright. When several share the name it prefers a
// unique definition in a file the caller imports, then a unique definition in
// the caller's own directory (same package); otherwise it returns "" rather
// than guess, leaving the call unresolved.
func disambiguate(ids []string, callerFile string, idFile map[string]string, imported map[string]bool) string {
	switch len(ids) {
	case 0:
		return ""
	case 1:
		return ids[0]
	}
	if id := unique(ids, func(id string) bool { return imported[idFile[id]] }); id != "" {
		return id
	}
	dir := path.Dir(filepath.ToSlash(callerFile))
	return unique(ids, func(id string) bool { return path.Dir(idFile[id]) == dir })
}

// unique returns the only id matching pred, or "" if zero or more than one do.
func unique(ids []string, pred func(string) bool) string {
	found := ""
	for _, id := range ids {
		if pred(id) {
			if found != "" {
				return ""
			}
			found = id
		}
	}
	return found
}

// resolveRelImport maps a relative import specifier to a file in the corpus,
// trying common extensions and index files. Returns "" for bare (external)
// specifiers or unresolved paths.
func resolveRelImport(fromFile, spec string, corpus map[string]bool) string {
	if spec == "" || (spec[0] != '.' && spec[0] != '/') {
		return "" // bare specifier — an external package
	}
	base := path.Clean(path.Join(path.Dir(filepath.ToSlash(fromFile)), spec))
	if corpus[base] {
		return base
	}
	for _, ext := range jsResolveExts {
		if corpus[base+ext] {
			return base + ext
		}
	}
	for _, ext := range jsResolveExts {
		if idx := base + "/index" + ext; corpus[idx] {
			return idx
		}
	}
	return ""
}
