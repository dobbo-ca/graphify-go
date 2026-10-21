package extract

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	tscs "github.com/dobbo-ca/gositter/grammars/csharp"
	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractCSharp pulls top-level functions, types (class/struct/interface/enum/
// record + their methods), `using` imports, and call edges out of a .cs file.
// Declarations nested inside (file-scoped) namespaces are descended into so
// namespaced source still surfaces its definitions.
func extractCSharp(rel string, src []byte) Result {
	root, done := parseRoot(src, tscs.Language())
	defer done()
	b := newBuilder(rel)

	ifaces := csInterfaceNames(root, src)
	b.csNested, b.csFields, b.csSupers = map[string]bool{}, map[string]map[string]string{}, map[string][]string{}
	b.csCollect(root, src, ifaces)
	// A nested namesake shares this node upstream.
	for key := range b.csFields {
		name, _, _ := strings.Cut(key, "`")
		delete(b.csNested, name)
	}
	b.csItems(root, src, ifaces, "", nil)
	// A later ref's stub hides it upstream.
	for _, g := range b.csGen {
		id, _, _ := strings.Cut(b.res.CsTypes[g.entry], " ")
		ref := func(t TypeRef) bool { return t.Name == g.name && t.Relation == "references" && !t.Arg }
		plain := slices.ContainsFunc(b.res.CsTypes, func(e string) bool { return strings.HasPrefix(e, id+" ") && !strings.Contains(e, " `") })
		if !plain && !slices.ContainsFunc(b.res.TypeRefs[:g.refs], ref) && slices.ContainsFunc(b.res.TypeRefs[g.refs:], ref) {
			b.res.CsTypes[g.entry] += " ~"
		}
	}
	b.csTwins()
	return b.res
}

// csTwins marks each of the types of one name and different arity in this
// file as a twin (*, *! for the one upstream keeps: first by "L<line>"
// text, then by id) with its own methods (.M) and bases (:B), and drops
// their fields: upstream binds no field table to a label twice in a file.
func (b *builder) csTwins() {
	byID := map[string][]csDecl{}
	var ids []string
	for _, d := range b.csDecls {
		id, _, _ := strings.Cut(b.res.CsTypes[d.entry], " ")
		if byID[id] == nil {
			ids = append(ids, id)
		}
		byID[id] = append(byID[id], d)
	}
	arity := func(d csDecl) string {
		_, a, _ := strings.Cut(b.res.CsTypes[d.entry], " `")
		a, _, _ = strings.Cut(a, " ")
		return a
	}
	for _, id := range ids {
		ds, seen := byID[id], map[string]bool{}
		for _, d := range ds {
			seen[arity(d)] = true
		}
		if len(ds) < 2 || len(seen) < len(ds) {
			// Partial halves merge upstream.
			continue
		}
		// Ids sort the bare name before name_1.
		kept := slices.MinFunc(ds, func(x, y csDecl) int {
			return cmp.Or(strings.Compare(x.loc, y.loc), strings.Compare(arity(x), arity(y)))
		})
		for _, d := range ds {
			codes := slices.DeleteFunc(strings.Split(b.res.CsTypes[d.entry], " "), func(f string) bool {
				return f != "" && f[0] != '&' && strings.Contains(f, "=")
			})
			codes = append(codes, "*")
			if d.entry == kept.entry {
				codes[len(codes)-1] = "*!"
			}
			for _, m := range d.methods {
				codes = append(codes, "."+m)
			}
			for _, s := range d.bases {
				codes = append(codes, ":"+strings.ReplaceAll(s, " ", "\t"))
			}
			b.res.CsTypes[d.entry] = strings.Join(codes, " ")
		}
	}
}

// csCollect records the nested type names and the field table and inherits
// bases of each top-level type, before any call is typed: a base may come
// later in the file.
func (b *builder) csCollect(n *ts.Node, src []byte, ifaces map[string]bool) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "class_declaration", "struct_declaration", "interface_declaration",
			"enum_declaration", "record_declaration":
			csNestedTypes(c, src, b.csNested)
			tps := csTypeParams(c, nil, src)
			name := csArityKey(fieldText(c, "name", src), len(tps))
			if b.csFields[name] == nil {
				b.csFields[name] = map[string]string{}
			}
			for j := uint(0); j < c.ChildCount(); j++ {
				switch l := c.Child(j); l.Kind() {
				case "parameter_list":
					// A primary constructor's parameters are fields.
					for k := uint(0); k < l.NamedChildCount() && c.Kind() != "interface_declaration"; k++ {
						p := l.NamedChild(k)
						if t, pn := b.csRecvType(p.ChildByFieldName("type"), src), fieldText(p, "name", src); p.Kind() == "parameter" && t != "" && !tps[t] && pn != "" {
							b.csFields[name][pn] = t
						}
					}
				}
			}
			for _, bn := range csBaseTypes(c) {
				base := csBaseName(bn, src)
				if !tps[base] && (c.Kind() == "interface_declaration" || !csIsInterfaceName(base, ifaces)) {
					an := bn
					if q := bn.ChildByFieldName("name"); bn.Kind() == "qualified_name" && q != nil {
						an = q
					}
					b.csSupers[name] = append(b.csSupers[name], csArityKey(base, len(csTypeArgList(an))))
				}
			}
			if body := c.ChildByFieldName("body"); body != nil {
				for j := uint(0); j < body.ChildCount(); j++ {
					b.csField(b.csFields[name], body.Child(j), tps, src)
				}
			}
		case "namespace_declaration":
			if body := c.ChildByFieldName("body"); body != nil {
				b.csCollect(body, src, ifaces)
			}
		case "file_scoped_namespace_declaration":
			b.csCollect(c, src, ifaces)
		}
	}
}

// csField adds a field or property declaration m to the table: name -> type,
// name[] -> element type, name{} (and .Keys{}, .Values{}) -> loop element.
func (b *builder) csField(table map[string]string, m *ts.Node, tps map[string]bool, src []byte) {
	var t *ts.Node
	var names []string
	switch m.Kind() {
	case "field_declaration":
		for j := uint(0); j < m.NamedChildCount(); j++ {
			v := m.NamedChild(j)
			if v.Kind() != "variable_declaration" {
				continue
			}
			t = v.ChildByFieldName("type")
			for k := uint(0); k < v.NamedChildCount(); k++ {
				if d := v.NamedChild(k); d.Kind() == "variable_declarator" {
					names = append(names, csDeclName(d, src))
				}
			}
		}
		// Properties keep type parameters upstream.
		if tail, _, _ := csReadTypeName(t, src); tail == "" || tps[tail] {
			return
		}
	case "property_declaration":
		t = m.ChildByFieldName("type")
		names = []string{fieldText(m, "name", src)}
	default:
		return
	}
	typ := b.csRecvType(t, src)
	if typ == "" {
		return
	}
	elem, loops := b.csElemType(t, tps, src), b.csLoopElems(t, tps, src)
	for _, name := range names {
		if name == "" {
			continue
		}
		table[name] = typ
		if elem != "" {
			table[name+"[]"] = elem
		}
		for suffix, e := range loops {
			table[name+suffix+"{}"] = e
		}
	}
}

// csDeclName is the name of a variable_declarator.
func csDeclName(d *ts.Node, src []byte) string {
	if name := d.ChildByFieldName("name"); name != nil {
		return name.Utf8Text(src)
	}
	for i := uint(0); i < d.NamedChildCount(); i++ {
		if c := d.NamedChild(i); c.Kind() == "identifier" {
			return c.Utf8Text(src)
		}
	}
	return ""
}

// csArityKey keys the tables apart for generic twins.
func csArityKey(name string, arity int) string {
	if arity == 0 {
		return name
	}
	return name + "`" + strconv.Itoa(arity)
}

// csFieldsUp merges the field tables of type name and its bases in this
// file, nearest first.
func (b *builder) csFieldsUp(name string) map[string]string {
	merged := map[string]string{}
	seen := map[string]bool{}
	for queue := []string{name}; len(queue) > 0; queue = queue[1:] {
		if seen[queue[0]] {
			continue
		}
		seen[queue[0]] = true
		for k, v := range b.csFields[queue[0]] {
			if _, ok := merged[k]; !ok {
				merged[k] = v
			}
		}
		queue = append(queue, b.csSupers[queue[0]]...)
	}
	return merged
}

// csItems handles each declaration directly under n (a compilation_unit, a
// namespace's declaration_list, or a file_scoped_namespace_declaration),
// recursing into nested namespaces. uses are the usings in scope.
func (b *builder) csItems(n *ts.Node, src []byte, ifaces map[string]bool, ns string, uses []string) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		// Top-level statements wrap their inner statement in a global_statement.
		if c.Kind() == "global_statement" && c.NamedChildCount() > 0 {
			c = c.NamedChild(0)
		}
		switch c.Kind() {
		case "class_declaration", "struct_declaration", "interface_declaration",
			"enum_declaration", "record_declaration":
			b.csType(c, src, ifaces, ns, uses)
		case "local_function_statement":
			b.csFunc(c, src)
		case "using_directive":
			b.csUsing(c, src)
			if u := csUseCode(c.Utf8Text(src)); u != "" {
				uses = append(slices.Clip(uses), u)
			}
		case "namespace_declaration":
			if body := c.ChildByFieldName("body"); body != nil {
				b.csItems(body, src, ifaces, csNamespace(ns, c, src), uses)
			}
		case "file_scoped_namespace_declaration":
			// Members follow as siblings, not children.
			ns = csNamespace(ns, c, src)
			b.csItems(c, src, ifaces, ns, uses)
		}
	}
}

// csNamespace is the namespace n declares inside ns.
func csNamespace(ns string, n *ts.Node, src []byte) string {
	// Spaces split CsTypes codes.
	name := strings.ReplaceAll(fieldText(n, "name", src), " ", "\t")
	if ns == "" {
		return name
	}
	return ns + "." + name
}

// csFunc records a top-level (local) function and its call sites.
func (b *builder) csFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := b.id(name)
	b.def(id, name, name+"()", line(n))
	b.csOwnNested = nil
	b.csCalls(n, id, nil, csTypeParams(n, nil, src), src)
}

// csType records a type definition and the methods declared in its body, each
// scoped under the type's name.
func (b *builder) csType(n *ts.Node, src []byte, ifaces map[string]bool, ns string, uses []string) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	typeID := b.id(name)
	b.def(typeID, name, name, line(n))
	tps := csTypeParams(n, nil, src)
	key := csArityKey(name, len(tps))
	fields := b.csFieldsUp(key)
	nested := csNestedTypes(n, src, map[string]bool{})
	skip := csTypeParams(n, nested, src)
	maps.DeleteFunc(nested, func(k string, _ bool) bool { return !b.csNested[k] })
	b.csOwnNested = nested
	entry := typeID + " " + ns
	var keys []string
	for k, v := range b.csFields[key] {
		if !strings.HasSuffix(k, "{}") && !nested[v] {
			keys = append(keys, k)
		}
	}
	for k := range nested {
		keys = append(keys, "^"+k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if k[0] == '^' {
			entry += " " + k
		} else {
			entry += " " + k + "=" + b.csFields[key][k]
		}
	}
	for _, u := range uses {
		entry += " " + u
	}
	if len(tps) > 0 {
		entry += " `" + strconv.Itoa(len(tps))
		b.csGen = append(b.csGen, csGen{len(b.res.CsTypes), len(b.res.TypeRefs), name})
	}
	b.res.CsTypes = append(b.res.CsTypes, entry)
	di := len(b.csDecls)
	b.csDecls = append(b.csDecls, csDecl{entry: len(b.res.CsTypes) - 1, loc: line(n).String()})
	for _, bn := range csBaseTypes(n) {
		b.csDecls[di].bases = append(b.csDecls[di].bases, csBaseName(bn, src))
	}
	b.csBases(typeID, n, src, ifaces, skip)
	// A record or class with a primary constructor takes its parameters as members.
	for i := uint(0); i < n.ChildCount(); i++ {
		if ps := n.Child(i); ps.Kind() == "parameter_list" {
			b.csParams(typeID, ps, skip, src)
		}
	}

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		switch m.Kind() {
		case "field_declaration":
			for j := uint(0); j < m.NamedChildCount(); j++ {
				if v := m.NamedChild(j); v.Kind() == "variable_declaration" {
					b.csTypeRefs(typeID, v.ChildByFieldName("type"), skip, src)
				}
			}
			continue
		case "property_declaration":
			b.csTypeRefs(typeID, m.ChildByFieldName("type"), skip, src)
			continue
		case "method_declaration":
		default:
			continue
		}
		mname := fieldText(m, "name", src)
		if mname == "" {
			continue
		}
		mid := b.id(name, mname)
		b.csDecls[di].methods = append(b.csDecls[di].methods, mname)
		b.addNode(mid, name+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: typeID, Target: mid, Relation: model.RelContains,
			Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(m),
		})
		// Register under the bare method name so `x.Method()` call sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.csSignature(mid, m, skip, src)
		b.csCalls(m, mid, fields, csTypeParams(m, tps, src), src)
	}
}

// csUseCode is the CsTypes code of a using directive: +A.B for a namespace,
// &X=A.B for an alias, "" for `using static`.
func csUseCode(text string) string {
	body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(text), ";"), "global "))
	body, ok := strings.CutPrefix(body, "using")
	if !ok || strings.HasPrefix(body, " static ") {
		return ""
	}
	code := "+" + strings.TrimSpace(body)
	if a, t, ok := strings.Cut(body, "="); ok {
		// Upstream strips only a trailing <...>.
		if t = strings.TrimSpace(t); strings.HasSuffix(t, ">") {
			for i, depth := len(t)-1, 0; i >= 0; i-- {
				if t[i] == '>' {
					depth++
				} else if t[i] == '<' {
					if depth--; depth == 0 {
						t = t[:i]
						break
					}
				}
			}
		}
		code = "&" + strings.TrimSpace(a) + "=" + strings.TrimSpace(t)
	}
	// Spaces split CsTypes codes.
	return strings.ReplaceAll(code, " ", "\t")
}

// csUsing records the namespace path of a `using` directive as an import. The
// imported namespace is the directive's `qualified_name`/`identifier` child;
// aliased usings (`using Foo = Bar;`) keep the aliased path.
func (b *builder) csUsing(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "qualified_name", "identifier":
			b.imp(c.Utf8Text(src), line(n))
			return
		}
	}
}

var csCallKinds = tscs.Language().KindMask("invocation_expression", "object_creation_expression")

// csCalls walks the body of the method or local function fn and records each
// call site. A bare call `f()` has no receiver. A member call records the
// receiver's type, as a code the resolver reads for .cs files alone:
//
//	this, base         the caller's own type, its base
//	!U T               T named in source (a type, a cast, a Pascal-case member), U its declared type
//	?U                 a local, parameter or field of type U, an element of a U list, a new U
//	?U F               field F of a U
//	new, new A.B       a construction of the callee type, qualified by A.B
//
// A member call whose receiver has no type is not recorded.
func (b *builder) csCalls(fn *ts.Node, callerID string, fields map[string]string, tps map[string]bool, src []byte) {
	body := fn.ChildByFieldName("body")
	if body == nil {
		return
	}
	skip := maps.Clone(tps)
	maps.Copy(skip, b.csOwnNested)
	targs := func(g *ts.Node) {
		n := len(b.res.TypeRefs)
		b.csTypeArgs(callerID, g, skip, src)
		for i := n; i < len(b.res.TypeRefs); i++ {
			b.res.TypeRefs[i].Arg = true
		}
	}
	var scope *csScope
	typeAt := func(name string, at uint) string {
		if scope == nil {
			scope = b.csScopeOf(fn, fields, tps, src)
		}
		return scope.typeAt(name, at)
	}
	walkKinds(body, csCallKinds, func(cv ts.Node) bool {
		c := &cv
		if c.Kind() == "object_creation_expression" {
			tail, _, prefix := csReadTypeName(c.ChildByFieldName("type"), src)
			if tail != "" && !b.csNested[tail] && !csBuiltins[tail] {
				recv := "new"
				if prefix != "" {
					recv += " " + prefix
				}
				b.callRecv(callerID, tail, recv, line(c))
			}
			return true
		}
		fn := c.ChildByFieldName("function")
		if fn == nil {
			return true
		}
		var name, recv *ts.Node
		switch fn.Kind() {
		case "identifier":
			b.call(callerID, fn.Utf8Text(src), line(c))
		case "generic_name":
			targs(fn)
		case "member_access_expression":
			name, recv = fn.ChildByFieldName("name"), fn.ChildByFieldName("expression")
		case "conditional_access_expression":
			for i := uint(0); i < fn.NamedChildCount(); i++ {
				if mb := fn.NamedChild(i); mb.Kind() == "member_binding_expression" {
					name, recv = mb.ChildByFieldName("name"), fn.ChildByFieldName("condition")
					break
				}
			}
		}
		if name == nil || recv == nil {
			return true
		}
		callee := name.Utf8Text(src)
		if name.Kind() == "generic_name" {
			targs(name)
			for i := uint(0); i < name.NamedChildCount(); i++ {
				if id := name.NamedChild(i); id.Kind() == "identifier" {
					callee = id.Utf8Text(src)
					break
				}
			}
		}
		if code := b.csRecv(recv, c.StartByte(), typeAt, src); code != "" {
			b.callRecv(callerID, callee, code, line(c))
		}
		return true
	})
}

// csRecv is the receiver code of a member call made at byte at.
func (b *builder) csRecv(recv *ts.Node, at uint, typeAt func(string, uint) string, src []byte) string {
	switch recv.Kind() {
	case "identifier":
		return b.csNamed(recv.Utf8Text(src), at, typeAt)
	case "this", "this_expression":
		return "this"
	case "base", "base_expression":
		return "base"
	case "member_access_expression":
		inner, f := recv.ChildByFieldName("expression"), recv.ChildByFieldName("name")
		if inner == nil || f == nil || f.Kind() != "identifier" {
			return ""
		}
		switch inner.Kind() {
		case "this", "this_expression":
			return b.csNamed(f.Utf8Text(src), at, typeAt)
		case "identifier":
			if t := typeAt(inner.Utf8Text(src), at); t != "" {
				return "?" + t + " " + f.Utf8Text(src)
			}
		}
	case "element_access_expression":
		if inner := recv.ChildByFieldName("expression"); inner != nil && inner.Kind() == "identifier" {
			if t := typeAt(inner.Utf8Text(src)+"[]", at); t != "" {
				return "?" + t
			}
		}
	case "object_creation_expression":
		// Upstream skips the Pascal-case check here.
		if t := csTypeTail(recv.ChildByFieldName("type"), src); t != "" {
			return "?" + t
		}
	case "parenthesized_expression":
		if recv.NamedChildCount() == 0 {
			return ""
		}
		var t *ts.Node
		switch in := recv.NamedChild(0); in.Kind() {
		case "cast_expression":
			t = in.ChildByFieldName("type")
		case "as_expression":
			t = in.ChildByFieldName("right")
		}
		tail, _, _ := csReadTypeName(t, src)
		return b.csNamed(tail, at, typeAt)
	}
	return ""
}

// csNamed is the receiver code of a name: a Pascal-case one names a type in
// source unless its declared type says otherwise, a lowercase one needs a
// declared type.
func (b *builder) csNamed(name string, at uint, typeAt func(string, uint) string) string {
	if name == "" {
		return ""
	}
	u := typeAt(name, at)
	r, _ := utf8.DecodeRuneInString(name)
	if !unicode.IsUpper(r) {
		if u == "" {
			return ""
		}
		return "?" + u
	}
	t := name
	switch {
	case u == "" || u == t:
		return "!" + t
	}
	return "!" + u + " " + t
}

// csGen is a generic type: its CsTypes entry, the TypeRefs made before it.
type csGen struct {
	entry, refs int
	name        string
}

// csDecl is a type declaration: its CsTypes entry and line, its own
// methods and bases.
type csDecl struct {
	entry          int
	loc            string
	methods, bases []string
}

// csBind is a name declared over the byte range [lo, hi), with its type ("" for none).
type csBind struct {
	lo, hi uint
	typ    string
}

// csScope types the receivers of one method: its declarations by range, and
// the field table of its type for a name none of them covers or poisons.
type csScope struct {
	binds  map[string][]csBind // sorted by lo
	fields map[string]string
	poison map[string]bool
}

// typeAt is the type of name at byte at: the innermost declaration covering
// it, else the field. A tie or an untyped declaration gives "".
func (s *csScope) typeAt(name string, at uint) string {
	bs := s.binds[name]
	j, _ := slices.BinarySearchFunc(bs, at+1, func(d csBind, t uint) int { return cmp.Compare(d.lo, t) })
	for j--; j >= 0 && bs[j].hi <= at; j-- {
	}
	if j < 0 {
		if s.poison[name] {
			return ""
		}
		return s.fields[name]
	}
	// Scopes nest: an earlier start covering at is wider.
	typ, size, n := "", uint(0), 0
	for k := j; k >= 0 && bs[k].lo == bs[j].lo; k-- {
		switch d := bs[k]; {
		case d.hi <= at:
		case n == 0 || d.hi-d.lo < size:
			typ, size, n = d.typ, d.hi-d.lo, 1
		case d.hi-d.lo == size:
			n++
		}
	}
	if n > 1 {
		return ""
	}
	return typ
}

// csScopeOf collects the declarations of fn. A declaration whose type
// differs from a same-named field hides that field in all of fn.
func (b *builder) csScopeOf(fn *ts.Node, fields map[string]string, tps map[string]bool, src []byte) *csScope {
	poison := map[string]bool{}
	s := &csScope{binds: map[string][]csBind{}, fields: fields, poison: poison}
	bind := func(name, typ string, scope *ts.Node, elem string) {
		if name == "" {
			return
		}
		for _, kv := range [2][2]string{{name, typ}, {name + "[]", elem}} {
			if f, ok := fields[kv[0]]; ok && f != kv[1] {
				poison[kv[0]] = true
			}
			s.binds[kv[0]] = append(s.binds[kv[0]], csBind{scope.StartByte(), scope.EndByte(), kv[1]})
		}
	}
	bindParams := func(ps, scope *ts.Node) {
		for i := uint(0); ps != nil && i < ps.ChildCount(); i++ {
			switch p := ps.Child(i); p.Kind() {
			case "parameter":
				t := p.ChildByFieldName("type")
				bind(fieldText(p, "name", src), b.csRecvType(t, src), scope, b.csElemType(t, tps, src))
			case "implicit_parameter":
				bind(p.Utf8Text(src), "", scope, "")
			}
		}
	}
	type loop struct {
		name, field, suffix string
		n                   *ts.Node
	}
	var loops []loop
	body := fn.ChildByFieldName("body")
	bindParams(fn.ChildByFieldName("parameters"), body)
	var visit func(n, scope *ts.Node)
	visit = func(n, scope *ts.Node) {
		switch n.Kind() {
		case "class_declaration", "struct_declaration", "interface_declaration",
			"enum_declaration", "record_declaration":
			return
		case "lambda_expression":
			if ps := n.ChildByFieldName("parameters"); ps != nil && ps.Kind() == "implicit_parameter" {
				bind(ps.Utf8Text(src), "", n, "")
			} else {
				bindParams(ps, n)
			}
		case "local_function_statement":
			bindParams(n.ChildByFieldName("parameters"), n)
		case "local_declaration_statement":
			for i := uint(0); i < n.NamedChildCount(); i++ {
				vd := n.NamedChild(i)
				if vd.Kind() != "variable_declaration" {
					continue
				}
				t := vd.ChildByFieldName("type")
				declared, elem := b.csRecvType(t, src), b.csElemType(t, tps, src)
				for j := uint(0); j < vd.NamedChildCount(); j++ {
					d := vd.NamedChild(j)
					if d.Kind() != "variable_declarator" {
						continue
					}
					typ := declared
					// ponytail: var v = M() untyped (byx.46)
					for k := uint(0); typ == "" && k < d.ChildCount(); k++ {
						if g := d.Child(k); g.Kind() == "object_creation_expression" {
							typ = b.csRecvType(g.ChildByFieldName("type"), src)
						} else if g.Kind() == "invocation_expression" {
							break
						}
					}
					bind(csDeclName(d, src), typ, scope, elem)
				}
				break
			}
		case "foreach_statement":
			if left := n.ChildByFieldName("left"); left != nil && left.Kind() == "identifier" {
				typ := b.csRecvType(n.ChildByFieldName("type"), src)
				field, suffix, ok := "", "", false
				if typ == "" {
					field, suffix, ok = csLoopSource(n.ChildByFieldName("right"), src)
				}
				if ok {
					loops = append(loops, loop{left.Utf8Text(src), field, suffix, n})
				} else {
					bind(left.Utf8Text(src), typ, n, "")
				}
			}
		case "declaration_expression", "declaration_pattern":
			if name := n.ChildByFieldName("name"); name != nil && name.Kind() == "identifier" {
				bind(name.Utf8Text(src), b.csRecvType(n.ChildByFieldName("type"), src), scope, "")
			}
		}
		switch n.Kind() {
		case "block", "lambda_expression", "local_function_statement":
			scope = n
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			visit(n.Child(i), scope)
		}
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		visit(body.Child(i), body)
	}
	// Hiding declarations are all known now.
	for _, l := range loops {
		typ := fields[l.field+l.suffix+"{}"]
		for _, d := range s.binds[l.field] {
			if d.lo <= l.n.StartByte() && l.n.StartByte() < d.hi {
				typ = ""
			}
		}
		bind(l.name, typ, l.n, "")
	}
	for k := range poison {
		delete(s.binds, k)
	}
	for _, bs := range s.binds {
		slices.SortStableFunc(bs, func(a, b csBind) int { return cmp.Compare(a.lo, b.lo) })
	}
	return s
}

// csLoopSource is the field and suffix ("", ".Keys", ".Values") of a foreach
// over f, this.f, f.Keys or f.Values.
func csLoopSource(e *ts.Node, src []byte) (field, suffix string, ok bool) {
	if e == nil {
		return "", "", false
	}
	if e.Kind() == "identifier" {
		return e.Utf8Text(src), "", true
	}
	inner, name := e.ChildByFieldName("expression"), e.ChildByFieldName("name")
	if e.Kind() != "member_access_expression" || inner == nil || name == nil || name.Kind() != "identifier" {
		return "", "", false
	}
	member := name.Utf8Text(src)
	if k := inner.Kind(); k == "this" || k == "this_expression" {
		return member, "", true
	}
	if member == "Values" || member == "Keys" {
		if f, sfx, ok := csLoopSource(inner, src); ok && sfx == "" {
			return f, "." + member, true
		}
	}
	return "", "", false
}

// csReadTypeName is the last name of a type, generic arguments cut, and the
// qualifier it was written with.
func csReadTypeName(n *ts.Node, src []byte) (tail string, qualified bool, prefix string) {
	tail, qualified, prefix, _ = csTypeName(n, src)
	// Spaces split CsTypes and receiver codes.
	return strings.ReplaceAll(tail, " ", "\t"), qualified, strings.ReplaceAll(prefix, " ", "\t")
}

// csBaseName is a base_list entry's name, read as upstream does.
func csBaseName(n *ts.Node, src []byte) string {
	tail, _, _, _ := csTypeName(n, src)
	return tail
}

func csTypeName(n *ts.Node, src []byte) (tail string, qualified bool, prefix string, ok bool) {
	if n == nil {
		return "", false, "", false
	}
	switch n.Kind() {
	case "identifier", "predefined_type":
		return n.Utf8Text(src), false, "", true
	case "qualified_name":
		text := n.Utf8Text(src)
		i := strings.LastIndex(text, ".")
		tail, _, _ = strings.Cut(text[i+1:], "<")
		return tail, true, text[:max(i, 0)], true
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if tail, qualified, prefix, ok = csTypeName(n.NamedChild(i), src); ok {
			return
		}
	}
	return "", false, "", false
}

// csRecvType is the type t names when a receiver of it can own a method:
// Pascal case.
func (b *builder) csRecvType(t *ts.Node, src []byte) string {
	tail, _, _ := csReadTypeName(t, src)
	if r, _ := utf8.DecodeRuneInString(tail); !unicode.IsUpper(r) {
		return ""
	}
	return tail
}

var csLists = set("List", "IList", "IReadOnlyList")

// csElemType is the element type of an array or list type t.
func (b *builder) csElemType(t *ts.Node, tps map[string]bool, src []byte) string {
	if t == nil {
		return ""
	}
	var elem *ts.Node
	switch t.Kind() {
	case "array_type":
		elem = t.ChildByFieldName("type")
	case "generic_name":
		if args := csTypeArgList(t); csLists[csTypeTail(t, src)] && len(args) == 1 {
			elem = args[0]
		}
	}
	if elem == nil || !csNameKinds[elem.Kind()] {
		return ""
	}
	if name := b.csRecvType(elem, src); !csLists[name] && !tps[name] {
		return name
	}
	return ""
}

var (
	csNameKinds = set("identifier", "qualified_name", "generic_name")
	csSeqs      = set("List", "IList", "IReadOnlyList", "IEnumerable", "ICollection",
		"IReadOnlyCollection", "HashSet", "ISet", "SortedSet", "Queue", "Stack", "LinkedList")
	csDicts = set("Dictionary", "IDictionary", "IReadOnlyDictionary", "SortedDictionary",
		"SortedList", "ConcurrentDictionary")
)

// csLoopElems maps what a foreach over a field of type t iterates ("" for
// the field, ".Keys", ".Values" for a dictionary's) to the element type.
func (b *builder) csLoopElems(t *ts.Node, tps map[string]bool, src []byte) map[string]string {
	var args []*ts.Node
	var suffixes []string
	switch {
	case t == nil:
	case t.Kind() == "array_type":
		args, suffixes = []*ts.Node{t.ChildByFieldName("type")}, []string{""}
	case t.Kind() == "generic_name":
		args = csTypeArgList(t)
		if tail := csTypeTail(t, src); csSeqs[tail] && len(args) == 1 {
			suffixes = []string{""}
		} else if csDicts[tail] && len(args) == 2 {
			suffixes = []string{".Keys", ".Values"}
		}
	}
	out := map[string]string{}
	for i, sfx := range suffixes {
		if a := args[i]; a != nil && csNameKinds[a.Kind()] {
			if name := b.csRecvType(a, src); name != "" && !tps[name] && !csSeqs[name] && !csDicts[name] {
				out[sfx] = name
			}
		}
	}
	return out
}

func csTypeTail(t *ts.Node, src []byte) string {
	tail, _, _ := csReadTypeName(t, src)
	return tail
}

// csTypeArgList is the type arguments of the generic_name t.
func csTypeArgList(t *ts.Node) []*ts.Node {
	var out []*ts.Node
	for i := uint(0); i < t.NamedChildCount(); i++ {
		if args := t.NamedChild(i); args.Kind() == "type_argument_list" {
			for j := uint(0); j < args.NamedChildCount(); j++ {
				out = append(out, args.NamedChild(j))
			}
		}
	}
	return out
}

// csBuiltins are upstream's builtin globals: `new X()` of one is no call.
var csBuiltins = set(
	"String", "Number", "Boolean", "Object", "Array", "Symbol", "BigInt",
	"Date", "RegExp", "Error", "TypeError", "RangeError", "SyntaxError",
	"ReferenceError", "EvalError", "URIError",
	"Promise", "Map", "Set", "WeakMap", "WeakSet", "JSON", "Math",
	"Reflect", "Proxy", "Intl",
	"parseInt", "parseFloat", "isNaN", "isFinite",
	"encodeURIComponent", "decodeURIComponent", "encodeURI", "decodeURI",
	"URL", "URLSearchParams", "FormData", "Blob", "File",
	"Headers", "Request", "Response", "AbortController", "AbortSignal",
	"TextEncoder", "TextDecoder", "console",
	"str", "int", "float", "bool", "list", "dict", "set", "tuple", "bytes",
	"len", "range", "enumerate", "zip", "map", "filter", "sum", "min", "max",
	"print", "open", "isinstance", "type", "super", "sorted", "reversed",
	"any", "all", "abs", "round", "next", "iter", "hash", "id", "repr",
	"callable", "getattr", "setattr", "hasattr", "delattr", "vars", "dir",
	"Int", "Int8", "Int16", "Int32", "Int64",
	"UInt", "UInt8", "UInt16", "UInt32", "UInt64",
	"Double", "Float", "Bool", "Character",
	"Sendable", "Codable", "Decodable", "Encodable", "Equatable", "Hashable",
	"Identifiable", "Comparable", "CaseIterable", "RawRepresentable",
	"CustomStringConvertible", "CustomDebugStringConvertible", "AnyObject",
	"LocalizedError",
	"Data", "UUID", "Decimal", "Calendar", "Locale", "TimeZone", "Bundle",
	"IndexPath", "IndexSet", "NotificationCenter", "UserDefaults",
	"FileManager", "URLSession", "URLRequest", "URLComponents",
	"JSONDecoder", "JSONEncoder", "DateFormatter", "NumberFormatter",
	"ISO8601DateFormatter",
	"NSObject", "NSString", "NSError", "NSLock", "NSAttributedString",
	"DispatchQueue", "DispatchGroup", "OperationQueue", "RunLoop",
	"View", "Color", "Font",
)

var csInterfaceKinds = tscs.Language().KindMask("interface_declaration")

// csInterfaceNames collects the interfaces declared anywhere in the file, so a
// base can be classified as implements rather than inherits.
func csInterfaceNames(root *ts.Node, src []byte) map[string]bool {
	names := map[string]bool{}
	walkKinds(root, csInterfaceKinds, func(nv ts.Node) bool {
		if name := fieldText(&nv, "name", src); name != "" {
			names[name] = true
		}
		return true
	})
	return names
}

// csBases records the entries of a type's base_list. C# does not distinguish
// a base class from an implemented interface syntactically, so a base is an
// implements target when it is an interface declared in this file or follows the
// IPascalCase convention, and an inherits target otherwise. Every base of an
// interface is interface inheritance.
func (b *builder) csBases(typeID string, n *ts.Node, src []byte, ifaces, skip map[string]bool) {
	isInterface := n.Kind() == "interface_declaration"
	for _, c := range csBaseTypes(n) {
		base := csBaseName(c, src)
		relation := "inherits"
		if !isInterface && csIsInterfaceName(base, ifaces) {
			relation = "implements"
		}
		switch c.Kind() {
		case "identifier", "generic_name", "qualified_name":
			if skip[base] {
				continue
			}
			b.ref(typeID, base, relation, false, line(n))
			if c.Kind() == "generic_name" {
				b.csTypeArgs(typeID, c, skip, src)
			}
		default:
			b.typeRef(typeID, base, relation, line(n))
		}
	}
}

// csBaseTypes are the base_list entries of n upstream reads.
func csBaseTypes(n *ts.Node) []*ts.Node {
	var out []*ts.Node
	for i := uint(0); i < n.ChildCount(); i++ {
		if bl := n.Child(i); bl.Kind() == "base_list" {
			for j := uint(0); j < bl.NamedChildCount(); j++ {
				// Constructor args; upstream skips `B(x)` record bases.
				if c := bl.NamedChild(j); c.Kind() != "argument_list" && c.Kind() != "primary_constructor_base_type" {
					out = append(out, c)
				}
			}
		}
	}
	return out
}

// csIsInterfaceName reports whether base names an interface: declared as one in
// this file, or named IPascalCase by convention.
func csIsInterfaceName(base string, ifaces map[string]bool) bool {
	if ifaces[base] {
		return true
	}
	r := []rune(base)
	return len(r) >= 2 && r[0] == 'I' && unicode.IsUpper(r[1])
}

// csTypeParams returns skip plus the type parameters n declares.
func csTypeParams(n *ts.Node, skip map[string]bool, src []byte) map[string]bool {
	return listTypeParams(n, "type_parameter_list", skip, src)
}

// listTypeParams returns skip plus the names in the listKind child of n.
func listTypeParams(n *ts.Node, listKind string, skip map[string]bool, src []byte) map[string]bool {
	out := map[string]bool{}
	for k := range skip {
		out[k] = true
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		list := n.Child(i)
		if list.Kind() != listKind {
			continue
		}
		for j := uint(0); j < list.NamedChildCount(); j++ {
			p := list.NamedChild(j)
			for k := uint(0); k < p.NamedChildCount(); k++ {
				if id := p.NamedChild(k); id.Kind() == "identifier" {
					out[id.Utf8Text(src)] = true
					break
				}
			}
		}
	}
	return out
}

// csNestedTypes adds the names of the types nested in n to out. They have
// no node, so a ref to one must not bind to a namesake elsewhere.
func csNestedTypes(n *ts.Node, src []byte, out map[string]bool) map[string]bool {
	if body := n.ChildByFieldName("body"); body != nil {
		for i := uint(0); i < body.NamedChildCount(); i++ {
			switch c := body.NamedChild(i); c.Kind() {
			case "class_declaration", "struct_declaration", "interface_declaration",
				"enum_declaration", "record_declaration":
				out[fieldText(c, "name", src)] = true
				csNestedTypes(c, src, out)
			}
		}
	}
	return out
}

func (b *builder) csRef(id, name string, skip map[string]bool, loc model.Loc) {
	if !skip[name] {
		b.ref(id, name, "references", false, loc)
	}
}

// csTypeRefs records a reference from id to each type t names, generic
// arguments included. Keyword types and names in skip give nothing; a
// qualified name keeps its last segment.
func (b *builder) csTypeRefs(id string, t *ts.Node, skip map[string]bool, src []byte) {
	if t == nil {
		return
	}
	switch t.Kind() {
	case "predefined_type":
	case "identifier":
		b.csRef(id, t.Utf8Text(src), skip, line(t))
	case "qualified_name":
		text := t.Utf8Text(src)
		name, _, _ := strings.Cut(text[strings.LastIndex(text, ".")+1:], "<")
		b.csRef(id, name, skip, line(t))
	case "generic_name":
		for i := uint(0); i < t.NamedChildCount(); i++ {
			if c := t.NamedChild(i); c.Kind() == "identifier" || c.Kind() == "qualified_name" {
				text := c.Utf8Text(src)
				b.csRef(id, text[strings.LastIndex(text, ".")+1:], skip, line(c))
				break
			}
		}
		b.csTypeArgs(id, t, skip, src)
	case "tuple_type":
		for i := uint(0); i < t.NamedChildCount(); i++ {
			if el := t.NamedChild(i); el.Kind() == "tuple_element" {
				b.csTypeRefs(id, el.ChildByFieldName("type"), skip, src)
			}
		}
	default:
		for i := uint(0); i < t.NamedChildCount(); i++ {
			b.csTypeRefs(id, t.NamedChild(i), skip, src)
		}
	}
}

// csTypeArgs records the types in the type_argument_list of the generic_name t.
func (b *builder) csTypeArgs(id string, t *ts.Node, skip map[string]bool, src []byte) {
	for i := uint(0); i < t.NamedChildCount(); i++ {
		if args := t.NamedChild(i); args.Kind() == "type_argument_list" {
			for j := uint(0); j < args.NamedChildCount(); j++ {
				b.csTypeRefs(id, args.NamedChild(j), skip, src)
			}
		}
	}
}

// csParams records the type of each parameter of the parameter_list ps.
func (b *builder) csParams(id string, ps *ts.Node, skip map[string]bool, src []byte) {
	for i := uint(0); i < ps.NamedChildCount(); i++ {
		if p := ps.NamedChild(i); p.Kind() == "parameter" {
			b.csTypeRefs(id, p.ChildByFieldName("type"), skip, src)
		}
	}
}

// csSignature records the parameter and return types and the attributes of
// method m. Upstream reads attributes from methods only.
func (b *builder) csSignature(id string, m *ts.Node, skip map[string]bool, src []byte) {
	skip = csTypeParams(m, skip, src)
	if ps := m.ChildByFieldName("parameters"); ps != nil {
		b.csParams(id, ps, skip, src)
	}
	b.csTypeRefs(id, m.ChildByFieldName("returns"), skip, src)
	for i := uint(0); i < m.NamedChildCount(); i++ {
		al := m.NamedChild(i)
		if al.Kind() != "attribute_list" {
			continue
		}
		for j := uint(0); j < al.NamedChildCount(); j++ {
			if a := al.NamedChild(j); a.Kind() == "attribute" {
				if nm := a.ChildByFieldName("name"); nm != nil {
					text := nm.Utf8Text(src)
					b.csRef(id, text[strings.LastIndex(text, ".")+1:], skip, line(a))
				}
			}
		}
	}
}
