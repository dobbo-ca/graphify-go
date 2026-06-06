package extract

import (
	"path"
	"path/filepath"

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

	// Index definitions by name (global) and by file+name (local-first calls).
	global := map[string][]string{}
	local := map[string]string{} // file\x00name -> id
	for _, r := range results {
		for _, d := range r.Defs {
			global[d.Name] = append(global[d.Name], d.ID)
			local[d.File+"\x00"+d.Name] = d.ID
		}
	}

	var out model.Extraction
	for _, r := range results {
		out.Nodes = append(out.Nodes, r.Nodes...)
		out.Edges = append(out.Edges, r.Edges...)
	}

	// Calls: prefer a definition in the same file, else a unique global one.
	for _, r := range results {
		for _, c := range r.Calls {
			tgt := local[c.File+"\x00"+c.Callee]
			if tgt == "" {
				if ids := global[c.Callee]; len(ids) == 1 {
					tgt = ids[0]
				}
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
	return out
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
