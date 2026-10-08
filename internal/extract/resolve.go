package extract

import (
	"net/url"
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
	// Concrete-only indexes: a bodiless declaration must not make a call to
	// its single implementation ambiguous.
	localC := map[string][]string{}
	globalC := map[string][]string{}
	for _, r := range results {
		for _, d := range r.Defs {
			global[d.Name] = append(global[d.Name], d.ID)
			key := d.File + "\x00" + d.Name
			if !contains(local[key], d.ID) {
				local[key] = append(local[key], d.ID)
			}
			idFile[d.ID] = d.File
			if !d.Abstract {
				globalC[d.Name] = append(globalC[d.Name], d.ID)
				if !contains(localC[key], d.ID) {
					localC[key] = append(localC[key], d.ID)
				}
			}
		}
	}

	// Go import path -> the files of that package, for every .go file under a
	// go.mod in the corpus (the nearest one above the file names its module).
	goMods := map[string]string{}
	for _, r := range results {
		for dir, mod := range r.GoMods {
			goMods[dir] = mod
		}
	}
	goPkgs := map[string][]string{}
	for _, f := range files {
		f = filepath.ToSlash(f)
		// Importers never see a package's test files.
		if len(goMods) == 0 || !strings.HasSuffix(f, ".go") || strings.HasSuffix(f, "_test.go") {
			continue
		}
		pkg := path.Dir(f)
		for dir := pkg; ; dir = path.Dir(dir) {
			if mod, ok := goMods[dir]; ok {
				rel := pkg
				if dir != "." {
					rel = strings.TrimPrefix(pkg, dir)
				}
				ip := path.Join(mod, rel)
				goPkgs[ip] = append(goPkgs[ip], f)
				break
			}
			if dir == "." || dir == "/" {
				break
			}
		}
	}

	// For each file, the corpus files it imports — used to pick the right target
	// when a called name is defined in more than one file.
	importedFiles := map[string]map[string]bool{}
	external := map[string]bool{} // file\x00spec of an import outside the corpus
	for _, r := range results {
		for _, im := range r.Imps {
			targets := importTargets(im, corpus, goPkgs)
			external[im.File+"\x00"+im.Spec] = len(targets) == 0
			for _, target := range targets {
				if importedFiles[im.File] == nil {
					importedFiles[im.File] = map[string]bool{}
				}
				importedFiles[im.File][target] = true
			}
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

	// Inheritance: a declared supertype name binds the same way a call does —
	// same file first, then disambiguated among the definitions sharing the name.
	// An unresolvable base (a library type outside the corpus) drops rather than
	// creating a stub node. Resolved ahead of the calls so a self/super call can
	// walk the chain; the edges are appended after them.
	var inherit []model.Edge
	supers := map[string][]string{} // type id -> resolved base type ids
	openSuper := map[string]bool{}  // type id with a base outside the corpus
	for _, r := range results {
		for _, t := range r.TypeRefs {
			tgt := ""
			if ids := typeDefs(local[t.File+"\x00"+t.Name], t.Name, idFile); len(ids) == 1 {
				tgt = ids[0]
			}
			if tgt == "" {
				tgt = disambiguate(typeDefs(global[t.Name], t.Name, idFile), t.File, idFile, importedFiles[t.File])
			}
			if tgt == "" || tgt == t.FromID || langfamily.Cross(t.File, idFile[tgt]) {
				if t.Relation == "inherits" {
					openSuper[t.FromID] = true
				}
				continue
			}
			if t.Relation == "inherits" {
				supers[t.FromID] = append(supers[t.FromID], tgt)
			}
			inherit = append(inherit, model.Edge{
				Source: t.FromID, Target: tgt, Relation: t.Relation,
				Confidence: "INFERRED", SourceFile: t.File, SourceLocation: t.Loc,
			})
		}
	}

	quals := defQualifiers(results)
	owner := defOwners(results)
	methods := map[string][]string{} // owner\x00name -> method ids
	for _, r := range results {
		for _, d := range r.Defs {
			if o := owner[d.ID]; o != "" && !contains(methods[o+"\x00"+d.Name], d.ID) {
				methods[o+"\x00"+d.Name] = append(methods[o+"\x00"+d.Name], d.ID)
			}
		}
	}

	// Calls: prefer a definition in the same file, else disambiguate among the
	// definitions sharing the called name (unique global, imported file, or same
	// package) rather than guessing. A member call binds on its receiver: self/this
	// to a method of the caller's own type or its nearest ancestor, anything
	// else only to a definition the receiver's last segment qualifies.
	for _, r := range results {
		for _, c := range r.Calls {
			if resolved[c.CallerID+"\x00"+c.Callee+"\x00"+c.Loc] {
				continue
			}
			tgt := ""
			super, isSelf := selfRecv[c.Recv]
			if ext := langRecv[c.Recv]; ext != "" && path.Ext(c.File) != ext {
				isSelf = false
			}
			if isSelf {
				tgt = selfTarget(owner[c.CallerID], c.Callee, super, methods, supers, openSuper)
			} else {
				here, all := local[c.File+"\x00"+c.Callee], global[c.Callee]
				if len(globalC[c.Callee]) > 0 {
					here, all = localC[c.File+"\x00"+c.Callee], globalC[c.Callee]
				}
				if c.Recv != "" {
					q := recvTail.FindString(c.Recv)
					ok := func(id string) bool { return q != "" && quals[id][q] }
					here, all = keep(here, ok), keep(all, ok)
				}
				// Two types in one file can each own a method of the same name; a
				// bare call then has no unambiguous local target, so fall through
				// to disambiguate rather than guess.
				if len(here) == 1 {
					tgt = here[0]
				}
				if tgt == "" {
					tgt = disambiguate(all, c.File, idFile, importedFiles[c.File])
				}
			}
			// A call on an external module has no definition to bind, so it
			// lands on the module's import node.
			if tgt == "" && c.Module != "" && external[c.File+"\x00"+c.Module] {
				out.Edges = append(out.Edges, model.Edge{
					Source: c.CallerID, Target: idutil.MakeID(c.Module), Relation: "calls",
					Confidence: "EXTRACTED", SourceFile: c.File, SourceLocation: c.Loc,
				})
				continue
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

	out.Edges = append(out.Edges, inherit...)

	// Imports: relative specifiers resolve to a corpus file (imports_from, used
	// for cycle detection); bare specifiers become external dependency nodes.
	extSeen := map[string]bool{}
	// A file can import from the same module twice (a type import plus a value
	// import); only one imports_from edge survives dedupe, so TypeOnly must be
	// the AND over every import of that target, not whichever parsed first.
	impEdge := map[string]int{}
	for _, r := range results {
		for _, im := range r.Imps {
			targets := importTargets(im, corpus, goPkgs)
			for _, target := range targets {
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
			}
			depID := idutil.MakeID(im.Spec)
			// A bare `from . import x` has no module name to mint a node from.
			if len(targets) > 0 || depID == "" {
				continue
			}
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
			if d.Owner != "" {
				toks[d.Owner] = true
			}
			out[d.ID] = toks
		}
	}
	return out
}

// selfRecv lists the receivers that name the caller's own instance or type;
// the value marks one that starts the lookup at the parent type.
var selfRecv = map[string]bool{
	"self": false, "this": false, "cls": false, "$this": false, "@self": false, "Self": false, "static": false,
	"super": true, "super()": true, "parent": true, "base": true,
}

// langRecv limits a receiver that is a plain identifier in most languages to
// the one where it is a keyword.
var langRecv = map[string]string{"static": ".php", "parent": ".php", "base": ".cs"}

// recvTail matches the last identifier of a receiver (`a.b` -> `b`); a receiver
// ending in anything else (`f()`, `a[0]`) has none.
var recvTail = regexp.MustCompile(`[\p{L}\p{N}_]+$`)

// defOwners maps each method to the type that owns it: the node containing it,
// or for a Go method its receiver type within the package directory. A
// definition contained only by its file has no owner.
func defOwners(results []Result) map[string]string {
	owner := map[string]string{}
	for _, r := range results {
		for _, e := range r.Edges {
			if e.Relation == "contains" {
				owner[e.Target] = e.Source
			}
		}
		for _, d := range r.Defs {
			if d.Owner != "" {
				owner[d.ID] = path.Dir(filepath.ToSlash(d.File)) + "\x00" + d.Owner
			} else if owner[d.ID] == idutil.MakeID(d.File) {
				delete(owner, d.ID)
			}
		}
	}
	return owner
}

// selfTarget binds a self/this call made from a method of type cls: the method
// of that name on cls, else on the nearest ancestor reached over resolved
// inherits edges. A super call starts at the parents. It returns "" on a tie or
// when the chain passes a base outside the corpus, rather than guess.
func selfTarget(cls, name string, super bool, methods, supers map[string][]string, openSuper map[string]bool) string {
	level := []string{cls}
	// Each class once, or a cyclic chain fans out per level.
	seen := map[string]bool{cls: true}
	// Bounded so a cyclic inherits chain can't hang.
	for depth := 0; cls != "" && len(level) > 0 && depth < 16; depth++ {
		var hits, next []string
		for _, c := range level {
			for _, id := range methods[c+"\x00"+name] {
				if !contains(hits, id) {
					hits = append(hits, id)
				}
			}
		}
		if super && depth == 0 {
			hits = nil
		}
		if len(hits) == 1 {
			return hits[0]
		}
		if len(hits) > 1 {
			return ""
		}
		for _, c := range level {
			if openSuper[c] {
				return ""
			}
			for _, p := range supers[c] {
				if !seen[p] {
					seen[p] = true
					next = append(next, p)
				}
			}
		}
		level = next
	}
	return ""
}

// keep returns the ids matching pred.
func keep(ids []string, pred func(string) bool) []string {
	var out []string
	for _, id := range ids {
		if pred(id) {
			out = append(out, id)
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
	// Raw first so literal %XX note names (and wikilinks) stay verbatim.
	if hit := lookupMD(fromFile, target, corpus); hit != "" {
		return hit
	}
	if u, err := url.PathUnescape(target); err == nil && u != target {
		return lookupMD(fromFile, u, corpus)
	}
	return ""
}

func lookupMD(fromFile, target string, corpus map[string]bool) string {
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
			if c.Recv != "" {
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

// typeDefs narrows a by-name candidate list to the top-level type definitions in
// it. A supertype reference can only bind to a type, and a Java constructor is
// registered under its class's name (`A.A()`), which would otherwise make every
// `class B extends A` lookup ambiguous and silently drop the edge. A top-level
// type's id is MakeID(fileStem(file), name) while a member's is nested under its
// owner, so the id shape separates the two.
func typeDefs(ids []string, name string, idFile map[string]string) []string {
	var out []string
	for _, id := range ids {
		if idutil.MakeID(fileStem(idFile[id]), name) == id {
			out = append(out, id)
		}
	}
	return out
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

// importTargets returns the corpus files an import binds to: at most one for a
// path specifier, for Python the module plus any imported name that is itself a
// module (`from . import b`, `from pkg import submodule`), for Rust the module
// each used name lives in, and for Go every non-test file of the imported package.
func importTargets(im Imp, corpus map[string]bool, goPkgs map[string][]string) []string {
	from := filepath.ToSlash(im.File)
	var out []string
	add := func(t string) {
		if t != "" && t != from && !contains(out, t) {
			out = append(out, t)
		}
	}
	switch path.Ext(from) {
	case ".py":
		add(resolvePyModule(from, im.Spec, corpus))
		for _, name := range im.Names {
			add(resolvePyModule(from, strings.TrimSuffix(im.Spec, ".")+"."+name, corpus))
		}
	case ".rs":
		if len(im.Names) == 0 {
			add(resolveRustPath(from, im.Spec, corpus))
		}
		for _, name := range im.Names {
			add(resolveRustPath(from, im.Spec+"::"+name, corpus))
		}
	case ".go":
		for _, f := range goPkgs[im.Spec] {
			add(f)
		}
	default:
		spec := im.Spec
		if im.Rel && !strings.HasPrefix(spec, ".") && !strings.HasPrefix(spec, "/") {
			spec = "./" + spec
		}
		if path.Ext(from) == ".rb" && path.Ext(spec) != ".rb" {
			spec += ".rb"
		}
		if t := resolveRelImport(im.File, spec, corpus); t != "" {
			return []string{t}
		}
	}
	return out
}

// resolveRustPath maps a `crate::`, `self::` or `super::` path to the corpus
// file of the deepest module it names (a/b.rs or a/b/mod.rs), dropping trailing
// segments that are items rather than modules. Any other path is external.
func resolveRustPath(from, spec string, corpus map[string]bool) string {
	segs := strings.Split(strings.TrimSuffix(spec, "::*"), "::")
	dir := path.Dir(from)
	// Children of mod.rs, lib.rs and main.rs sit beside it; any other file
	// keeps them in a directory named after itself.
	own := dir
	if b := path.Base(from); b != "mod.rs" && b != "lib.rs" && b != "main.rs" {
		own = path.Join(dir, strings.TrimSuffix(b, ".rs"))
	}
	var bases []string
	switch segs[0] {
	case "crate":
		for root := dir; ; root = path.Dir(root) {
			if corpus[path.Join(root, "lib.rs")] || corpus[path.Join(root, "main.rs")] {
				bases = []string{root}
				break
			}
			if root == "." || root == "/" {
				return ""
			}
		}
		segs = segs[1:]
	case "self":
		// A crate root that is not lib.rs/main.rs (src/bin/x.rs) also keeps
		// its children beside it.
		bases = []string{own, dir}
		segs = segs[1:]
	case "super":
		for ; len(segs) > 0 && segs[0] == "super"; segs = segs[1:] {
			own = path.Dir(own)
		}
		bases = []string{own}
	default:
		return ""
	}
	for _, base := range bases {
		for n := len(segs); n > 0; n-- {
			p := path.Join(base, path.Join(segs[:n]...))
			if corpus[p+".rs"] {
				return p + ".rs"
			}
			if corpus[p+"/mod.rs"] {
				return p + "/mod.rs"
			}
		}
	}
	return ""
}

// resolvePyModule maps a dotted Python module to a corpus file. Leading dots
// walk up from the importer's directory. An absolute module is probed from the
// corpus root and from each ancestor directory that is not itself a package,
// and binds only when exactly one of them holds it.
func resolvePyModule(from, mod string, corpus map[string]bool) string {
	rel := strings.TrimLeft(mod, ".")
	dots := len(mod) - len(rel)
	rel = strings.ReplaceAll(rel, ".", "/")
	probe := func(dir string) string {
		base := path.Join(dir, rel)
		if rel != "" && corpus[base+".py"] {
			return base + ".py"
		}
		if init := path.Join(base, "__init__.py"); corpus[init] {
			return init
		}
		return ""
	}
	dir := path.Dir(from)
	if dots > 0 {
		return probe(path.Join(dir, strings.Repeat("../", dots-1)))
	}
	found := probe(".")
	for ; dir != "."; dir = path.Dir(dir) {
		// A package's siblings are not importable by bare name.
		if corpus[path.Join(dir, "__init__.py")] {
			continue
		}
		if hit := probe(dir); hit != "" {
			if found != "" {
				return ""
			}
			found = hit
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
