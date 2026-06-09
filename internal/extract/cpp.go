package extract

import (
	ts "github.com/tree-sitter/go-tree-sitter"
	tscpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractCpp pulls free functions, types (class/struct/enum + methods),
// #includes / using-declarations, and call edges out of a C++ file. Items
// nested in `namespace` blocks are descended into so namespaced code still
// surfaces its definitions. Methods are scoped under their enclosing type,
// whether defined inline in the class body or out-of-line as `Type::method`.
func extractCpp(rel string, src []byte) Result {
	root, done := parseRoot(src, tscpp.Language())
	defer done()
	b := newBuilder(rel)

	b.cppItems(root, src)
	return b.res
}

// cppItems handles each item directly under n (a translation_unit or a
// namespace's declaration_list), recursing into nested namespaces.
func (b *builder) cppItems(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "function_definition":
			b.cppFunc(c, src)
		case "class_specifier", "struct_specifier", "union_specifier", "enum_specifier":
			b.cppType(c, src)
		case "preproc_include":
			b.cppInclude(c, src)
		case "using_declaration":
			b.cppUsing(c, src)
		case "namespace_definition":
			if body := c.ChildByFieldName("body"); body != nil {
				b.cppItems(body, src)
			}
		case "template_declaration":
			// Unwrap `template<...>` and re-dispatch on the templated item.
			b.cppItems(c, src)
		}
	}
}

// cppFunc records a free function (or an out-of-line method definition such as
// `Server::start`). Out-of-line definitions whose declarator names a scope are
// scoped under that type so they share the type's method node.
func (b *builder) cppFunc(n *ts.Node, src []byte) {
	scope, name := cppDeclaratorName(n.ChildByFieldName("declarator"), src)
	if name == "" {
		return
	}
	if scope != "" {
		mid := idutil.MakeID(b.stem, scope, name)
		b.addNode(mid, scope+"."+name+"()", line(n))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: idutil.MakeID(b.stem, scope), Target: mid, Relation: "contains",
			Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(n),
		})
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: name, File: b.file})
		b.cppCalls(n.ChildByFieldName("body"), mid, src)
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.cppCalls(n.ChildByFieldName("body"), id, src)
}

// cppType records a class/struct/union/enum and the methods inside its body.
// Inline methods appear as function_definition children of the
// field_declaration_list and are scoped under the type's name.
func (b *builder) cppType(n *ts.Node, src []byte) {
	name := cppNameText(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	typeID := idutil.MakeID(b.stem, name)
	b.def(typeID, name, name, line(n))

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		if m.Kind() != "function_definition" {
			continue
		}
		_, mname := cppDeclaratorName(m.ChildByFieldName("declarator"), src)
		if mname == "" {
			continue
		}
		mid := idutil.MakeID(b.stem, name, mname)
		b.addNode(mid, name+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: typeID, Target: mid, Relation: "contains",
			Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(m),
		})
		// Register under the bare method name so `x.method()` sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.cppCalls(m.ChildByFieldName("body"), mid, src)
	}
}

// cppInclude records the included header. `#include "x.h"` records the path
// without quotes; `#include <x>` records the bare header name.
func (b *builder) cppInclude(n *ts.Node, src []byte) {
	p := n.ChildByFieldName("path")
	if p == nil {
		return
	}
	spec := p.Utf8Text(src)
	switch p.Kind() {
	case "string_literal":
		spec = trimDelims(spec, '"', '"')
	case "system_lib_string":
		spec = trimDelims(spec, '<', '>')
	}
	b.imp(spec, line(n))
}

// cppUsing records a `using` declaration's qualified path as an import.
func (b *builder) cppUsing(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() == "identifier" || c.Kind() == "qualified_identifier" {
			b.imp(c.Utf8Text(src), line(n))
			return
		}
	}
}

// cppDeclaratorName descends a function_definition's declarator chain to its
// innermost name, returning (scope, name). scope is non-empty only for
// out-of-line `Scope::name` definitions.
func cppDeclaratorName(n *ts.Node, src []byte) (scope, name string) {
	for n != nil {
		switch n.Kind() {
		case "function_declarator", "reference_declarator", "pointer_declarator",
			"parenthesized_declarator":
			n = n.ChildByFieldName("declarator")
		case "qualified_identifier":
			return cppNameText(n.ChildByFieldName("scope"), src),
				cppNameText(n.ChildByFieldName("name"), src)
		case "identifier", "field_identifier", "type_identifier",
			"destructor_name", "operator_name":
			return "", n.Utf8Text(src)
		default:
			return "", ""
		}
	}
	return "", ""
}

// cppNameText returns the simple text of a name node, unwrapping a nested
// qualified_identifier down to its trailing component.
func cppNameText(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "qualified_identifier":
		return cppNameText(n.ChildByFieldName("name"), src)
	case "template_type", "template_function", "template_method":
		return cppNameText(n.ChildByFieldName("name"), src)
	}
	return n.Utf8Text(src)
}

// cppCalls walks a function body and records each call site. Direct calls
// (`f()`), member calls (`x.f()` / `p->f()`), and scoped calls (`N::f()`) all
// record the trailing called name.
func (b *builder) cppCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		if c.Kind() != "call_expression" {
			return true
		}
		fn := c.ChildByFieldName("function")
		if fn == nil {
			return true
		}
		switch fn.Kind() {
		case "identifier":
			b.call(callerID, fn.Utf8Text(src), line(c))
		case "field_expression":
			if f := fn.ChildByFieldName("field"); f != nil {
				b.call(callerID, cppNameText(f, src), line(c))
			}
		case "qualified_identifier":
			b.call(callerID, cppNameText(fn.ChildByFieldName("name"), src), line(c))
		case "template_function":
			b.call(callerID, cppNameText(fn.ChildByFieldName("name"), src), line(c))
		}
		return true
	})
}

// trimDelims strips a single leading open and trailing close delimiter if both
// are present (e.g. `"foo.h"` -> foo.h, `<vector>` -> vector).
func trimDelims(s string, open, close byte) string {
	if len(s) >= 2 && s[0] == open && s[len(s)-1] == close {
		return s[1 : len(s)-1]
	}
	return s
}
