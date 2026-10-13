package extract

import (
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

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
	return Join(ResolveParts(results, files))
}

// Join copies the parts of an extraction into one.
func Join(nodes [][]model.Node, edges [][]model.Edge) model.Extraction {
	nNodes, nEdges := 0, 0
	for _, p := range nodes {
		nNodes += len(p)
	}
	for _, p := range edges {
		nEdges += len(p)
	}
	out := model.Extraction{Nodes: make([]model.Node, 0, nNodes), Edges: make([]model.Edge, 0, nEdges)}
	for _, p := range nodes {
		out.Nodes = append(out.Nodes, p...)
	}
	for _, p := range edges {
		out.Edges = append(out.Edges, p...)
	}
	return out
}

// ResolveParts is Resolve without the copy into one slice: the nodes and edges
// come back in parts, in the order Resolve joins them. The per-file parts are
// the slices of results, so the caller must not write to them. One slice of a
// whole corpus's edges is over 100 MB, and an allocation that size lands on
// new memory however much of the heap is free.
func ResolveParts(results []Result, files []string) ([][]model.Node, [][]model.Edge) {
	var fc fileMemo // for the passes that run on this goroutine
	corpus := make(map[string]bool, len(files))
	for _, f := range files {
		corpus[filepath.ToSlash(f)] = true
	}

	// The qualifier and owner indexes read the results alone, so they are built
	// beside the indexes below.
	var quals map[string]qualSet
	var owner map[string]string
	methods := map[string][]string{} // owner\x00name -> method ids
	var defsDone sync.WaitGroup
	defsDone.Go(func() { quals = defQualifiers(results) })
	defsDone.Go(func() {
		owner = defOwners(results)
		for _, r := range results {
			for _, d := range r.Defs {
				if o := owner[d.ID]; o != "" && !contains(methods[o+"\x00"+d.Name], d.ID) {
					methods[o+"\x00"+d.Name] = append(methods[o+"\x00"+d.Name], d.ID)
				}
			}
		}
	})

	// Index definitions by name (global) and by file+name (local-first calls),
	// and remember each definition's file for disambiguation.
	global := map[string][]string{}
	local := map[[2]string][]string{} // file, name -> ids
	idFile := map[string]string{}     // def id -> defining file
	// Concrete-only indexes: a bodiless declaration must not make a call to
	// its single implementation ambiguous. Kept only for names that have such
	// a declaration; for every other name they would repeat local and global,
	// which is what a miss falls back to.
	localC := map[[2]string][]string{}
	globalC := map[string][]string{}
	// Filled beside the import targets below, which do not read them.
	defsDone.Go(func() {
		abstract := map[string]bool{}
		for _, r := range results {
			for _, d := range r.Defs {
				if d.Abstract {
					abstract[d.Name] = true
				}
			}
		}
		for _, r := range results {
			for _, d := range r.Defs {
				global[d.Name] = append(global[d.Name], d.ID)
				key := [2]string{d.File, d.Name}
				if !contains(local[key], d.ID) {
					local[key] = append(local[key], d.ID)
				}
				idFile[d.ID] = d.File
				if !d.Abstract && abstract[d.Name] {
					globalC[d.Name] = append(globalC[d.Name], d.ID)
					if !contains(localC[key], d.ID) {
						localC[key] = append(localC[key], d.ID)
					}
				}
			}
		}
	})

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
		if dir := goModDir(goMods, f); dir != "" {
			rel := pkg
			if dir != "." {
				rel = strings.TrimPrefix(pkg, dir)
			}
			ip := path.Join(goMods[dir], rel)
			goPkgs[ip] = append(goPkgs[ip], f)
		}
	}

	// Imports: relative specifiers resolve to a corpus file (imports_from, used
	// for cycle detection); bare specifiers become external dependency nodes.
	// They depend on the targets alone, so they are made beside the targets,
	// the indexes and the calls, a batch of files behind the loop below, and
	// appended after the calls.
	var impNodes []model.Node
	// Held until the calls are bound, so only what the import itself does not
	// say is kept: 32 bytes an import against an Edge's 120.
	type impRec struct {
		imp                *Imp // source, file and location
		tgt                string
		typeOnly, external bool
	}
	var impEdges [][]impRec // parts of impPart
	type impBatch struct {
		results []Result
		targets [][]string // per import, in results order
	}
	const batchFiles = 256
	batches := make(chan impBatch, len(results)/batchFiles+1) // never blocks the sender
	var importsDone sync.WaitGroup
	importsDone.Go(func() {
		extSeen := map[string]bool{}
		// A file can import from the same module twice (a type import plus a value
		// import); only one imports_from edge survives dedupe, so TypeOnly must be
		// the AND over every import of that target, not whichever parsed first.
		// Keyed by the numbers of the source and target ids: the two strings
		// are 32 bytes a key, and a Go corpus has an edge per call edge.
		impEdge := map[uint64]uint32{}
		srcNum, tgtNum := map[string]uint32{}, map[string]uint32{}
		// A Go package is imported by many files; each target id is made once.
		type tgtID struct {
			id  string
			num uint32
		}
		ids := map[string]tgtID{}
		n := 0
		add := func(e impRec) {
			if n%impPart == 0 {
				impEdges = append(impEdges, make([]impRec, 0, impPart))
			}
			impEdges[n/impPart] = append(impEdges[n/impPart], e)
			n++
		}
		for b := range batches {
			nextImp := 0
			for _, r := range b.results {
				for k := range r.Imps {
					im := &r.Imps[k]
					src, ok := srcNum[im.FileID]
					if !ok {
						src = uint32(len(srcNum))
						srcNum[im.FileID] = src
					}
					targets := b.targets[nextImp]
					nextImp++
					for _, target := range targets {
						t, ok := ids[target]
						if !ok {
							t.id = idutil.MakeID(target)
							if t.num, ok = tgtNum[t.id]; !ok {
								t.num = uint32(len(tgtNum))
								tgtNum[t.id] = t.num
							}
							ids[target] = t
						}
						ekey := uint64(src)<<32 | uint64(t.num)
						if i, ok := impEdge[ekey]; ok {
							if !im.TypeOnly {
								impEdges[i/impPart][i%impPart].typeOnly = false
							}
							continue
						}
						impEdge[ekey] = uint32(n)
						add(impRec{imp: im, tgt: t.id, typeOnly: im.TypeOnly})
					}
					depID := idutil.MakeID(im.Spec)
					// A bare `from . import x` has no module name to mint a node from.
					if len(targets) > 0 || depID == "" {
						continue
					}
					if !extSeen[depID] {
						extSeen[depID] = true
						impNodes = append(impNodes, model.Node{ID: depID, Label: im.Spec, FileType: "concept"})
					}
					add(impRec{imp: im, tgt: depID, external: true})
				}
			}
		}
	})

	// For each file, the corpus files of each of its imports — used to pick the
	// right target when a called name is defined in more than one file. The
	// lists are the ones the imports resolved to; a set per file was 20 MB on
	// a Go corpus, so importedSet makes it when a call needs it.
	impTargets := map[string][][]string{}
	external := map[[2]string]bool{} // file, spec of an import outside the corpus
	goTargets := map[[2]string][]string{}
	for lo := 0; lo < len(results); lo += batchFiles {
		b := impBatch{results: results[lo:min(lo+batchFiles, len(results))]}
		for _, r := range b.results {
			for _, im := range r.Imps {
				targets := importTargets(im, corpus, goPkgs, goMods, goTargets)
				b.targets = append(b.targets, targets)
				external[[2]string{im.File, im.Spec}] = len(targets) == 0
				if len(targets) > 0 {
					impTargets[im.File] = append(impTargets[im.File], targets)
				}
			}
		}
		batches <- b
	}
	close(batches)

	// Holds only the import-guided edges until the calls are bound: the
	// per-file records are copied in after that, once, so they are not a
	// second live copy while the indexes above are.
	var out model.Extraction

	defsDone.Wait()

	// Markdown links: resolve each [text](target) to the concept node of the
	// markdown file it points at and emit a `references` edge. Directory
	// structure (an index.md owning its siblings) yields `contains` edges.
	// It reads the indexes alone, so it runs beside the calls.
	var md model.Extraction
	var mdDone sync.WaitGroup
	mdDone.Go(func() { resolveMarkdown(results, files, corpus, quals, global, &md) })

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
	imported := importedSet{targets: impTargets}
	supers := map[string][]string{} // type id -> resolved base type ids
	openSuper := map[string]bool{}  // type id with a base outside the corpus
	for _, r := range results {
		for _, t := range r.TypeRefs {
			tgt := ""
			if ids := typeDefs(local[[2]string{t.File, t.Name}], t.Name, idFile); len(ids) == 1 {
				tgt = ids[0]
			}
			if tgt == "" {
				tgt = disambiguate(typeDefs(global[t.Name], t.Name, idFile), t.File, idFile, &imported)
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
				Source: t.FromID, Target: tgt, Relation: model.Rel(t.Relation),
				Confidence: model.ConfInferred, SourceFile: fc.of(t.File), SourceLocation: t.Loc,
			})
		}
	}

	// Calls: prefer a definition in the same file, else disambiguate among the
	// definitions sharing the called name (unique global, imported file, or same
	// package) rather than guessing. A member call binds on its receiver: self/this
	// to a method of the caller's own type or its nearest ancestor, anything
	// else only to a definition the receiver's last segment qualifies.
	// Each run of files binds against the indexes above, which no longer change,
	// so the runs are bound side by side and their edges joined in file order.
	bind := func(results []Result) [][]model.Edge {
		// In parts of impPart: one slice a run grew by copies, and the outgrown
		// ones were live with it at the collection that sets the peak. Only
		// the first part grows, so a small corpus does not pay for a full one.
		var edges [][]model.Edge
		add := func(e model.Edge) {
			n := len(edges)
			if n == 0 {
				edges = append(edges, nil)
				n++
			} else if len(edges[n-1]) == impPart {
				edges = append(edges, make([]model.Edge, 0, impPart))
				n++
			}
			edges[n-1] = append(edges[n-1], e)
		}
		imported := importedSet{targets: impTargets}
		kept := map[string][]string{}      // callee\x00qualifier -> the definitions it qualifies
		allQuals := map[string][]qualSet{} // callee -> quals of each id in its global list
		for _, r := range results {
			rf := model.FileOf(r.File)
			for i, n := range r.Calls {
				c := r.call(n)
				if len(resolved) > 0 && resolved[callSite{c.CallerID, c.Callee, c.Loc}] {
					continue
				}
				tgt := ""
				super, isSelf := selfRecv[c.Recv]
				if ext := langRecv[c.Recv]; ext != "" && path.Ext(r.File) != ext {
					isSelf = false
				}
				if isSelf {
					tgt = selfTarget(owner[c.CallerID], c.Callee, super, methods, supers, openSuper)
				} else {
					lkey := [2]string{r.File, c.Callee}
					here, all := localC[lkey], globalC[c.Callee]
					if len(all) == 0 {
						here, all = local[lkey], global[c.Callee]
					}
					if c.Recv != "" {
						q := recvTail(c.Recv)
						ok := func(id string) bool { return q != "" && quals[id].has(q) }
						here = keep(here, ok)
						// all depends on the callee alone and can run to thousands
						// of ids, so the filtered list is kept per qualifier, and
						// each id's qualifier set is looked up once per callee.
						key := c.Callee + "\x00" + q
						ids, done := kept[key]
						if !done {
							qs, ok := allQuals[c.Callee]
							if !ok {
								qs = make([]qualSet, len(all))
								for i, id := range all {
									qs[i] = quals[id]
								}
								allQuals[c.Callee] = qs
							}
							if q != "" {
								for i, id := range all {
									if qs[i].has(q) {
										ids = append(ids, id)
									}
								}
							}
							kept[key] = ids
						}
						all = ids
					}
					// Two types in one file can each own a method of the same name; a
					// bare call then has no unambiguous local target, so fall through
					// to disambiguate rather than guess.
					if len(here) == 1 {
						tgt = here[0]
					}
					if tgt == "" {
						tgt = disambiguate(all, r.File, idFile, &imported)
					}
				}
				// A call on an external module has no definition to bind, so it
				// lands on the module's import node.
				if tgt == "" && i < len(r.CallMods) && r.CallMods[i] != "" && external[[2]string{r.File, r.CallMods[i]}] {
					add(model.Edge{
						Source: c.CallerID, Target: idutil.MakeID(r.CallMods[i]), Relation: model.RelCalls,
						Confidence: model.ConfExtracted, SourceFile: rf, SourceLocation: c.Loc,
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
				if langfamily.Cross(r.File, idFile[tgt]) {
					continue
				}
				add(model.Edge{
					Source: c.CallerID, Target: tgt, Relation: model.RelCalls,
					Confidence: model.ConfInferred, SourceFile: rf, SourceLocation: c.Loc,
				})
			}
		}
		return edges
	}
	nCalls := 0
	for _, r := range results {
		nCalls += len(r.Calls)
	}
	perRun := nCalls/runtime.GOMAXPROCS(0) + 1
	var runs [][]Result
	for lo, hi, n := 0, 0, 0; hi < len(results); hi++ {
		if n += len(results[hi].Calls); n >= perRun || hi+1 == len(results) {
			runs = append(runs, results[lo:hi+1])
			lo, n = hi+1, 0
		}
	}
	callEdges := make([][][]model.Edge, len(runs))
	var callsDone sync.WaitGroup
	for i, run := range runs {
		callsDone.Go(func() { callEdges[i] = bind(run) })
	}
	callsDone.Wait()
	importsDone.Wait()
	nodes := make([][]model.Node, 0, len(results)+1)
	nParts := len(impEdges)
	for _, parts := range callEdges {
		nParts += len(parts)
	}
	edges := make([][]model.Edge, 0, len(results)+nParts+6)
	for _, r := range results {
		nodes = append(nodes, r.Nodes)
		edges = append(edges, r.Edges)
	}
	edges = append(edges, out.Edges) // import-guided
	for _, parts := range callEdges {
		edges = append(edges, parts...)
	}
	edges = append(edges, inherit)

	// A Go corpus has as many import edges as all the rest, so they go in
	// parts too.
	for _, recs := range impEdges {
		part := make([]model.Edge, 0, len(recs))
		for _, e := range recs {
			rel := model.RelImportsFrom
			if e.external {
				rel = model.RelImports
			}
			part = append(part, model.Edge{
				Source: e.imp.FileID, Target: e.tgt, Relation: rel,
				Confidence: model.ConfExtracted, SourceFile: fc.of(e.imp.File), SourceLocation: e.imp.Loc,
				TypeOnly: e.typeOnly,
			})
		}
		edges = append(edges, part)
	}
	// From here out holds what the passes below add.
	out.Nodes, out.Edges = impNodes, nil

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
								Source: dirID, Target: idutil.MakeID(ff), Relation: model.RelContains,
								Confidence: model.ConfExtracted, SourceFile: fc.of(target),
							})
						}
					}
					out.Edges = append(out.Edges, model.Edge{
						Source: m.FromID, Target: dirID, Relation: model.RelReferences,
						Confidence: model.ConfExtracted, SourceFile: fc.of(m.File), SourceLocation: m.Loc,
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
				Source: m.FromID, Target: extID, Relation: model.RelReferences,
				Confidence: model.ConfExtracted, SourceFile: fc.of(m.File), SourceLocation: m.Loc,
			})
		}
	}

	mdDone.Wait()
	nodes, edges = append(nodes, out.Nodes), append(edges, out.Edges, md.Edges)

	// Stage C: complete partial cloudposse null-label ids across local wrapper
	// chains, using the module-source edges and invocation args captured above.
	// It writes to the nodes, so it gets a joined copy.
	if slices.ContainsFunc(results, func(r Result) bool { return len(r.NullLabels) > 0 }) {
		out = Join(nodes, edges)
		resolveNullLabels(results, &out)
		nodes, edges = [][]model.Node{out.Nodes}, [][]model.Edge{out.Edges}
	}

	// C# interface dispatch: join a single-implementer interface's method to the
	// implementing method so directed walks do not stop at the interface.
	return nodes, append(edges, resolveCSharpDispatch(nodes, edges))
}

// impPart is how many import edges ResolveParts puts in one part.
const impPart = 4096

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
// quals holds the tokens each definition may be qualified by (defQualifiers), so
// `pkg.Widget` can resolve. symDefs maps a name to its code definition ids, in
// results order, for a backtick `symbol` span that does not point at a markdown
// file. Defs are exactly the code definitions, so markdown concept/heading nodes
// never appear there; a name mapping to more than one definition is ambiguous
// and drops (no map iteration feeds this).
func resolveMarkdown(results []Result, files []string, corpus map[string]bool, quals map[string]qualSet, symDefs map[string][]string, out *model.Extraction) {
	var fc fileMemo
	seenSym := map[string]bool{} // FromID\x00defID, to not emit a symbol edge twice
	for _, r := range results {
		for _, m := range r.MDRefs {
			if tgt := resolveMDTarget(m.File, m.Target, corpus); tgt != "" {
				out.Edges = append(out.Edges, model.Edge{
					Source: m.FromID, Target: idutil.MakeID(strings.TrimSuffix(tgt, path.Ext(tgt))),
					Relation: model.RelReferences, Confidence: model.ConfExtracted,
					SourceFile: fc.of(m.File), SourceLocation: m.Loc,
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
				Source: m.FromID, Target: id, Relation: model.RelReferences,
				Confidence: model.ConfExtracted, SourceFile: fc.of(m.File), SourceLocation: m.Loc,
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
			Relation: model.RelContains, Confidence: model.ConfExtracted, SourceFile: fc.of(idx),
		})
	}
}

// defQualifiers maps each code definition id to the tokens a qualified markdown
// mention may cite it by: the labels of the nodes containing it (a class owning
// a method) and the segments of its source path. This is what lets `pkg.Widget`
// bind to a Widget defined under pkg/ while `time.sleep` stays off a repo's own
// sleep.
func defQualifiers(results []Result) map[string]qualSet {
	label := map[string]string{}
	parent := map[string]string{} // contained id -> containing id
	for _, r := range results {
		for _, n := range r.Nodes {
			label[n.ID] = n.Label
		}
		for _, e := range r.Edges {
			if e.Relation == model.RelContains {
				parent[e.Target] = e.Source
			}
		}
	}
	out := make(map[string]qualSet)
	for _, r := range results {
		var toks qualSet
		file := ""
		for _, d := range r.Defs {
			if toks.path == nil || d.File != file {
				file, toks.path = d.File, map[string]bool{}
				for _, seg := range strings.Split(filepath.ToSlash(d.File), "/") {
					toks.path[seg] = true
					toks.path[strings.TrimSuffix(seg, path.Ext(seg))] = true
				}
			}
			toks.own = nil
			// Walk the containment chain, bounded so a cyclic edge can't hang.
			for id, n := parent[d.ID], 0; id != "" && n < 16; id, n = parent[id], n+1 {
				if l := label[id]; l != "" && !toks.has(l) {
					toks.own = append(toks.own, l)
				}
			}
			if d.Owner != "" && !toks.has(d.Owner) {
				toks.own = append(toks.own, d.Owner)
			}
			out[d.ID] = toks
		}
	}
	return out
}

// qualSet is the tokens one definition answers to: the path tokens, shared by
// every definition of its file, and the labels and owner that are its own.
type qualSet struct {
	path map[string]bool
	own  []string
}

func (s qualSet) has(q string) bool { return s.path[q] || contains(s.own, q) }

// selfRecv lists the receivers that name the caller's own instance or type;
// the value marks one that starts the lookup at the parent type.
var selfRecv = map[string]bool{
	"self": false, "this": false, "cls": false, "$this": false, "@self": false, "Self": false, "static": false,
	"super": true, "super()": true, "parent": true, "base": true,
}

// langRecv limits a receiver that is a plain identifier in most languages to
// the one where it is a keyword.
var langRecv = map[string]string{"static": ".php", "parent": ".php", "base": ".cs"}

// recvTail returns the last identifier of a receiver (`a.b` -> `b`); a receiver
// ending in anything else (`f()`, `a[0]`) has none.
func recvTail(s string) string {
	i := len(s)
	for i > 0 {
		r, n := rune(s[i-1]), 1
		if r >= utf8.RuneSelf {
			r, n = utf8.DecodeLastRuneInString(s[:i])
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsNumber(r) {
			break
		}
		i -= n
	}
	return s[i:]
}

// defOwners maps each method to the type that owns it: the node containing it,
// or for a Go method its receiver type within the package directory. A
// definition contained only by its file has no owner.
func defOwners(results []Result) map[string]string {
	owner := map[string]string{}
	for _, r := range results {
		for _, e := range r.Edges {
			if e.Relation == model.RelContains {
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
func uniqueCodeDef(sym string, index map[string][]string, quals map[string]qualSet) string {
	if !mdCodeSymbol.MatchString(sym) {
		return ""
	}
	segs := mdCodeSep.Split(sym, -1)
	name, prefix := segs[len(segs)-1], segs[:len(segs)-1]
	var hit string
	for _, id := range index[name] {
		ok := true
		for _, q := range prefix {
			if !quals[id].has(q) {
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

// fileMemo gives the code of a path. Edges of one file come in runs, so the
// last answer saves most lookups in the table.
type fileMemo struct {
	s string
	f model.File
}

func (m *fileMemo) of(s string) model.File {
	if s != m.s {
		m.s, m.f = s, model.FileOf(s)
	}
	return m.f
}

// callSite identifies a call inside one corpus.
type callSite struct {
	caller, callee string
	loc            model.Loc
}

// resolveImportGuided emits EXTRACTED calls edges for Python calls backed by
// explicit `from M import N [as L]` evidence. It builds a (module_stem, symbol)
// index from all definitions, then for each per-file alias resolves a matching
// bare call to the unique definition. Member calls and self-edges are skipped.
// It returns the set of call sites it resolved so the generic name pass leaves
// them alone.
func resolveImportGuided(results []Result, idFile map[string]string, out *model.Extraction) map[callSite]bool {
	// (module_stem, symbol) -> def ids, used only when an import names that symbol.
	// Only a stem some import names is ever looked up.
	want := map[string]bool{}
	for _, r := range results {
		for _, a := range r.ImportAliases {
			want[a.ModuleStem] = true
		}
	}
	index := map[string][]string{}
	for _, r := range results {
		if len(want) == 0 {
			break
		}
		for _, d := range r.Defs {
			stem := defStem(d.File)
			if stem == "" || !want[stem] {
				continue
			}
			index[stem+"\x00"+d.Name] = append(index[stem+"\x00"+d.Name], d.ID)
		}
	}

	resolved := map[callSite]bool{}
	for _, r := range results {
		if len(r.ImportAliases) == 0 {
			continue
		}
		aliases := map[string]ImportAlias{}
		for _, a := range r.ImportAliases {
			aliases[a.Local] = a // last write wins, mirroring upstream alias dict
		}
		rf := model.FileOf(r.File)
		for _, n := range r.Calls {
			c := r.call(n)
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
			if langfamily.Cross(r.File, idFile[ids[0]]) {
				continue
			}
			resolved[callSite{c.CallerID, c.Callee, c.Loc}] = true
			out.Edges = append(out.Edges, model.Edge{
				Source: c.CallerID, Target: ids[0], Relation: model.RelCalls,
				Confidence: model.ConfExtracted, ConfidenceScore: 1.0,
				SourceFile: rf, SourceLocation: c.Loc,
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
func disambiguate(ids []string, callerFile string, idFile map[string]string, importedBy *importedSet) string {
	switch len(ids) {
	case 0:
		return ""
	case 1:
		return ids[0]
	}
	imported := importedBy.of(callerFile)
	if id := unique(ids, func(id string) bool { return imported[idFile[id]] }); id != "" {
		return id
	}
	dir := path.Dir(filepath.ToSlash(callerFile))
	return unique(ids, func(id string) bool { return path.Dir(idFile[id]) == dir })
}

// importedSet gives the set of corpus files a file imports, from the target
// lists of its imports. It holds one set at a time: calls come file by file.
type importedSet struct {
	targets map[string][][]string
	file    string
	set     map[string]bool
}

func (s *importedSet) of(file string) map[string]bool {
	if s.set != nil && file == s.file {
		return s.set
	}
	if s.set == nil {
		s.set = map[string]bool{}
	}
	clear(s.set)
	for _, ts := range s.targets[file] {
		for _, t := range ts {
			s.set[t] = true
		}
	}
	s.file = file
	return s.set
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

// goModDir returns the directory of the nearest go.mod above file f, or "".
func goModDir(goMods map[string]string, f string) string {
	for dir := path.Dir(f); ; dir = path.Dir(dir) {
		if _, ok := goMods[dir]; ok {
			return dir
		}
		if dir == "." || dir == "/" {
			return ""
		}
	}
}

// importTargets returns the corpus files an import binds to: at most one for a
// path specifier, for Python the module plus any imported name that is itself a
// module (`from . import b`, `from pkg import submodule`), for Rust the module
// each used name lives in, and for Go every non-test file of the imported package.
//
// goTargets holds the file list of each Go package as one importing module
// sees it. Every importer outside the package gets that same slice, so the
// caller must not change what it is given.
func importTargets(im Imp, corpus map[string]bool, goPkgs map[string][]string, goMods map[string]string, goTargets map[[2]string][]string) []string {
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
		own := goModDir(goMods, from)
		key := [2]string{im.Spec, own}
		pkg, ok := goTargets[key]
		if !ok {
			all := goPkgs[im.Spec]
			// Two checkouts of one module: bind inside the importer's own.
			if mine := keep(all, func(f string) bool { return goModDir(goMods, f) == own }); len(mine) > 0 {
				all = mine
			}
			for _, f := range all {
				if f != "" && !contains(pkg, f) {
					pkg = append(pkg, f)
				}
			}
			goTargets[key] = pkg
		}
		if !contains(pkg, from) {
			return pkg
		}
		for _, f := range pkg {
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
	// One reused buffer: a missed probe then allocates nothing.
	buf := make([]byte, 0, len(base)+len("/index.d.ts"))
	buf = append(buf, base...)
	for _, ext := range jsResolveExts {
		if buf = append(buf[:len(base)], ext...); corpus[string(buf)] {
			return string(buf)
		}
	}
	buf = append(buf[:len(base)], "/index"...)
	for _, ext := range jsResolveExts {
		if buf = append(buf[:len(base)+len("/index")], ext...); corpus[string(buf)] {
			return string(buf)
		}
	}
	return ""
}
