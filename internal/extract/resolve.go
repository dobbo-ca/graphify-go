package extract

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/langfamily"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// jsResolveExts are tried in order when resolving a relative JS/TS import to a
// file in the corpus.
var jsResolveExts = []string{".ts", ".mts", ".cts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".d.ts"}

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
	local := map[string][]string{} // file\x00name -> ids
	idFile := map[string]string{}  // def id -> defining file
	for _, r := range results {
		for _, d := range r.Defs {
			global[d.Name] = append(global[d.Name], d.ID)
			key := d.File + "\x00" + d.Name
			if !contains(local[key], d.ID) {
				local[key] = append(local[key], d.ID)
			}
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

	// Python import-guided calls: explicit `from M import N [as L]` evidence
	// resolves a call to the unique (module_stem, symbol) definition with
	// EXTRACTED confidence. Runs before the generic name pass and marks each
	// resolved call site so the weaker name pass leaves it alone.
	resolved := resolveImportGuided(results, idFile, &out)

	// Calls: prefer a definition in the same file, else disambiguate among the
	// definitions sharing the called name (unique global, imported file, or same
	// package) rather than guessing.
	for _, r := range results {
		for _, c := range r.Calls {
			if resolved[c.CallerID+"\x00"+c.Callee+"\x00"+c.Loc] {
				continue
			}
			tgt := ""
			// Two types in one file can each own a method of the same name; a
			// bare call then has no unambiguous local target, so fall through
			// to disambiguate rather than guess.
			if ids := local[c.File+"\x00"+c.Callee]; len(ids) == 1 {
				tgt = ids[0]
			}
			if tgt == "" {
				tgt = disambiguate(global[c.Callee], c.File, idFile, importedFiles[c.File])
			}
			if tgt == "" || tgt == c.CallerID {
				continue
			}
			// Never bind a call to a definition in a different language family:
			// the same short name (render, parse, Path) collides across languages,
			// and a name match across a family boundary is a phantom edge, not a
			// real call. Unknown families (non-code, unrecognized ext) stay
			// permissive.
			if langfamily.Cross(c.File, idFile[tgt]) {
				continue
			}
			out.Edges = append(out.Edges, model.Edge{
				Source: c.CallerID, Target: tgt, Relation: "calls",
				Confidence: "INFERRED", SourceFile: c.File, SourceLocation: c.Loc,
			})
		}
	}

	// Inheritance: a declared supertype name binds the same way a call does —
	// same file first, then disambiguated among the definitions sharing the name.
	// An unresolvable base (a library type outside the corpus) drops rather than
	// creating a stub node.
	for _, r := range results {
		for _, t := range r.TypeRefs {
			tgt := ""
			if ids := local[t.File+"\x00"+t.Name]; len(ids) == 1 {
				tgt = ids[0]
			}
			if tgt == "" {
				tgt = disambiguate(global[t.Name], t.File, idFile, importedFiles[t.File])
			}
			if tgt == "" || tgt == t.FromID || langfamily.Cross(t.File, idFile[tgt]) {
				continue
			}
			out.Edges = append(out.Edges, model.Edge{
				Source: t.FromID, Target: tgt, Relation: t.Relation,
				Confidence: "INFERRED", SourceFile: t.File, SourceLocation: t.Loc,
			})
		}
	}

	// Imports: relative specifiers resolve to a corpus file (imports_from, used
	// for cycle detection); bare specifiers become external dependency nodes.
	extSeen := map[string]bool{}
	// A file can import from the same module twice (a type import plus a value
	// import); only one imports_from edge survives dedupe, so TypeOnly must be
	// the AND over every import of that target, not whichever parsed first.
	impEdge := map[string]int{}
	for _, r := range results {
		for _, im := range r.Imps {
			if target := resolveRelImport(im.File, im.Spec, corpus); target != "" {
				tgtID := idutil.MakeID(target)
				if i, ok := impEdge[im.FileID+"\x00"+tgtID]; ok {
					if !im.TypeOnly {
						out.Edges[i].TypeOnly = false
					}
					continue
				}
				impEdge[im.FileID+"\x00"+tgtID] = len(out.Edges)
				out.Edges = append(out.Edges, model.Edge{
					Source: im.FileID, Target: tgtID, Relation: "imports_from",
					Confidence: "EXTRACTED", SourceFile: im.File, SourceLocation: im.Loc,
					TypeOnly: im.TypeOnly,
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

	// Markdown links: resolve each [text](target) to the concept node of the
	// markdown file it points at and emit a `references` edge. Directory
	// structure (an index.md owning its siblings) yields `contains` edges.
	resolveMarkdown(results, files, corpus, &out)

	// Stage C: complete partial cloudposse null-label ids across local wrapper
	// chains, using the module-source edges and invocation args captured above.
	resolveNullLabels(results, &out)

	// C# interface dispatch: join a single-implementer interface's method to the
	// implementing method so directed walks do not stop at the interface.
	resolveCSharpDispatch(&out)
	return out
}

// mdExts are the markdown suffixes a link target may resolve to in the corpus.
var mdExts = []string{".md", ".mdx", ".markdown"}

// mdCodeSymbol is the identifier shape a backtick `code` span must match to be a
// candidate reference to a code definition. The span may be qualified with `.`
// or `::` (`pkg.Widget`, `Widget::render`); spans with spaces or dashes
// (`git status`, `--flag`) are rejected as noise.
var mdCodeSymbol = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:(?:::|\.)[A-Za-z_][A-Za-z0-9_]*)*$`)

// mdCodeSep splits a qualified code span into its segments.
var mdCodeSep = regexp.MustCompile(`::|\.`)

// resolveMarkdown stitches markdown link references into `references` edges and
// adds the `contains` edges implied by directory structure (a directory's
// index.md owns the other markdown concepts in that directory). Link targets are
// resolved against the bundle root for a leading '/', otherwise relative to the
// linking file's directory; http(s)/mailto/in-page (#anchor) targets are ignored.
func resolveMarkdown(results []Result, files []string, corpus map[string]bool, out *model.Extraction) {
	// Name -> code definition ids, for resolving a backtick `symbol` span that does
	// not point at a markdown file. Defs are exactly the code definitions, so
	// markdown concept/heading nodes never appear here; a name mapping to more than
	// one definition is ambiguous and drops (no map iteration feeds this).
	symDefs := map[string][]string{}
	for _, r := range results {
		for _, d := range r.Defs {
			symDefs[d.Name] = append(symDefs[d.Name], d.ID)
		}
	}

	// Tokens each definition may be qualified by, so `pkg.Widget` can resolve.
	quals := defQualifiers(results)

	seenSym := map[string]bool{} // FromID\x00defID, to not emit a symbol edge twice
	for _, r := range results {
		for _, m := range r.MDRefs {
			if tgt := resolveMDTarget(m.File, m.Target, corpus); tgt != "" {
				out.Edges = append(out.Edges, model.Edge{
					Source: m.FromID, Target: idutil.MakeID(strings.TrimSuffix(tgt, path.Ext(tgt))),
					Relation: "references", Confidence: "EXTRACTED",
					SourceFile: m.File, SourceLocation: m.Loc,
				})
				continue
			}
			// Not a markdown link: a backtick `symbol` matching a unique code
			// definition becomes a references edge to that definition. Drop on
			// ambiguity (0 or >1 candidates) and on non-identifier noise.
			id := uniqueCodeDef(m.Target, symDefs, quals)
			if id == "" {
				continue
			}
			key := m.FromID + "\x00" + id
			if seenSym[key] {
				continue
			}
			seenSym[key] = true
			out.Edges = append(out.Edges, model.Edge{
				Source: m.FromID, Target: id, Relation: "references",
				Confidence: "EXTRACTED", SourceFile: m.File, SourceLocation: m.Loc,
			})
		}
	}

	// dir index.md --contains--> each sibling markdown concept in that directory.
	for _, f := range files {
		sf := filepath.ToSlash(f)
		if !isMarkdown(sf) || strings.EqualFold(stemName(sf), "index") {
			continue
		}
		dir := path.Dir(sf)
		var idx string
		for _, ext := range mdExts {
			if cand := path.Join(dir, "index"+ext); corpus[cand] {
				idx = cand
				break
			}
		}
		if idx == "" {
			continue
		}
		out.Edges = append(out.Edges, model.Edge{
			Source:   idutil.MakeID(strings.TrimSuffix(idx, path.Ext(idx))),
			Target:   idutil.MakeID(strings.TrimSuffix(sf, path.Ext(sf))),
			Relation: "contains", Confidence: "EXTRACTED", SourceFile: idx,
		})
	}
}

// defQualifiers maps each code definition id to the tokens a qualified markdown
// mention may cite it by: the labels of the nodes containing it (a class owning
// a method) and the segments of its source path. This is what lets `pkg.Widget`
// bind to a Widget defined under pkg/ while `time.sleep` stays off a repo's own
// sleep.
func defQualifiers(results []Result) map[string]map[string]bool {
	label := map[string]string{}
	parent := map[string]string{} // contained id -> containing id
	for _, r := range results {
		for _, n := range r.Nodes {
			label[n.ID] = n.Label
		}
		for _, e := range r.Edges {
			if e.Relation == "contains" {
				parent[e.Target] = e.Source
			}
		}
	}
	out := make(map[string]map[string]bool)
	for _, r := range results {
		for _, d := range r.Defs {
			toks := map[string]bool{}
			for _, seg := range strings.Split(filepath.ToSlash(d.File), "/") {
				toks[seg] = true
				toks[strings.TrimSuffix(seg, path.Ext(seg))] = true
			}
			// Walk the containment chain, bounded so a cyclic edge can't hang.
			for id, n := parent[d.ID], 0; id != "" && n < 16; id, n = parent[id], n+1 {
				if l := label[id]; l != "" {
					toks[l] = true
				}
			}
			out[d.ID] = toks
		}
	}
	return out
}

// uniqueCodeDef resolves a backtick code-span symbol to the single code
// definition it names, or "" when the span is not an identifier or when
// zero/several definitions survive (drop-on-ambiguity). For a qualified span
// (`pkg.Widget`, `Widget::render`) the last segment is the name and every
// preceding segment must be a qualifier the candidate answers to.
func uniqueCodeDef(sym string, index map[string][]string, quals map[string]map[string]bool) string {
	if !mdCodeSymbol.MatchString(sym) {
		return ""
	}
	segs := mdCodeSep.Split(sym, -1)
	name, prefix := segs[len(segs)-1], segs[:len(segs)-1]
	var hit string
	for _, id := range index[name] {
		ok := true
		for _, q := range prefix {
			if !quals[id][q] {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if hit != "" {
			return "" // several definitions match
		}
		hit = id
	}
	return hit
}

// resolveMDTarget maps a markdown link target to a markdown file in the corpus,
// returning its slash path, or "" when the link is external (http/mailto),
// in-page (#anchor), or points outside the corpus.
func resolveMDTarget(fromFile, target string, corpus map[string]bool) string {
	if i := strings.IndexByte(target, '#'); i >= 0 {
		target = target[:i] // drop in-page anchor
	}
	if target == "" || isExternalLink(target) {
		return ""
	}
	var base string
	if strings.HasPrefix(target, "/") {
		base = path.Clean(strings.TrimPrefix(target, "/"))
	} else {
		base = path.Clean(path.Join(path.Dir(filepath.ToSlash(fromFile)), target))
	}
	if corpus[base] {
		return base
	}
	for _, ext := range mdExts {
		if corpus[base+ext] {
			return base + ext
		}
	}
	return ""
}

// isExternalLink reports whether a markdown link target points off-corpus
// (an absolute URL or a mail link) rather than at another bundle file.
func isExternalLink(target string) bool {
	lower := strings.ToLower(target)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:")
}

func isMarkdown(p string) bool {
	for _, ext := range mdExts {
		if strings.EqualFold(path.Ext(p), ext) {
			return true
		}
	}
	return false
}

// stemName returns a file's base name without its extension.
func stemName(p string) string {
	b := path.Base(p)
	return strings.TrimSuffix(b, path.Ext(b))
}

// resolveImportGuided emits EXTRACTED calls edges for Python calls backed by
// explicit `from M import N [as L]` evidence. It builds a (module_stem, symbol)
// index from all definitions, then for each per-file alias resolves a matching
// bare call to the unique definition. Member calls and self-edges are skipped.
// It returns the set of call sites it resolved (keyed callerID\x00callee\x00loc)
// so the generic name pass leaves them alone.
func resolveImportGuided(results []Result, idFile map[string]string, out *model.Extraction) map[string]bool {
	// (module_stem, symbol) -> def ids, used only when an import names that symbol.
	index := map[string][]string{}
	for _, r := range results {
		for _, d := range r.Defs {
			stem := defStem(d.File)
			if stem == "" {
				continue
			}
			index[stem+"\x00"+d.Name] = append(index[stem+"\x00"+d.Name], d.ID)
		}
	}

	resolved := map[string]bool{}
	for _, r := range results {
		if len(r.ImportAliases) == 0 {
			continue
		}
		aliases := map[string]ImportAlias{}
		for _, a := range r.ImportAliases {
			aliases[a.Local] = a // last write wins, mirroring upstream alias dict
		}
		for _, c := range r.Calls {
			if c.IsMember {
				continue
			}
			a, ok := aliases[c.Callee]
			if !ok {
				continue
			}
			ids := index[a.ModuleStem+"\x00"+a.Imported]
			if len(ids) != 1 || ids[0] == c.CallerID {
				continue
			}
			// Same cross-family guard as the generic pass: import evidence still
			// must not bind a call to a definition in a different language family.
			if langfamily.Cross(c.File, idFile[ids[0]]) {
				continue
			}
			resolved[c.CallerID+"\x00"+c.Callee+"\x00"+c.Loc] = true
			out.Edges = append(out.Edges, model.Edge{
				Source: c.CallerID, Target: ids[0], Relation: "calls",
				Confidence: "EXTRACTED", ConfidenceScore: 1.0,
				SourceFile: c.File, SourceLocation: c.Loc,
			})
		}
	}
	return resolved
}

// defStem returns a definition file's bare stem (basename without extension),
// the key import-guided resolution matches a module stem against.
func defStem(file string) string {
	base := filepath.Base(file)
	return strings.TrimSuffix(base, filepath.Ext(base))
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

// contains reports whether ids already holds id.
func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
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
	// The `@/` alias is the de-facto project-root convention in Next.js/Vite/
	// modern-TS repos; resolve it against the corpus root so those imports
	// become imports_from edges instead of external dependency nodes.
	if strings.HasPrefix(spec, "@/") {
		return resolveModulePath(path.Clean(strings.TrimPrefix(spec, "@/")), corpus)
	}
	if spec == "" || (spec[0] != '.' && spec[0] != '/') {
		return "" // bare specifier — an external package
	}
	return resolveModulePath(path.Clean(path.Join(path.Dir(filepath.ToSlash(fromFile)), spec)), corpus)
}

// resolveModulePath probes a corpus-relative module path as-is, then with each
// JS/TS extension, then as a directory index file. Returns "" if none exist.
func resolveModulePath(base string, corpus map[string]bool) string {
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
