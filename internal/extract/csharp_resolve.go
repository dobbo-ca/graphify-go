package extract

import (
	"path"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// csIndex binds the receiver codes csCalls writes: a C# member call on its
// receiver's type, a `new X()` on type X. It is read by every bind run, so
// nothing in it changes once made.
type csIndex struct {
	ns              map[string]string   // top-level type id -> namespace
	byName          map[string][]string // type name -> type ids, by file, once per upstream node
	byNS            map[string]string   // name\x00namespace -> first such id of byName
	byKey           map[string][]string // csKey of type name -> type ids
	mkey            map[string][]string // type id\x00csKey of method name -> method ids
	fields          map[string][]string // type id\x00field -> type id\x00field type, as selfTarget reads
	usings          map[string][]string // type id -> the namespaces its usings bring in
	alias           map[string][]string // type id\x00alias -> the names its usings alias
	aliased         map[string]bool     // file\x00name a using of the file aliases
	aliasIn         map[string]bool     // file\x00alias a using of the file declares, and file
	nested          map[string]bool     // type id\x00name of a type nested in it
	nestedIn        map[string][]string // name -> files with a nested type of it
	shadow          map[string]bool     // generic type ids an in-file stub hides
	stubs           map[[2]string]bool  // file, a name its type refs make a stub of
	named           map[string]bool     // source\x00name of a C# type ref
	refs            map[string]bool     // source\x00target of a C# type ref edge
	owner           map[string]string   // method -> its type
	local           map[[2]string][]string
	idFile          map[string]string
	methods, supers map[string][]string
	openSuper       map[string]bool
	on              bool                  // upstream's C# resolvers need a .cs file
	twins           map[string][]*csTwin  // type id -> its same-file generic twins
	upper           map[string]bool       // types of .CS files, upstream's index skips them
	mname           map[string]string     // method id -> its name
	foreign         func() map[string]int // csKey -> other languages' types upstream counts
	test            map[string]bool       // test files, by upstream's rule
}

// csTwin is one of the generic twins upstream keeps apart: its own
// methods and base names, kept for the one a type name stands for.
type csTwin struct {
	kept           bool
	methods, bases []string
}

func newCSIndex(results []Result, inherit []model.Edge, owner map[string]string, local map[[2]string][]string, idFile map[string]string, methods, supers map[string][]string, openSuper map[string]bool) *csIndex {
	x := &csIndex{ns: map[string]string{}, byName: map[string][]string{}, byNS: map[string]string{}, byKey: map[string][]string{}, mkey: map[string][]string{}, fields: map[string][]string{},
		usings: map[string][]string{}, alias: map[string][]string{}, aliased: map[string]bool{}, aliasIn: map[string]bool{},
		nested: map[string]bool{}, nestedIn: map[string][]string{}, shadow: map[string]bool{}, stubs: map[[2]string]bool{},
		named: map[string]bool{}, refs: map[string]bool{}, twins: map[string][]*csTwin{}, upper: map[string]bool{},
		mname: map[string]string{}, test: map[string]bool{},
		owner: owner, local: local, idFile: idFile, methods: methods, supers: supers, openSuper: openSuper}
	// One upstream node per namespace, arity.
	decls, seen := map[string]int{}, map[string]bool{}
	for _, r := range results {
		x.on = x.on || path.Ext(r.File) == ".cs"
		if !strings.EqualFold(path.Ext(r.File), ".cs") {
			continue
		}
		// Upstream skips .CS in index, usings, bases.
		upper := path.Ext(r.File) != ".cs"
		x.test[r.File] = csTestPath(r.File)
		for _, e := range r.CsTypes {
			parts := strings.Split(e, " ")
			id := parts[0]
			x.ns[id] = parts[1]
			x.upper[id] = upper
			arity := ""
			var tw *csTwin
			for _, f := range parts[2:] {
				if f == "" {
					continue
				}
				switch f[0] {
				case '+':
					if !upper {
						x.usings[id] = append(x.usings[id], f[1:])
					}
					continue
				case '&':
					if upper {
						continue
					}
					a, t, _ := strings.Cut(f[1:], "=")
					x.alias[id+"\x00"+a] = append(x.alias[id+"\x00"+a], t)
					x.aliased[r.File+"\x00"+t] = true
					x.aliasIn[r.File+"\x00"+a], x.aliasIn[r.File] = true, true
					continue
				case '^':
					x.nested[id+"\x00"+f[1:]] = true
					if !contains(x.nestedIn[f[1:]], r.File) {
						// One upstream node per file.
						x.nestedIn[f[1:]] = append(x.nestedIn[f[1:]], r.File)
					}
					continue
				case '`':
					arity = f
					continue
				case '~':
					x.shadow[id] = true
					continue
				case '*':
					tw = &csTwin{kept: f == "*!"}
					x.twins[id] = append(x.twins[id], tw)
					continue
				case '.':
					tw.methods = append(tw.methods, csKey(f[1:]))
					continue
				case ':':
					tw.bases = append(tw.bases, f[1:])
					continue
				}
				name, typ, _ := strings.Cut(f, "=")
				val := id + "\x00" + typ
				if base, ok := strings.CutSuffix(name, "[]"); ok {
					// Upstream skips list fields here.
					name, val = base, id+"\x00[]"
				}
				if k := id + "\x00" + name; !contains(x.fields[k], val) {
					x.fields[k] = append(x.fields[k], val)
				}
			}
			if k := id + " " + parts[1] + arity; !seen[k] {
				seen[k] = true
				decls[id]++
			}
		}
		for _, d := range r.Defs {
			if o := owner[d.ID]; o != "" {
				x.mname[d.ID] = csKey(d.Name)
				if k := o + "\x00" + csKey(d.Name); !contains(x.mkey[k], d.ID) {
					x.mkey[k] = append(x.mkey[k], d.ID)
				}
			}
			if n := decls[d.ID]; n > 0 && !contains(x.byName[d.Name], d.ID) {
				for range n {
					x.byName[d.Name] = append(x.byName[d.Name], d.ID)
					x.byKey[csKey(d.Name)] = append(x.byKey[csKey(d.Name)], d.ID)
				}
			}
		}
		for _, t := range r.TypeRefs {
			if !t.Arg {
				x.stubs[[2]string{r.File, t.Name}] = true
			}
			x.named[t.FromID+"\x00"+t.Name] = true
		}
	}
	// Upstream's unique-name fallback counts every language's types.
	// Built on first use: it walks every non-C# file.
	x.foreign = sync.OnceValue(func() map[string]int {
		foreign := map[string]int{}
		if !x.on {
			return foreign
		}
		// Files scan in parallel, merged in file order.
		hits := make([][][2]string, len(results))
		var wg sync.WaitGroup
		for w, n := 0, runtime.GOMAXPROCS(0); w < n; w++ {
			wg.Go(func() {
				for i := w; i < len(results); i += n {
					r := &results[i]
					if strings.EqualFold(path.Ext(r.File), ".cs") {
						continue
					}
					var contained map[string]bool
					for _, nd := range r.Nodes {
						l := strings.TrimSpace(nd.Label)
						if nd.SourceFile == "" || nd.FileType != "code" || l == "" || strings.HasSuffix(l, ")") || strings.Contains(l, ".") {
							continue
						}
						if contained == nil {
							contained = map[string]bool{}
							for _, e := range r.Edges {
								if e.Relation == model.RelContains {
									contained[e.Target] = true
								}
							}
						}
						if contained[nd.ID] {
							hits[i] = append(hits[i], [2]string{nd.ID, csKey(l)})
						}
					}
				}
			})
		}
		wg.Wait()
		counted := map[string]bool{}
		for _, h := range hits {
			for _, p := range h {
				if !counted[p[0]] {
					counted[p[0]] = true
					foreign[p[1]]++
				}
			}
		}
		return foreign
	})
	for name, ids := range x.byName {
		slices.SortStableFunc(ids, func(a, b string) int { return strings.Compare(idFile[a], idFile[b]) })
		for _, id := range ids {
			if k := name + "\x00" + x.ns[id]; !x.upper[id] && x.byNS[k] == "" {
				x.byNS[k] = id
			}
		}
	}
	for _, e := range inherit {
		if strings.EqualFold(path.Ext(e.SourceFile.String()), ".cs") {
			x.refs[e.Source+"\x00"+e.Target] = true
		}
	}
	return x
}

// resolve binds a type name seen from type from the way upstream's namespace
// resolver does: an alias of the name in from's file decides alone, by the
// ones in from's scope, else from's namespace,
// the global one and its usings (one hit, or none when several), then the
// parent namespaces, then a name unique in the corpus: "^" when that is a
// nested type, which has no node.
// ponytail: no `using static`; aliases are seen only through types.
func (x *csIndex) resolve(name, from string, unique bool) string {
	in := func(name, ns string) string { return x.byNS[name+"\x00"+ns] }
	ns := x.ns[from]
	var keys [][2]string
	aliases, aliased := x.alias[from+"\x00"+name], x.aliasIn[x.idFile[from]+"\x00"+name]
	for _, a := range aliases {
		i := strings.LastIndex(a, ".")
		if i < 0 {
			// Upstream never resolves a dotless alias.
			continue
		}
		keys = append(keys, [2]string{a[i+1:], a[:max(i, 0)]})
	}
	if !aliased {
		for _, s := range append([]string{ns, ""}, x.usings[from]...) {
			keys = append(keys, [2]string{name, s})
		}
	}
	hit := ""
	for _, k := range keys {
		if id := in(k[0], k[1]); id != "" && id != hit {
			if hit != "" {
				return ""
			}
			hit = id
		}
	}
	if hit != "" || aliased {
		return hit
	}
	for n := ns; strings.Contains(n, "."); {
		n = n[:strings.LastIndex(n, ".")]
		if id := in(name, n); id != "" {
			return id
		}
	}
	name = csKey(name)
	ids := x.byKey[name]
	switch {
	case !unique || len(ids)+len(x.nestedIn[name])+x.foreign()[name] != 1:
		return ""
	case len(ids) == 0:
		return "^"
	}
	return ids[0]
}

// csKey is upstream's bare-name key: no non-word runes.
func csKey(s string) string {
	plain := true
	for i := 0; i < len(s) && plain; i++ {
		c := s[i]
		plain = c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
	}
	if plain {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return -1
	}, s)
}

// csMemo caches the new X() work of one bind run.
type csMemo struct {
	picks map[[3]string]csPick // file, name, qualifier -> its pick
	cands map[string]*csCands  // name -> its candidates, indexed
}

// csPick is a cached choice among the types of one name.
type csPick struct {
	tgt  string
	done bool
}

// bind is the target of a C# call with a receiver code made in file.
func (x *csIndex) bind(m *csMemo, file, caller, callee, recv string) (string, model.Confidence) {
	enc := x.owner[caller]
	t, conf := "", model.ConfInferred
	var tw *csTwin
	switch {
	case recv == "new" || strings.HasPrefix(recv, "new "):
		t, conf = x.bindNew(m, file, caller, callee, strings.TrimPrefix(strings.TrimPrefix(recv, "new"), " "), enc)
		if t == enc {
			// Upstream's contains edge wins.
			return "", conf
		}
		return t, conf
	case !x.on:
		// Only member calls need a .cs file.
	case recv == "this":
		t, conf, tw = enc, model.ConfExtracted, x.twinOf(enc, caller)
	case recv == "base":
		if bs, ok := x.bases(enc, x.twinOf(enc, caller)); ok && len(bs) == 1 {
			t, conf = bs[0], model.ConfExtracted
		}
	case recv[0] == '!':
		u, typ, ok := strings.Cut(recv[1:], " ")
		if !ok {
			u, typ = "", u
		}
		if t = x.resolve(u, enc, true); t == "" {
			t = x.resolve(typ, enc, true)
		}
		conf = model.ConfExtracted
	case recv[0] == '?':
		u, f, chain := strings.Cut(recv[1:], " ")
		if t = x.resolve(u, enc, true); t != "" && t != "^" && chain {
			o, typ, _ := strings.Cut(x.lookup(t, nil, f, x.fields), "\x00")
			t = x.resolve(typ, o, true)
		}
	}
	if t == "" || t == "^" {
		return "", conf
	}
	return x.lookup(t, tw, csKey(callee), x.mkey), conf
}

// lookup is upstream's search for key on type t and its bases: a branch
// stops where it is declared, one distinct hit wins, and an open base the
// walk reaches spoils it. A twinned type is its twin d, else the kept one.
func (x *csIndex) lookup(t string, d *csTwin, key string, table map[string][]string) string {
	type at struct {
		id string
		d  *csTwin
	}
	var hits []string
	seen := map[at]bool{}
	for q := []at{{t, d}}; len(q) > 0; {
		c := q[len(q)-1]
		q = q[:len(q)-1]
		if c.d == nil {
			c.d = x.twinOf(c.id, "")
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		if ids := table[c.id+"\x00"+key]; len(ids) > 0 && (c.d == nil || contains(c.d.methods, key)) {
			for _, id := range ids {
				if !contains(hits, id) {
					hits = append(hits, id)
				}
			}
			continue
		}
		bs, ok := x.bases(c.id, c.d)
		if !ok {
			return ""
		}
		for _, b := range bs {
			q = append(q, at{b, nil})
		}
	}
	if len(hits) != 1 {
		return ""
	}
	return hits[0]
}

// twinOf is the twin of type t that declares method caller, else the
// kept one, nil for a type with no twins.
func (x *csIndex) twinOf(t, caller string) *csTwin {
	var kept, own *csTwin
	name := x.mname[caller]
	for _, tw := range x.twins[t] {
		if tw.kept {
			kept = tw
		}
		if own == nil && contains(tw.methods, name) {
			own = tw
		}
	}
	if own == nil || kept != nil && contains(kept.methods, name) {
		return kept
	}
	return own
}

// bases are the base ids of type t, as its twin d when it has twins; ok
// is false when one is outside the corpus.
func (x *csIndex) bases(t string, d *csTwin) (ids []string, ok bool) {
	if d == nil {
		d = x.twinOf(t, "")
	}
	switch {
	case x.upper[t]:
		return nil, true
	case d == nil:
		return x.supers[t], !x.openSuper[t]
	}
	for _, b := range d.bases {
		i := slices.IndexFunc(x.supers[t], func(s string) bool { return contains(x.byName[b], s) })
		switch {
		case contains(x.byName[b], t):
			// A twin's base binds the kept twin.
			ids = append(ids, t)
		case i < 0:
			return nil, false
		default:
			ids = append(ids, x.supers[t][i])
		}
	}
	return ids, true
}

// bindNew binds `new X()`, qualified by prefix, to the type X.
func (x *csIndex) bindNew(m *csMemo, file, caller, name, prefix, enc string) (string, model.Confidence) {
	own := x.local[[2]string{file, name}]
	types := keep(own, func(id string) bool { _, ok := x.ns[id]; return ok })
	lexical := x.ownMethod(enc, name, file)
	shadow := len(types) > 0 && x.shadow[types[0]]
	// Methods or a stub defer upstream.
	if len(types) > 0 && x.owner[own[len(own)-1]] == "" && !lexical && !shadow {
		return types[0], model.ConfExtracted
	}
	cands := x.byName[name]
	inNS := func(id string) bool { return x.ns[id] == prefix }
	if prefix != "" && slices.ContainsFunc(x.byName[prefix[strings.LastIndex(prefix, ".")+1:]], func(id string) bool { return x.nested[id+"\x00"+name] }) {
		// Nested: no node; namespace pass binds.
		return unique(cands, inNS), model.ConfExtracted
	}
	// Upstream's import evidence: a file alias.
	evidence := func(id string) bool { return x.aliased[file+"\x00"+x.ns[id]+"."+name] }
	via := prefix == "" && !lexical && (len(types) == 0 || shadow) && x.stubs[[2]string{file, name}]
	tgt := ""
	if via {
		// Stub of X: scope or evidence.
		if tgt = x.resolve(name, enc, false); !contains(cands, tgt) {
			tgt = ""
		}
	}
	scoped := tgt != ""
	if !scoped {
		k := [3]string{file, name, prefix}
		p, ok := m.picks[k]
		if !ok {
			var ev []string
			if x.aliasIn[file] {
				ev = keep(cands, evidence)
			}
			all := m.cands[name]
			if all == nil {
				all = x.candsOf(name)
				m.cands[name] = all
			}
			switch {
			case all.n == 1:
				p.tgt = all.id
			case len(ev) == 1:
				p.tgt = ev[0]
			case prefix == "":
				p.tgt = all.tieBreak(file, x.test[file])
			default:
				p.tgt = unique(cands, inNS)
				p.done = p.tgt != ""
			}
			m.picks[k] = p
		}
		if tgt = p.tgt; p.done {
			return tgt, model.ConfExtracted
		}
	}
	if strings.HasPrefix(tgt, "\x00") {
		return "", model.ConfInferred
	}
	conf := model.ConfInferred
	if tgt != "" && evidence(tgt) {
		conf = model.ConfExtracted
	}
	if via && !scoped && conf != model.ConfExtracted || x.refs[caller+"\x00"+tgt] || via && x.named[caller+"\x00"+name] {
		// Unscoped stub, or references hold pair.
		return "", model.ConfInferred
	}
	return tgt, conf
}

// ownMethod reports whether the type enc or a base of it in file declares
// a method name, as upstream's lexical lookup finds first.
func (x *csIndex) ownMethod(enc, name, file string) bool {
	seen := map[string]bool{}
	for q := []string{enc}; len(q) > 0; q = q[1:] {
		if seen[q[0]] || x.idFile[q[0]] != file {
			continue
		}
		seen[q[0]] = true
		if len(x.methods[q[0]+"\x00"+name]) > 0 {
			return true
		}
		q = append(q, x.supers[q[0]]...)
	}
	return false
}

var (
	csTestDirs = set("tests", "test", "spec", "specs", "__tests__")
	csTestName = regexp.MustCompile(`(?i)^test_|_test\..|\.test\..|\.spec\..|_spec\..|\.tests\.ps1$`)
	csTestCase = regexp.MustCompile(`Tests?\.java$|Tests\.cs$`)
)

// csTestPath is upstream's _is_test_path.
func csTestPath(p string) bool {
	segs := strings.Split(p, "/")
	for _, s := range segs {
		if csTestDirs[strings.ToLower(s)] {
			return true
		}
	}
	name := segs[len(segs)-1]
	return csTestName.MatchString(name) || csTestCase.MatchString(name)
}

// fileOf is the file of a candidate, nested ones "\x00file".
func (x *csIndex) fileOf(id string) string {
	if f, ok := strings.CutPrefix(id, "\x00"); ok {
		return f
	}
	return x.idFile[id]
}

// csCands are the candidates of one name, nested ones "\x00file", split
// by upstream's test rule and indexed by file, dir and ancestor dir.
type csCands struct {
	csGroup
	tests, rest *csGroup
}

// csGroup counts candidates, and keeps one, overall and per key.
type csGroup struct {
	csHit
	file, dir, under map[string]csHit
}

type csHit struct {
	n  int
	id string
}

func (h *csHit) add(id string) {
	if h.n++; h.n == 1 {
		h.id = id
	}
}

// only is the one candidate counted, else "".
func (h csHit) only() string {
	if h.n == 1 {
		return h.id
	}
	return ""
}

func (x *csIndex) candsOf(name string) *csCands {
	c := &csCands{}
	g := func() *csGroup {
		return &csGroup{file: map[string]csHit{}, dir: map[string]csHit{}, under: map[string]csHit{}}
	}
	c.tests, c.rest = g(), g()
	ids := slices.Clone(x.byName[name])
	// Nested types count, with no node.
	for _, f := range x.nestedIn[name] {
		ids = append(ids, "\x00"+f)
	}
	for _, id := range ids {
		c.add(id)
		f := x.fileOf(id)
		g := c.rest
		if x.test[f] {
			g = c.tests
		}
		g.add(id)
		inc := func(m map[string]csHit, k string) {
			h := m[k]
			h.add(id)
			m[k] = h
		}
		inc(g.file, f)
		inc(g.dir, path.Dir(f))
		for d := path.Dir(f); d != "." && d != "/"; d = path.Dir(d) {
			inc(g.under, d)
		}
	}
	return c
}

// tieBreak is upstream's pick among several candidates for a caller in
// file: test ones by the caller's kind, then the same file, the same
// dir, the longest shared dir.
func (c *csCands) tieBreak(file string, test bool) string {
	g := c.rest
	if test && c.tests.n > 0 {
		if id := c.tests.file[file].only(); id != "" {
			return id
		}
		g = c.tests
	}
	if g.n == 1 {
		return g.id
	}
	if h := g.file[file]; h.n > 0 {
		return h.only()
	}
	if h := g.dir[path.Dir(file)]; h.n > 0 {
		return h.only()
	}
	for d := path.Dir(file); d != "." && d != "/"; d = path.Dir(d) {
		if h := g.under[d]; h.n > 0 {
			return h.only()
		}
	}
	return ""
}
