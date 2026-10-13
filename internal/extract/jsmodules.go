package extract

import (
	"path"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/langfamily"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// jsLike reports whether a file's imports are JS/TS ones (see jsImp).
func jsLike(file string) bool {
	switch path.Ext(file) {
	case ".vue", ".svelte", ".astro":
		return true
	}
	return langfamily.Of(file) == "jsts"
}

// jsRe is one `export ... from` of a file, or an export of an imported name:
// it exports name of target as as. Both are empty for `export *`.
type jsRe struct{ target, name, as string }

// jsMods resolves what the import and export statements of JS/TS files name.
type jsMods struct {
	results  []Result
	corpus   map[string]bool
	local    map[[2]string][]string
	idFile   map[string]string
	defaults map[string]string // file -> local name of its default export
	reex     map[string][]jsRe // file -> its re-exports
	byFile   map[string]int    // file -> index in results, made by isFn
	exp      map[string][]Imp  // file -> its exports of imported names, no Names
	memo     map[[2]string]string
}

func newJSMods(results []Result, corpus map[string]bool, local map[[2]string][]string, idFile map[string]string) *jsMods {
	m := &jsMods{results: results, corpus: corpus, local: local, idFile: idFile, defaults: map[string]string{}, reex: map[string][]jsRe{}, exp: map[string][]Imp{}, memo: map[[2]string]string{}}
	for _, r := range results {
		if !jsLike(r.File) {
			continue
		}
		if r.Default != "" {
			m.defaults[r.File] = r.Default
		}
		for _, im := range r.Imps {
			if !im.Export {
				continue
			}
			t := importTargets(im, corpus, nil, nil, nil)
			if len(t) == 0 {
				continue
			}
			if len(im.Names) == 0 {
				m.reex[r.File] = append(m.reex[r.File], jsRe{target: t[0]})
			}
			for _, n := range im.Names {
				name, as, _ := strings.Cut(n, " as ")
				if as == "" {
					as = name
				}
				m.reex[r.File] = append(m.reex[r.File], jsRe{t[0], name, as})
			}
		}
		m.exports(&r)
	}
	return m
}

// exports finds the names `export { x as y }` of r takes from its imports and
// files them as re-exports of the imported module.
func (m *jsMods) exports(r *Result) {
	for _, e := range r.Exports {
		local, as, _ := strings.Cut(e, " as ")
		if as == "" {
			as = local
		}
		for _, im := range r.Imps {
			t := importTargets(im, m.corpus, nil, nil, nil)
			for _, n := range im.Names {
				name, l, _ := strings.Cut(n, " as ")
				if l == "" {
					l = name
				}
				if im.Export || len(t) == 0 || l != local || name == "*" {
					continue
				}
				m.reex[r.File] = append(m.reex[r.File], jsRe{t[0], name, as})
				ex := im
				ex.Export, ex.Names = true, nil
				m.exp[r.File] = append(m.exp[r.File], ex)
			}
		}
	}
}

// def is the top-level definition of name that file exports: its own, else one
// a chain of re-exports leads to. "default" is the file's own default export.
func (m *jsMods) def(file, name string) string {
	key := [2]string{file, name}
	id, ok := m.memo[key]
	if !ok {
		id = m.find(file, name, map[[2]string]bool{})
		m.memo[key] = id
	}
	return id
}

// find is def without the memo. seen stops a cycle of `export *` barrels.
func (m *jsMods) find(file, name string, seen map[[2]string]bool) string {
	key := [2]string{file, name}
	if seen[key] {
		return ""
	}
	seen[key] = true
	own := name
	if name == "default" {
		own = m.defaults[file]
	}
	if ids := typeDefs(m.local[[2]string{file, own}], own, m.idFile); own != "" && len(ids) > 0 {
		return ids[0]
	}
	for _, re := range m.reex[file] {
		from := re.name
		if re.as == "" && name != "default" {
			from = name
		} else if re.as != name {
			continue
		}
		if id := m.find(re.target, from, seen); id != "" {
			return id
		}
	}
	return ""
}

// each calls fn for every name an import or re-export of r binds to a
// definition: the import (or re-export) and the definition id. A re-export
// of a whole module has no name, so fn gets an empty id and the target file.
func (m *jsMods) each(r *Result, fn func(im *Imp, local, id, file string)) {
	for k := range r.Imps {
		im := &r.Imps[k]
		if !im.Export && len(im.Names) == 0 {
			continue
		}
		t := importTargets(*im, m.corpus, nil, nil, nil)
		if len(t) == 0 {
			continue
		}
		if im.Export {
			fn(im, "", "", t[0])
		}
		for _, n := range im.Names {
			name, local, _ := strings.Cut(n, " as ")
			if local == "" {
				local = name
			}
			if name == "*" || im.Export && name == "default" {
				continue
			}
			if id := m.def(t[0], name); id != "" {
				fn(im, local, id, t[0])
			}
		}
	}
}

// edges are the imports and re_exports edges of the JS/TS files.
func (m *jsMods) edges(fc *fileMemo) []model.Edge {
	var out []model.Edge
	for i := range m.results {
		r := &m.results[i]
		if !jsLike(r.File) {
			continue
		}
		m.each(r, func(im *Imp, _, id, file string) {
			e := model.Edge{
				Source: im.FileID, Target: id, Relation: model.RelImports,
				Confidence: model.ConfExtracted, SourceFile: fc.of(im.File), SourceLocation: im.Loc,
			}
			switch {
			case im.Export && id == "":
				e.Target, e.Relation = idutil.MakeID(file), model.RelReExports
			case im.Export:
				e.Relation = model.RelReExports
			}
			out = append(out, e)
		})
		for _, im := range m.exp[r.File] {
			out = append(out, model.Edge{
				Source: im.FileID, Target: idutil.MakeID(importTargets(im, m.corpus, nil, nil, nil)[0]), Relation: model.RelReExports,
				Confidence: model.ConfExtracted, SourceFile: fc.of(im.File), SourceLocation: im.Loc,
			})
		}
	}
	return out
}

// bound maps the local names r's named imports bind to their definitions.
func (m *jsMods) bound(r *Result) map[string]string {
	out := map[string]string{}
	m.each(r, func(im *Imp, local, id, _ string) {
		if !im.Export && id != "" {
			out[local] = id
		}
	})
	return out
}

// aliases maps (file, local name) to the definition of every name a file
// imports: a barrel can export it under a name no definition has.
func (m *jsMods) aliases() map[[2]string]string {
	out := map[[2]string]string{}
	for i := range m.results {
		r := &m.results[i]
		if jsLike(r.File) {
			m.each(r, func(im *Imp, local, id, _ string) {
				if !im.Export && id != "" {
					out[[2]string{r.File, local}] = id
				}
			})
		}
	}
	return out
}

// supertype is the id a heritage clause in r names. Upstream binds a name
// declared in the file or imported by name, nothing else, and a function is
// no base. A type inside a namespace is no Def, so the file's nodes are read.
func (m *jsMods) supertype(t TypeRef, r *Result, bound map[string]string) string {
	id := ""
	if ids := typeDefs(m.local[[2]string{t.File, t.Name}], t.Name, m.idFile); len(ids) > 0 {
		id = ids[0]
	} else if id = bound[t.Name]; id == "" {
		for _, n := range r.Nodes {
			if n.Label == t.Name && n.ID != t.FromID {
				id = n.ID
				break
			}
		}
	}
	if id == "" || m.isFn(id) {
		return ""
	}
	return id
}

// isFn reports whether the node of id is labelled as a function.
func (m *jsMods) isFn(id string) bool {
	if m.byFile == nil {
		m.byFile = map[string]int{}
		for i := range m.results {
			m.byFile[m.results[i].File] = i
		}
	}
	for _, n := range m.results[m.byFile[m.idFile[id]]].Nodes {
		if n.ID == id {
			return strings.HasSuffix(n.Label, "()")
		}
	}
	return false
}
