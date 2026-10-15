package extract

import (
	"bytes"
	"cmp"
	"github.com/dobbo-ca/gositter"
	tsjs "github.com/dobbo-ca/gositter/grammars/javascript"
	tstsx "github.com/dobbo-ca/gositter/grammars/tsx"
	tsts "github.com/dobbo-ca/gositter/grammars/typescript"
	"regexp"
	"slices"
	"strings"

	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// The call walk's kind, named before any parse; extractJS asks for its bit.
func init() {
	for _, l := range []*gositter.Language{tsjs.Language(), tsts.Language(), tstsx.Language()} {
		l.KindMask("call_expression")
		l.KindMask("export_statement")
	}
}

// extractJS pulls functions, classes (+ methods), interfaces, type aliases,
// enums, imports, and call edges out of a JS/TS/TSX file. The same walker
// serves all three because TypeScript's grammar is a superset of JavaScript's.
func extractJS(rel string, src []byte, langPtr *gositter.Language) Result {
	root, done := parseRoot(src, langPtr)
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		b.jsStatement(root.Child(i), src, "")
	}
	// One walk serves the import calls and every body jsCalls registered. The
	// bodies are in source order and nest, so a stack holds the open ones and
	// its top is the innermost body of the call.
	slices.SortStableFunc(b.jsBodies, func(x, y jsBody) int { return cmp.Compare(x.start, y.start) })
	next := 0
	var open []jsBody
	walkKinds(root, langPtr.KindMask("call_expression"), func(cv ts.Node) bool {
		c := &cv
		b.jsCallImport(c, src)
		for len(open) > 0 && c.StartByte() >= open[len(open)-1].end {
			open = open[:len(open)-1]
		}
		for next < len(b.jsBodies) && c.StartByte() >= b.jsBodies[next].start {
			if c.StartByte() < b.jsBodies[next].end {
				open = append(open, b.jsBodies[next])
			}
			next++
		}
		if len(open) > 0 {
			b.jsCall(c, open[len(open)-1], src)
		}
		return true
	})
	// An exported namespace member is no call target of plain code when the file has a plain one.
	for _, d := range b.jsNSDefs {
		if b.jsExp[d.ID] && !slices.ContainsFunc(b.res.Defs, func(p Def) bool { return p.Name == d.Name }) {
			b.res.Defs = append(b.res.Defs, d)
		}
	}
	b.jsExportOrphans(root, src, langPtr)
	// An owner node never takes the id of a declaration.
	for _, o := range b.jsOwners {
		if !b.seen[o.id] {
			b.constNode(o.id, o.owner, o.loc)
		}
	}
	b.jsProtoOwners()
	b.jsRationale(src)
	return b.res
}

// jsExportOrphans gives every exported declarator that has no node yet a
// node with no edge, as upstream's export facts do. Any export statement
// counts, nested or under a parse error. A const or let with an identifier
// value also names its whole pattern.
func (b *builder) jsExportOrphans(root *ts.Node, src []byte, lang *gositter.Language) {
	walkKinds(root, lang.KindMask("export_statement"), func(ev ts.Node) bool {
		d := ev.ChildByFieldName("declaration")
		if d == nil || d.Kind() != "lexical_declaration" && d.Kind() != "variable_declaration" {
			return true
		}
		for i := uint(0); i < d.NamedChildCount(); i++ {
			dc := d.NamedChild(i)
			nm, val := dc.ChildByFieldName("name"), dc.ChildByFieldName("value")
			if dc.Kind() != "variable_declarator" || nm == nil {
				continue
			}
			alias := val != nil && d.Kind() == "lexical_declaration" && (val.Kind() == "identifier" || val.Kind() == "type_identifier")
			if !alias && nm.Kind() != "identifier" {
				continue
			}
			name := nm.Utf8Text(src)
			if id := idutil.MakeID(b.stem, name); !degenerateName(name) && !b.seen[id] {
				b.addNode(id, name, line(&ev))
			}
		}
		return true
	})
}

// jsStatement handles one node, unwrapping `export ...` first, and recurses
// into anything it has no case for: a declaration in an IIFE, a callback or an
// `if` is a file-level one. owner is the caller of the closures met on the
// way, empty for none.
func (b *builder) jsStatement(n *ts.Node, src []byte, owner string) {
	switch n.Kind() {
	case "export_statement":
		d := n.ChildByFieldName("declaration")
		s := n.ChildByFieldName("source")
		if jsHasDefault(n) {
			b.jsDefault(n, d, src)
		}
		if d != nil {
			b.jsStatement(d, src, owner)
			if len(b.jsNS) > 0 {
				if b.jsExp == nil {
					b.jsExp = map[string]bool{}
				}
				b.jsExp[b.id(fieldText(d, "name", src))] = true
			}
		} else if s == nil {
			b.jsChildren(n, src, owner)
		}
		if s != nil {
			b.jsImp(n, unquote(s.Utf8Text(src)), src, true)
		} else if c := jsChild(n, "export_clause"); c != nil {
			for i := uint(0); i < c.NamedChildCount(); i++ {
				sp := c.NamedChild(i)
				e, a := fieldText(sp, "name", src), fieldText(sp, "alias", src)
				if a == "default" {
					b.res.Default = e // `export { Foo as default }`
				}
				if a != "" && a != e {
					e += " as " + a
				}
				b.res.Exports = append(b.res.Exports, e)
			}
		}
	case "import_statement":
		if s := n.ChildByFieldName("source"); s != nil {
			b.jsImp(n, unquote(s.Utf8Text(src)), src, false)
		}
	case "function_declaration", "generator_function_declaration":
		b.jsFunc(n, src)
	case "class_declaration", "abstract_class_declaration":
		b.jsClass(n, src)
	case "interface_declaration", "type_alias_declaration", "enum_declaration":
		b.jsNamedType(n, src)
	case "lexical_declaration", "variable_declaration":
		b.jsVarFuncs(n, src, owner)
	case "expression_statement":
		if !b.jsMemberAssign(n, src) {
			// Only a closure in a call statement is the file's call.
			for i := uint(0); i < n.ChildCount(); i++ {
				if k := n.Child(i).Kind(); k == "call_expression" || k == "new_expression" {
					owner = b.fileID
					// Its closures' members belong to the file.
					var cs []*ts.Node
					jsTopClosures(n.Child(i), &cs)
					for _, c := range cs {
						b.jsMemberScan(c.ChildByFieldName("body"), b.fileID, "", b.fileID, src)
					}
				}
			}
			b.jsChildren(n, src, owner)
		}
	case "internal_module", "module":
		b.jsNamespace(n, src, owner)
	case "method_definition", "method_signature":
		b.jsLitMethod(n, src)
	case "arrow_function", "function_expression", "function", "generator_function":
		if owner != "" {
			b.jsCalls(n.ChildByFieldName("body"), owner)
		}
		b.jsChildren(n, src, owner)
	default:
		b.jsChildren(n, src, owner)
	}
}

// jsNamespace makes a node for a namespace or module and walks its body with
// the name in member ids, so equal names in two namespaces both survive.
func (b *builder) jsNamespace(n *ts.Node, src []byte, owner string) {
	name := unquote(fieldText(n, "name", src))
	if !degenerateName(name) {
		b.constNode(b.id(name), name, line(n))
	}
	b.jsNS = append(b.jsNS, name)
	b.jsChildren(n, src, owner)
	b.jsNS = b.jsNS[:len(b.jsNS)-1]
}

func (b *builder) jsChildren(n *ts.Node, src []byte, owner string) {
	for i := uint(0); i < n.ChildCount(); i++ {
		b.jsStatement(n.Child(i), src, owner)
	}
}

// jsCallImport records `require("m")` and dynamic `import("m")` as imports.
// Only a lone string literal counts; a computed spec names no file.
func (b *builder) jsCallImport(n *ts.Node, src []byte) {
	if n.Kind() != "call_expression" {
		return
	}
	fn, args := n.ChildByFieldName("function"), n.ChildByFieldName("arguments")
	if fn == nil || args == nil || args.NamedChildCount() != 1 {
		return
	}
	if fn.Kind() != "import" && (fn.Kind() != "identifier" || fn.Utf8Text(src) != "require") {
		return
	}
	if a := args.NamedChild(0); a.Kind() == "string" {
		b.imp(unquote(a.Utf8Text(src)), line(n))
	}
}

// jsTypeOnlyImport reports whether an import or re-export statement is a
// whole-statement `import type { T } from "m"`. The `type` keyword is a direct child only in
// that form; for a mixed `import { type A, B }` tree-sitter nests it inside the
// specifier, so a direct-child check keeps mixed imports as value imports.
func jsTypeOnlyImport(n *ts.Node) bool {
	for i := uint(0); i < n.ChildCount(); i++ {
		if n.Child(i).Kind() == "type" {
			return true
		}
	}
	return false
}

// jsChild is the first child of kind k.
func jsChild(n *ts.Node, k string) *ts.Node {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == k {
			return c
		}
	}
	return nil
}

func jsHasDefault(n *ts.Node) bool { return jsChild(n, "default") != nil }

// jsDefault records the local name `export default` exports: a declaration's
// name or a bare identifier. An anonymous default has none.
func (b *builder) jsDefault(n, d *ts.Node, src []byte) {
	if d != nil {
		b.res.Default = fieldText(d, "name", src)
	} else if v := n.ChildByFieldName("value"); v != nil && v.Kind() == "identifier" {
		b.res.Default = v.Utf8Text(src)
	}
}

// jsImp records an import or `export ... from` and the names it binds. An
// import entry is "name" or "name as local" (name "default" for the default
// clause); a re-export entry is "name" or "name as exported", name being the
// one in the source module, "*" for `export * as ns`. A plain `export *` has
// none.
func (b *builder) jsImp(n *ts.Node, spec string, src []byte, export bool) {
	b.impTyped(spec, line(n), jsTypeOnlyImport(n))
	if spec == "" {
		return
	}
	im := &b.res.Imps[len(b.res.Imps)-1]
	im.Export = export
	spec1 := func(sp *ts.Node) {
		name := sp.ChildByFieldName("name")
		if name == nil || name.Kind() == "string" {
			return
		}
		e := name.Utf8Text(src)
		if a := fieldText(sp, "alias", src); a != "" && a != e {
			e += " as " + a
		}
		im.Names = append(im.Names, e)
	}
	if c := jsChild(n, "namespace_export"); c != nil {
		im.Names = []string{"*"}
	}
	if c := jsChild(n, "export_clause"); c != nil {
		for i := uint(0); i < c.NamedChildCount(); i++ {
			spec1(c.NamedChild(i))
		}
	}
	if c := jsChild(n, "import_clause"); c != nil {
		for i := uint(0); i < c.NamedChildCount(); i++ {
			switch k := c.NamedChild(i); k.Kind() {
			case "identifier":
				im.Names = append(im.Names, "default as "+k.Utf8Text(src))
			case "named_imports":
				for j := uint(0); j < k.NamedChildCount(); j++ {
					spec1(k.NamedChild(j))
				}
			}
		}
	}
}

// jsHeritage records the bases of a class or interface from its extends,
// implements or bare JS heritage clause. Only a plain or generic name is
// bound: a qualified name or a call names no declaration.
func (b *builder) jsHeritage(fromID string, clause *ts.Node, src []byte) {
	rel := "inherits"
	if clause.Kind() == "implements_clause" {
		rel = "implements"
	}
	for i := uint(0); i < clause.NamedChildCount(); i++ {
		c := clause.NamedChild(i)
		if c.Kind() == "generic_type" {
			c = c.NamedChild(0)
		}
		if k := c.Kind(); k == "identifier" || k == "type_identifier" {
			b.typeRef(fromID, c.Utf8Text(src), rel, line(clause))
		}
	}
}

// degenerateName reports whether a symbol name carries no identifier signal — it
// normalizes to an empty ID (a minified `$`, a JSONC `//` comment key). MakeID
// would collapse such a name to the bare file stem, producing a label-only noise
// node that can also collide with an extensionless sibling file. Skip it (#1899).
func degenerateName(name string) bool { return idutil.NormalizeID(name) == "" }

func (b *builder) jsFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if degenerateName(name) {
		return
	}
	id := b.id(name)
	b.def(id, name, name+"()", line(n))
	b.jsFnBody(n.ChildByFieldName("body"), id, src)
	b.jsMemberScan(n.ChildByFieldName("body"), id, name, id, src)
}

func (b *builder) jsClass(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if degenerateName(name) {
		return
	}
	classID := b.id(name)
	b.def(classID, name, name, line(n))
	if h := jsChild(n, "class_heritage"); h != nil {
		b.jsHeritage(classID, h, src)
		for i := uint(0); i < h.NamedChildCount(); i++ {
			if c := h.NamedChild(i); c.Kind() == "extends_clause" || c.Kind() == "implements_clause" {
				b.jsHeritage(classID, c, src)
			}
		}
	}

	// `@D export class X {}` hangs the decorator on the export statement.
	b.jsDecorators(classID, n, src)
	if p := n.Parent(); p != nil && p.Kind() == "export_statement" {
		b.jsDecorators(classID, p, src)
	}

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	var pending []*ts.Node // decorators written before the next member
	var scans [][2]any     // method bodies to scan once every member has its node
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		if m.Kind() == "class_static_block" {
			b.jsChildren(m, src, "")
			continue
		}
		if m.Kind() == "decorator" {
			pending = append(pending, m)
			continue
		}
		// A comment or a `;` does not part a decorator from its member.
		if !m.IsNamed() || m.Kind() == "comment" {
			continue
		}
		decos := pending
		pending = nil
		fn := m // the node with the parameters and the body
		if m.Kind() == "public_field_definition" || m.Kind() == "field_definition" {
			// A field is no node: its type and its decorators are the class's.
			b.jsTypeNames(classID, m.ChildByFieldName("type"), src, line(m))
			b.jsDecorators(classID, m, src)
			for _, d := range decos {
				b.jsDecorator(classID, d, src)
			}
			// A function value makes the field a method too.
			fn = m.ChildByFieldName("value")
			if fn == nil || !jsFuncValue(fn) {
				continue
			}
			decos = nil
		} else if m.Kind() != "method_definition" && m.Kind() != "abstract_method_signature" && m.Kind() != "method_signature" {
			continue
		}
		mname := fieldText(m, "name", src)
		if mname == "" {
			mname = fieldText(m, "property", src)
		}
		// `override abstract h()` parses as name "abstract" then ERROR "h".
		if mname == "abstract" {
			if e := jsChild(m, "ERROR"); e != nil && e.NamedChildCount() == 1 && e.NamedChild(0).Kind() == "identifier" {
				mname = e.NamedChild(0).Utf8Text(src)
			}
		}
		if degenerateName(mname) {
			continue
		}
		mid := b.id(name, mname)
		// The class owns an id another symbol took first too, as upstream.
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: classID, Target: mid, Relation: model.RelContains,
			Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(m),
		})
		// An overload's node sits at its first signature.
		if !b.seen[mid] {
			b.addNode(mid, name+"."+mname+"()", line(m))
			b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file, Abstract: m.Kind() != "method_definition" && fn == m})
		} else if m.Kind() == "method_definition" || fn != m {
			// The implementation after overload signatures is no abstract method.
			for j := len(b.res.Defs) - 1; j >= 0; j-- {
				if b.res.Defs[j].ID == mid {
					b.res.Defs[j].Abstract = false
					break
				}
			}
		}
		if fn == m {
			b.jsDecorators(mid, m, src)
		}
		for _, d := range decos {
			b.jsDecorator(mid, d, src)
		}
		if fn == m {
			b.jsSignature(mid, fn, src)
			b.jsFnBody(fn.ChildByFieldName("body"), mid, src)
			scans = append(scans, [2]any{fn.ChildByFieldName("body"), mid})
		} else {
			// Upstream scans no declarations in a field's function.
			b.jsCalls(fn.ChildByFieldName("body"), mid)
		}
	}
	// `this.x = fn` is the class's; a local object stays the method's.
	for _, sc := range scans {
		b.jsMemberScan(sc[0].(*ts.Node), classID, name, sc[1].(string), src)
	}
}

// jsFuncValue reports whether n is a function value.
func jsFuncValue(n *ts.Node) bool {
	switch n.Kind() {
	case "arrow_function", "function_expression", "function", "generator_function":
		return true
	}
	return false
}

// jsPrimitive holds the type names that are no symbols.
var jsPrimitive = set("string", "number", "boolean", "any", "unknown", "void", "never", "object",
	"null", "undefined", "bigint", "symbol", "this")

// jsSignature records the types the parameters and the return type of the
// method m name, and the decorators of its parameters.
func (b *builder) jsSignature(id string, m *ts.Node, src []byte) {
	if ps := m.ChildByFieldName("parameters"); ps != nil {
		for i := uint(0); i < ps.NamedChildCount(); i++ {
			p := ps.NamedChild(i)
			b.jsTypeNames(id, p.ChildByFieldName("type"), src, line(m))
			b.jsDecorators(id, p, src)
		}
	}
	b.jsTypeNames(id, m.ChildByFieldName("return_type"), src, line(m))
}

// jsTypeNames records each name under the type annotation n as a type
// reference of id. Resolve keeps only the names the file imports.
func (b *builder) jsTypeNames(id string, n *ts.Node, src []byte, loc model.Loc) {
	walk(n, func(cv ts.Node) bool {
		c := &cv
		switch c.Kind() {
		case "nested_type_identifier":
			if name := fieldText(c, "name", src); !jsPrimitive[name] {
				b.typeRef(id, name, "references", loc)
			}
			return false
		case "type_identifier", "identifier":
			if name := c.Utf8Text(src); !jsPrimitive[name] {
				b.typeRef(id, name, "references", loc)
			}
		}
		return true
	})
}

// jsDecorators records the decorators that are children of n.
func (b *builder) jsDecorators(id string, n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		if d := n.Child(i); d.Kind() == "decorator" {
			b.jsDecorator(id, d, src)
		}
	}
}

// jsDecorator records a references edge from id to the name of decorator d:
// `@D`, `@D(x)`, and the last name of `@ns.D`.
func (b *builder) jsDecorator(id string, d *ts.Node, src []byte) {
	e := firstNamed(d)
	if e != nil && e.Kind() == "call_expression" {
		e = e.ChildByFieldName("function")
	}
	if e != nil && e.Kind() == "member_expression" {
		e = e.ChildByFieldName("property")
	}
	if e != nil && (e.Kind() == "identifier" || e.Kind() == "property_identifier") {
		b.ref(id, e.Utf8Text(src), "references", true, line(d))
	}
}

func (b *builder) jsNamedType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if degenerateName(name) {
		return
	}
	id := b.id(name)
	b.def(id, name, name, line(n))
	if c := jsChild(n, "extends_type_clause"); c != nil {
		b.jsHeritage(id, c, src)
	}
	// An interface references its property types; its method signatures
	// wait for their nodes (graphify-go-vi5.9).
	if body := n.ChildByFieldName("body"); body != nil && n.Kind() == "interface_declaration" {
		for i := uint(0); i < body.NamedChildCount(); i++ {
			if m := body.NamedChild(i); m.Kind() == "property_signature" {
				b.jsTypeNames(id, m.ChildByFieldName("type"), src, line(m))
			}
		}
	}
}

// jsVarFuncs captures `const foo = () => {}` and `const foo = function() {}`,
// the dominant way functions are defined in modern JS/TS. Below module level
// it makes no node and only walks the value, as upstream's #1077 guard.
func (b *builder) jsVarFuncs(n *ts.Node, src []byte, owner string) {
	p := n.Parent()
	exported := p != nil && p.Kind() == "export_statement"
	if exported {
		p = p.Parent()
	}
	top := p != nil && p.Kind() == "program"
	// An exported variable of a namespace body is an orphan node.
	inNS := false
	if exported && p != nil && p.Kind() == "statement_block" {
		if g := p.Parent(); g != nil {
			inNS = g.Kind() == "internal_module" || g.Kind() == "module"
		}
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		d := n.Child(i)
		if d.Kind() != "variable_declarator" {
			continue
		}
		if nm := d.ChildByFieldName("name"); inNS && nm != nil && nm.Kind() == "identifier" && !degenerateName(nm.Utf8Text(src)) {
			id := b.id(nm.Utf8Text(src))
			b.addNode(id, nm.Utf8Text(src), line(d))
			if b.jsBare == nil {
				b.jsBare = map[string]bool{}
			}
			b.jsBare[id] = true
		}
		val := d.ChildByFieldName("value")
		// A type literal's method signatures are the walk's too, where no
		// value takes the declarator.
		if t := d.ChildByFieldName("type"); t != nil && (val == nil || !jsFuncValue(val) && !(top && n.Kind() == "lexical_declaration" && jsConstValue(d, val, exported))) {
			b.jsStatement(t, src, owner)
		}
		if val == nil {
			continue
		}
		fn := val.Kind() == "arrow_function" || val.Kind() == "function_expression" || val.Kind() == "function" || val.Kind() == "generator_function" && n.Kind() == "lexical_declaration"
		if top && !fn && n.Kind() == "lexical_declaration" && b.jsConst(d, val, exported, src) {
			continue
		}
		if !top || !fn {
			b.jsStatement(val, src, owner)
			continue
		}
		name := fieldText(d, "name", src)
		if degenerateName(name) {
			continue
		}
		id := b.id(name)
		b.def(id, name, name+"()", line(d))
		b.jsFnBody(val.ChildByFieldName("body"), id, src)
		if n.Kind() == "lexical_declaration" {
			b.jsMemberScan(val.ChildByFieldName("body"), id, name, id, src)
		}
	}
}

// jsConst makes a node for a module-level const or let that is no function:
// an object, array, cast, call or new value, or any value of an exported
// plain name. A closure in a call is called by the const. It reports
// whether the value was taken.
func (b *builder) jsConst(d, val *ts.Node, exported bool, src []byte) bool {
	nm := d.ChildByFieldName("name")
	if nm == nil {
		return false
	}
	if !jsConstValue(d, val, exported) {
		return false
	}
	if nm.Kind() == "object_pattern" && exported {
		for i := uint(0); i < nm.NamedChildCount(); i++ {
			pr := nm.NamedChild(i)
			name := ""
			switch pr.Kind() {
			case "shorthand_property_identifier_pattern":
				name = pr.Utf8Text(src)
			case "pair_pattern":
				if k := pr.ChildByFieldName("key"); k != nil {
					name = k.Utf8Text(src)
				}
			}
			if !degenerateName(name) {
				b.constDef(b.id(name), name, line(d))
			}
		}
		return true
	}
	name := nm.Utf8Text(src)
	if degenerateName(name) {
		return true
	}
	id := b.id(name)
	b.constDef(id, name, line(d))
	in := val
	for in.Kind() == "as_expression" || in.Kind() == "satisfies_expression" {
		if in.NamedChildCount() == 0 {
			return true
		}
		in = in.NamedChild(0)
	}
	if k := in.Kind(); k == "call_expression" || k == "new_expression" {
		b.jsClosures(in, id)
	}
	return true
}

// jsConstValue reports whether jsConst takes the declarator d.
func jsConstValue(d, val *ts.Node, exported bool) bool {
	switch val.Kind() {
	case "object", "array", "as_expression", "satisfies_expression", "call_expression", "new_expression":
		return d.ChildByFieldName("name") != nil
	}
	nm := d.ChildByFieldName("name")
	return exported && nm != nil && nm.Kind() == "identifier"
}

// constNode adds a node the file contains, with no Def: a namespace marker
// is no call target.
func (b *builder) constNode(id, name string, loc model.Loc) {
	b.addNode(id, name, loc)
	b.contains(id, loc)
}

// constDef is constNode for a const, which a call can bind.
func (b *builder) constDef(id, name string, loc model.Loc) {
	b.constNode(id, name, loc)
	// Abstract: a function of the same name wins the call.
	b.res.Defs = append(b.res.Defs, Def{ID: id, Name: name, File: b.file, Abstract: true, Value: true})
}

// jsClosures registers the outermost closures under n as bodies of id.
func (b *builder) jsClosures(n *ts.Node, id string) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "arrow_function", "function_expression", "function", "generator_function":
			b.jsCalls(c.ChildByFieldName("body"), id)
		default:
			b.jsClosures(c, id)
		}
	}
}

// jsMemberAssign captures `exports.x = fn`, `module.exports.x = fn` as a
// file function, `Foo.prototype.x = fn` as a method of Foo and `o.x = fn`
// as a method of o when the file exports o. Any other receiver names no
// owner: no node. It reports whether the statement was one.
func (b *builder) jsMemberAssign(n *ts.Node, src []byte) bool {
	a := firstNamed(n)
	if a == nil || a.Kind() != "assignment_expression" {
		return false
	}
	l, r := a.ChildByFieldName("left"), a.ChildByFieldName("right")
	if l == nil || r == nil || l.Kind() != "member_expression" || !jsFuncValue(r) {
		return false
	}
	name := fieldText(l, "property", src)
	o := l.ChildByFieldName("object")
	if o == nil || degenerateName(name) {
		return false
	}
	var id string
	switch kind, owner := jsReceiver(o, src); {
	case kind == "exports":
		id = b.id(name)
		b.def(id, name, name+"()", line(n))
	case kind == "prototype":
		id = b.id(owner, name)
		b.jsProto = append(b.jsProto, jsProto{id: id, owner: owner, name: name, loc: line(n), oid: b.id(owner)})
		b.res.Defs = append(b.res.Defs, Def{ID: id, Name: name, File: b.file})
	case kind == "object" && n.Parent() != nil && n.Parent().Kind() == "program" && b.jsExportedObj(n.Parent(), owner, src):
		oid := b.id(owner)
		if !b.seen[oid] {
			b.constDef(oid, owner, line(n))
		} else if !slices.ContainsFunc(b.res.Defs, func(d Def) bool { return d.ID == oid }) {
			b.res.Defs = append(b.res.Defs, Def{ID: oid, Name: owner, File: b.file, Abstract: true, Value: true})
		}
		id = b.id(owner, name)
		b.jsMethodNode(oid, id, owner, name, line(n))
		b.res.Defs = append(b.res.Defs, Def{ID: id, Name: name, File: b.file})
	default:
		return false
	}
	b.jsCalls(r.ChildByFieldName("body"), id)
	return true
}

// jsReceiver classifies the object of a member assignment: "this",
// "exports" (also module.exports), "prototype" with the owner of
// Foo.prototype, "object" with an identifier's name, or "" for any other.
func jsReceiver(o *ts.Node, src []byte) (string, string) {
	switch o.Kind() {
	case "this":
		return "this", ""
	case "identifier":
		if t := o.Utf8Text(src); t != "exports" {
			return "object", t
		}
		return "exports", ""
	case "member_expression":
		io, p := o.ChildByFieldName("object"), fieldText(o, "property", src)
		if io == nil || io.Kind() != "identifier" {
			return "", ""
		}
		if t := io.Utf8Text(src); t == "module" && p == "exports" {
			return "exports", ""
		} else if p == "prototype" {
			return "prototype", t
		}
	}
	return "", ""
}

// jsProto is a prototype member; its owner may be declared further down.
type jsProto struct {
	id, owner, name, oid string
	loc                  model.Loc
}

// jsProtoOwners hangs each prototype member on its owner, or on the file
// when the file declares no such owner.
func (b *builder) jsProtoOwners() {
	for _, p := range b.jsProto {
		if b.seen[p.oid] {
			b.jsMethodNode(p.oid, p.id, p.owner, p.name, p.loc)
			continue
		}
		b.addNode(p.id, p.owner+"."+p.name+"()", p.loc)
		b.contains(p.id, p.loc)
	}
}

// jsMethodNode adds the method node id of the owner node ownerID.
func (b *builder) jsMethodNode(ownerID, id, owner, name string, loc model.Loc) {
	if b.seen[id] {
		return
	}
	b.addNode(id, owner+"."+name+"()", loc)
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: ownerID, Target: id, Relation: model.RelContains,
		Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: loc,
	})
}

// jsExportedObj reports whether a module-level statement of prog exports
// name, as upstream's _js_find_exported_objects reads them.
func (b *builder) jsExportedObj(prog *ts.Node, name string, src []byte) bool {
	if b.jsExpObjs != nil {
		return b.jsExpObjs[name]
	}
	b.jsExpObjs = map[string]bool{}
	add := func(n *ts.Node) {
		if n != nil && n.Kind() == "identifier" {
			b.jsExpObjs[n.Utf8Text(src)] = true
		}
	}
	isExports := func(t string) bool {
		return t == "module.exports" || t == "exports" || strings.HasPrefix(t, "module.exports.") || strings.HasPrefix(t, "exports.")
	}
	for i := uint(0); i < prog.NamedChildCount(); i++ {
		st := prog.NamedChild(i)
		switch st.Kind() {
		case "export_statement":
			for j := uint(0); j < st.ChildCount(); j++ {
				switch c := st.Child(j); c.Kind() {
				case "identifier":
					add(c)
				case "export_clause":
					for k := uint(0); k < c.NamedChildCount(); k++ {
						add(c.NamedChild(k).ChildByFieldName("name"))
					}
				case "lexical_declaration", "variable_declaration":
					for k := uint(0); k < c.NamedChildCount(); k++ {
						add(c.NamedChild(k).ChildByFieldName("name"))
					}
				}
			}
		case "lexical_declaration", "variable_declaration":
			// `var x = module.exports = ...` or `var x = exports`.
			for k := uint(0); k < st.NamedChildCount(); k++ {
				d := st.NamedChild(k)
				nm, v := d.ChildByFieldName("name"), d.ChildByFieldName("value")
				if nm == nil || nm.Kind() != "identifier" || v == nil {
					continue
				}
				for v != nil && v.Kind() == "assignment_expression" {
					if l := v.ChildByFieldName("left"); l != nil && isExports(l.Utf8Text(src)) {
						add(nm)
						break
					}
					v = v.ChildByFieldName("right")
				}
				if v != nil && v.Kind() != "assignment_expression" && (v.Utf8Text(src) == "module.exports" || v.Utf8Text(src) == "exports") {
					add(nm)
				}
			}
		case "expression_statement":
			a := firstNamed(st)
			if a == nil || a.Kind() != "assignment_expression" {
				continue
			}
			if l := a.ChildByFieldName("left"); l == nil || !isExports(l.Utf8Text(src)) {
				continue
			}
			v := a.ChildByFieldName("right")
			for v != nil && v.Kind() == "assignment_expression" {
				add(v.ChildByFieldName("left"))
				v = v.ChildByFieldName("right")
			}
			if v != nil && v.Kind() == "identifier" {
				add(v)
			} else if v != nil && v.Kind() == "object" {
				for k := uint(0); k < v.NamedChildCount(); k++ {
					switch pr := v.NamedChild(k); pr.Kind() {
					case "shorthand_property_identifier":
						b.jsExpObjs[pr.Utf8Text(src)] = true
					case "pair":
						add(pr.ChildByFieldName("value"))
					}
				}
			}
		}
	}
	return b.jsExpObjs[name]
}

// jsTopClosures collects the outermost closures under n.
func jsTopClosures(n *ts.Node, out *[]*ts.Node) {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); jsFuncValue(c) {
			*out = append(*out, c)
		} else {
			jsTopClosures(c, out)
		}
	}
}

// jsMemberScan makes the members a function body assigns in its own
// statements: `this.x = fn` a method of thisID (owner its label, "" for the
// file: then x() is a file function), `o.x = fn` a method of o when o is
// an object literal the body declares, o itself under scope.
func (b *builder) jsMemberScan(body *ts.Node, thisID, owner, scope string, src []byte) {
	if body == nil {
		return
	}
	objs := map[string]*ts.Node{}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		st := body.NamedChild(i)
		if st.Kind() != "lexical_declaration" && st.Kind() != "variable_declaration" {
			continue
		}
		for j := uint(0); j < st.NamedChildCount(); j++ {
			d := st.NamedChild(j)
			nm, v := d.ChildByFieldName("name"), d.ChildByFieldName("value")
			if d.Kind() == "variable_declarator" && nm != nil && nm.Kind() == "identifier" && v != nil && v.Kind() == "object" {
				objs[nm.Utf8Text(src)] = d
			}
		}
	}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		st := body.NamedChild(i)
		a := firstNamed(st)
		if st.Kind() != "expression_statement" || a == nil || a.Kind() != "assignment_expression" {
			continue
		}
		l, r := a.ChildByFieldName("left"), a.ChildByFieldName("right")
		if l == nil || r == nil || l.Kind() != "member_expression" || !jsFuncValue(r) {
			continue
		}
		name := fieldText(l, "property", src)
		o := l.ChildByFieldName("object")
		if o == nil || degenerateName(name) {
			continue
		}
		var id string
		switch kind, on := jsReceiver(o, src); {
		case kind == "this" && owner == "":
			id = idutil.MakeID(thisID, name)
			b.addNode(id, name+"()", line(st))
			b.res.Edges = append(b.res.Edges, model.Edge{
				Source: thisID, Target: id, Relation: model.RelContains,
				Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(st),
			})
		case kind == "this":
			id = idutil.MakeID(thisID, name)
			b.jsMethodNode(thisID, id, owner, name, line(st))
		case kind == "object" && objs[on] != nil:
			oid := idutil.MakeID(scope, on)
			if !b.seen[oid] {
				b.addNode(oid, on, line(objs[on]))
				b.res.Edges = append(b.res.Edges, model.Edge{
					Source: scope, Target: oid, Relation: model.RelContains,
					Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(objs[on]),
				})
			}
			id = idutil.MakeID(oid, name)
			b.jsMethodNode(oid, id, on, name, line(st))
		default:
			continue
		}
		b.jsCalls(r.ChildByFieldName("body"), id)
	}
}

// jsLitMethod makes a node for a method the walk meets outside a class
// declaration: one of an object literal, a class expression or a type
// literal. Its owner is the module-level name the value is bound to, else
// the class expression's name, else the file.
func (b *builder) jsLitMethod(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if degenerateName(name) {
		return
	}
	// No Def: a literal's method is no call target of other files.
	owner, at := jsLitOwner(n, src)
	var id, thisID string
	if owner == "" {
		id = b.id(name)
		if !b.seen[id] {
			b.addNode(id, name+"()", line(n))
			b.contains(id, line(n))
		}
		thisID = id
	} else {
		oid := b.id(owner)
		b.jsOwners = append(b.jsOwners, jsProto{id: oid, owner: owner, loc: at})
		id = b.id(owner, name)
		b.jsMethodNode(oid, id, owner, name, line(n))
		thisID = oid
	}
	body := n.ChildByFieldName("body")
	b.jsFnBody(body, id, src)
	if owner == "" {
		owner = name
	}
	b.jsMemberScan(body, thisID, owner, id, src)
}

// jsLitOwner names the owner of the method n and the line of its binding.
// Only a module-level binding owns: a local one is not seen from outside.
func jsLitOwner(n *ts.Node, src []byte) (string, model.Loc) {
	cur := n.Parent()
	if cur == nil {
		return "", 0
	}
	cls := ""
	if cur.Kind() == "class_body" {
		if cur = cur.Parent(); cur == nil {
			return "", 0
		}
		cls = fieldText(cur, "name", src)
	}
	for p := cur.Parent(); p != nil; cur, p = p, p.Parent() {
		switch p.Kind() {
		case "pair", "object", "array", "parenthesized_expression", "as_expression", "satisfies_expression",
			"ternary_expression", "type_annotation", "object_type":
			continue
		case "variable_declarator":
			d := p.Parent()
			if d != nil && d.Parent() != nil && d.Parent().Kind() == "export_statement" {
				d = d.Parent()
			}
			if nm := p.ChildByFieldName("name"); nm != nil && nm.Kind() == "identifier" && d != nil && d.Parent() != nil && d.Parent().Kind() == "program" {
				return nm.Utf8Text(src), line(p)
			}
		case "assignment_expression":
			l := p.ChildByFieldName("left")
			st := p.Parent()
			if l == nil || st == nil || st.Kind() != "expression_statement" || st.Parent() == nil || st.Parent().Kind() != "program" {
				break
			}
			if l.Kind() == "identifier" {
				return l.Utf8Text(src), line(p)
			}
			if o := l.ChildByFieldName("object"); l.Kind() == "member_expression" && o != nil {
				switch kind, on := jsReceiver(o, src); kind {
				case "exports":
					return fieldText(l, "property", src), line(p)
				case "prototype":
					return on, line(p)
				}
			}
		}
		break
	}
	if !degenerateName(cls) {
		return cls, line(cur)
	}
	return "", 0
}

// jsBody is a function body whose calls belong to callerID.
type jsBody struct {
	start, end uint
	callerID   string
	ns         []string // namespaces around the body
}

// jsFnBody registers a function body and the functions declared inside it.
func (b *builder) jsFnBody(body *ts.Node, callerID string, src []byte) {
	b.jsCalls(body, callerID)
	b.jsNestedFuncs(body, callerID, src)
}

// jsNestedFuncs makes a node for each function declared inside n, under
// parent; a closure passes through, so its declarations keep the parent.
// A method of a class or object met here is no tracked body: its calls drop.
func (b *builder) jsNestedFuncs(n *ts.Node, parent string, src []byte) {
	if n == nil {
		return
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "function_declaration", "generator_function_declaration":
			name := fieldText(c, "name", src)
			if degenerateName(name) {
				continue
			}
			id := idutil.MakeID(parent, name)
			if !b.seen[id] {
				b.addNode(id, name+"()", line(c))
				b.res.Edges = append(b.res.Edges, model.Edge{
					Source: parent, Target: id, Relation: model.RelContains,
					Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(c),
				})
				b.jsScope(parent, name, id)
			}
			b.jsFnBody(c.ChildByFieldName("body"), id, src)
		case "method_definition":
			b.jsCalls(c.ChildByFieldName("body"), "")
			b.jsNestedFuncs(c, parent, src)
		default:
			b.jsNestedFuncs(c, parent, src)
		}
	}
}

// jsScope records that id is the function name declared inside parent. Such
// a function stays out of the global index: only calls from inside parent
// bind to it.
func (b *builder) jsScope(parent, name, id string) {
	if b.jsNested == nil {
		b.jsNested = map[[2]string]string{}
		b.jsParent = map[string]string{}
	}
	b.jsNested[[2]string{parent, name}] = id
	b.jsParent[id] = parent
}

// jsCalls registers a function body; extractJS records its call sites in the
// one walk it makes over the file.
func (b *builder) jsCalls(body *ts.Node, callerID string) {
	if body == nil {
		return
	}
	// The first owner of a body keeps it: a member scan runs before the walk.
	if b.jsBodySeen == nil {
		b.jsBodySeen = map[uint]bool{}
	}
	if b.jsBodySeen[body.StartByte()] {
		return
	}
	b.jsBodySeen[body.StartByte()] = true
	b.jsBodies = append(b.jsBodies, jsBody{body.StartByte(), body.EndByte(), callerID, slices.Clone(b.jsNS)})
}

// jsCall records the call site c, a call_expression, for the body's caller.
func (b *builder) jsCall(c *ts.Node, body jsBody, src []byte) {
	callerID := body.callerID
	fn := c.ChildByFieldName("function")
	if fn == nil || callerID == "" {
		return
	}
	// A method body is a boundary: its calls are no caller's.
	for p := c; p != nil && (p.StartByte() != body.start || p.EndByte() != body.end); p = p.Parent() {
		if p.Kind() == "method_definition" {
			return
		}
	}
	switch fn.Kind() {
	case "identifier":
		name := fn.Utf8Text(src)
		for s := callerID; s != ""; s = b.jsParent[s] {
			if id, ok := b.jsNested[[2]string{s, name}]; ok {
				b.jsNestedCall(callerID, id, line(c))
				return
			}
		}
		// Innermost namespace first; a name in none is a plain global call.
		for k := len(body.ns); k > 0; k-- {
			id := idutil.MakeID(append(append([]string{b.stem, "ns"}, body.ns[:k]...), name)...)
			if b.seen[id] && !b.jsBare[id] {
				b.jsNestedCall(callerID, id, line(c))
				return
			}
		}
		// The file's contains edge to its own function hides this call upstream.
		if callerID == b.fileID && b.jsFileOwns(name) {
			return
		}
		b.call(callerID, name, line(c))
	case "member_expression":
		if p := fn.ChildByFieldName("property"); p != nil {
			b.callRecv(callerID, p.Utf8Text(src), recvText(fn, p, src), line(c))
		}
	}
}

// jsFileOwns reports whether the file contains a definition called name.
func (b *builder) jsFileOwns(name string) bool {
	if b.jsOwned == nil {
		own := map[string]bool{}
		for _, e := range b.res.Edges {
			if e.Source == b.fileID && e.Relation == model.RelContains {
				own[e.Target] = true
			}
		}
		b.jsOwned = map[string]bool{}
		for _, ds := range [][]Def{b.res.Defs, b.jsNSDefs} {
			for _, d := range ds {
				if own[d.ID] {
					b.jsOwned[d.Name] = true
				}
			}
		}
	}
	return b.jsOwned[name]
}

// jsNestedCall binds a call to a nested function seen from its scope.
func (b *builder) jsNestedCall(callerID, id string, loc model.Loc) {
	if callerID == id || b.jsLinked[[2]string{callerID, id}] {
		return
	}
	if b.jsLinked == nil {
		b.jsLinked = map[[2]string]bool{}
	}
	b.jsLinked[[2]string{callerID, id}] = true
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: callerID, Target: id, Relation: model.RelCalls,
		Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: loc,
	})
}

// jsRationalePrefixes are the leading comment tokens that mark an explanatory
// comment worth capturing as a rationale node (mirrors upstream
// _JS_RATIONALE_PREFIXES; covers `//` line comments and `*`-prefixed lines
// inside block comments).
var jsRationalePrefixes = []string{
	"// NOTE:", "// IMPORTANT:", "// HACK:", "// WHY:", "// RATIONALE:",
	"// TODO:", "// FIXME:",
	"* NOTE:", "* IMPORTANT:", "* HACK:", "* WHY:", "* RATIONALE:",
	"* TODO:", "* FIXME:",
}

// jsCommentLineRe matches a line that begins (after optional indent) with a
// comment marker, so doc references are only harvested from comments.
var jsCommentLineRe = regexp.MustCompile(`^\s*(//|/\*|\*)`)

// jsDocRefRe finds architecture-decision / RFC references (ADR-0011, RFC 793) in
// a comment; jsDocRefParseRe splits a matched token into its kind and number.
var (
	jsDocRefRe      = regexp.MustCompile(`(?i)\b(ADR[- ]?\d{1,5}|RFC[- ]?\d{1,5})\b`)
	jsDocRefParseRe = regexp.MustCompile(`([A-Za-z]+)[- ]?(\d+)`)
)

// jsRationale is a deterministic post-pass mirroring upstream
// _extract_js_rationale: `// NOTE:`-style comments become "rationale" nodes
// (rationale_for edge to the file) and ADR/RFC tokens in comments become
// "doc_ref" nodes (cites edge from the file). No LLM is involved.
func (b *builder) jsRationale(src []byte) {
	seenDocRefs := map[string]bool{}
	for lineNum := 1; ; lineNum++ {
		line, rest, more := bytes.Cut(src, []byte("\n"))
		// Every prefix and comment marker starts with / or * after the indent.
		if t := bytes.TrimSpace(line); len(t) > 0 && (t[0] == '/' || t[0] == '*') {
			lineText, stripped := string(line), string(t)
			for _, p := range jsRationalePrefixes {
				if strings.HasPrefix(stripped, p) {
					b.addRationale(strings.TrimLeft(stripped, "/* "), lineNum, b.fileID)
					break
				}
			}
			if mayCiteDoc(stripped) && jsCommentLineRe.MatchString(lineText) {
				for _, m := range jsDocRefRe.FindAllStringSubmatch(stripped, -1) {
					b.addDocRef(m[1], lineNum, seenDocRefs)
				}
			}
		}
		if !more {
			return
		}
		src = rest
	}
}

// mayCiteDoc reports whether s holds "adr" or "rfc" in any case, as every
// jsDocRefRe match does.
func mayCiteDoc(s string) bool {
	for i := 1; i+1 < len(s); i++ {
		switch s[i] | 0x20 {
		case 'd':
			if s[i-1]|0x20 == 'a' && s[i+1]|0x20 == 'r' {
				return true
			}
		case 'f':
			if s[i-1]|0x20 == 'r' && s[i+1]|0x20 == 'c' {
				return true
			}
		}
	}
	return false
}

// addDocRef records a "doc_ref" node for a normalized ADR/RFC reference and a
// cites edge from the file to it. Labels are normalized (ADR-0011, RFC-793) and
// deduped per file so repeated citations collapse to one node. Mirrors upstream
// _add_doc_ref.
func (b *builder) addDocRef(token string, lineNum int, seenDocRefs map[string]bool) {
	m := jsDocRefParseRe.FindStringSubmatch(token)
	if m == nil {
		return
	}
	kind := strings.ToUpper(m[1])
	label := kind + "-" + m[2]
	if kind == "ADR" {
		label = kind + "-" + zfill(m[2], 4)
	}
	if seenDocRefs[label] {
		return
	}
	seenDocRefs[label] = true
	rid := idutil.MakeID("docref", label)
	loc := model.Line(lineNum)
	if !b.seen[rid] {
		b.seen[rid] = true
		b.res.Nodes = append(b.res.Nodes, model.Node{
			ID: rid, Label: label, FileType: "doc_ref",
			SourceFile: b.file, SourceLocation: loc.String(),
		})
	}
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: b.fileID, Target: rid, Relation: model.RelCites,
		Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: loc,
	})
}

// zfill left-pads s with zeros to at least width characters (Python str.zfill for
// unsigned numeric strings).
func zfill(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat("0", width-len(s)) + s
}
