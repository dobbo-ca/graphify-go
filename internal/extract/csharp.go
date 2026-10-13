package extract

import (
	"strings"
	"unicode"

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

	b.csItems(root, src, csInterfaceNames(root, src))
	return b.res
}

// csItems handles each declaration directly under n (a compilation_unit, a
// namespace's declaration_list, or a file_scoped_namespace_declaration),
// recursing into nested namespaces.
func (b *builder) csItems(n *ts.Node, src []byte, ifaces map[string]bool) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		// Top-level statements wrap their inner statement in a global_statement.
		if c.Kind() == "global_statement" && c.NamedChildCount() > 0 {
			c = c.NamedChild(0)
		}
		switch c.Kind() {
		case "class_declaration", "struct_declaration", "interface_declaration",
			"enum_declaration", "record_declaration":
			b.csType(c, src, ifaces)
		case "local_function_statement":
			b.csFunc(c, src)
		case "using_directive":
			b.csUsing(c, src)
		case "namespace_declaration":
			if body := c.ChildByFieldName("body"); body != nil {
				b.csItems(body, src, ifaces)
			}
		case "file_scoped_namespace_declaration":
			// File-scoped namespaces hold their members as unfielded children.
			b.csItems(c, src, ifaces)
		}
	}
}

// csFunc records a top-level (local) function and its call sites.
func (b *builder) csFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := b.id(name)
	b.def(id, name, name+"()", line(n))
	b.csCalls(n.ChildByFieldName("body"), id, src)
}

// csType records a type definition and the methods declared in its body, each
// scoped under the type's name.
func (b *builder) csType(n *ts.Node, src []byte, ifaces map[string]bool) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	typeID := b.id(name)
	b.def(typeID, name, name, line(n))
	skip := csTypeParams(n, csNestedTypes(n, src, map[string]bool{}), src)
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
		b.addNode(mid, name+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: typeID, Target: mid, Relation: model.RelContains,
			Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(m),
		})
		// Register under the bare method name so `x.Method()` call sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.csSignature(mid, m, skip, src)
		b.csCalls(m.ChildByFieldName("body"), mid, src)
	}
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

var csCallKinds = tscs.Language().KindMask("invocation_expression")

// csCalls walks a method/function body and records each call site. Direct calls
// (`f()`), member calls (`x.f()`), and qualified calls (`A.B.f()`) all record
// the trailing invoked name.
func (b *builder) csCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walkKinds(body, csCallKinds, func(cv ts.Node) bool {
		c := &cv
		fn := c.ChildByFieldName("function")
		if fn == nil {
			return true
		}
		switch fn.Kind() {
		case "identifier":
			b.call(callerID, fn.Utf8Text(src), line(c))
		case "conditional_access_expression":
			// `x?.F()`: the callee is the binding's name.
			for i := uint(0); i < fn.ChildCount(); i++ {
				if mb := fn.Child(i); mb.Kind() == "member_binding_expression" {
					if name := mb.ChildByFieldName("name"); name != nil {
						b.call(callerID, name.Utf8Text(src), line(c))
					}
				}
			}
		case "member_access_expression", "qualified_name":
			if name := fn.ChildByFieldName("name"); name != nil {
				b.callRecv(callerID, name.Utf8Text(src), recvText(fn, name, src), line(c))
			}
		}
		return true
	})
}

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
	for i := uint(0); i < n.ChildCount(); i++ {
		bl := n.Child(i)
		if bl.Kind() != "base_list" {
			continue
		}
		for j := uint(0); j < bl.NamedChildCount(); j++ {
			c := bl.NamedChild(j)
			base := baseTypeName(c.Utf8Text(src))
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
