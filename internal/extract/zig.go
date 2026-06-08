package extract

import (
	tszig "github.com/tree-sitter-grammars/tree-sitter-zig/bindings/go"
	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractZig pulls functions, container types (struct/enum/union/opaque) plus
// their methods, `@import` dependencies, and call edges out of a .zig file.
//
// Zig has no dedicated class/import statements: types are values bound by a
// `const`, e.g. `const Foo = struct { ... }`, and imports are
// `const std = @import("std")`. So a top-level definition is a
// variable_declaration whose value is a container declaration or an @import
// builtin, and a method is a function_declaration nested inside a container
// body.
func extractZig(rel string, src []byte) Result {
	root, done := parseRoot(src, tszig.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		n := root.Child(i)
		switch n.Kind() {
		case "function_declaration":
			b.zigFunc(n, src)
		case "variable_declaration":
			b.zigVarDecl(n, src)
		}
	}
	return b.res
}

// zigFunc records a top-level function definition and the calls in its body.
func (b *builder) zigFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.zigCalls(n.ChildByFieldName("body"), id, src)
}

// zigVarDecl handles a `const`/`var` binding. If its value is a container
// declaration it becomes a type (with nested methods); if its value is an
// `@import("...")` it becomes an import.
func (b *builder) zigVarDecl(n *ts.Node, src []byte) {
	name := zigVarName(n, src)
	if name == "" {
		return
	}
	val := zigVarValue(n)
	if val == nil {
		return
	}
	switch val.Kind() {
	case "struct_declaration", "enum_declaration", "union_declaration", "opaque_declaration":
		b.zigType(name, val, src)
	case "builtin_function":
		if spec := zigImportSpec(val, src); spec != "" {
			b.imp(spec, line(n))
		}
	}
}

// zigType records a container type definition and each method (a
// function_declaration directly inside the container body) scoped under it.
func (b *builder) zigType(name string, container *ts.Node, src []byte) {
	typeID := idutil.MakeID(b.stem, name)
	b.def(typeID, name, name, line(container))

	for i := uint(0); i < container.ChildCount(); i++ {
		m := container.Child(i)
		if m.Kind() != "function_declaration" {
			continue
		}
		mname := fieldText(m, "name", src)
		if mname == "" {
			continue
		}
		mid := idutil.MakeID(b.stem, name, mname)
		b.addNode(mid, name+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: typeID, Target: mid, Relation: "contains",
			Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(m),
		})
		// Register under the bare method name so `x.method()` call sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.zigCalls(m.ChildByFieldName("body"), mid, src)
	}
}

// zigVarName returns the bound name of a variable_declaration: the leading
// `identifier` child (the `:` type and any alignment clauses come after it).
func zigVarName(n *ts.Node, src []byte) string {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() == "identifier" {
			return c.Utf8Text(src)
		}
	}
	return ""
}

// zigVarValue returns the value expression of a variable_declaration (the node
// after `=`), skipping the name identifier, the `type` field, and the
// alignment/section metadata that can precede it.
func zigVarValue(n *ts.Node) *ts.Node {
	typeField := n.ChildByFieldName("type")
	seenName := false
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if !c.IsNamed() {
			continue
		}
		if typeField != nil && c.Equals(*typeField) {
			continue
		}
		switch c.Kind() {
		case "identifier":
			if !seenName {
				seenName = true
				continue
			}
			return c
		case "byte_alignment", "address_space", "link_section", "string":
			continue
		}
		return c
	}
	return nil
}

// zigImportSpec returns the imported path for an `@import("path")` builtin, or
// "" if the builtin is something else.
func zigImportSpec(bf *ts.Node, src []byte) string {
	var ident, args *ts.Node
	for i := uint(0); i < bf.ChildCount(); i++ {
		c := bf.Child(i)
		switch c.Kind() {
		case "builtin_identifier":
			ident = c
		case "arguments":
			args = c
		}
	}
	if ident == nil || args == nil || ident.Utf8Text(src) != "@import" {
		return ""
	}
	var spec string
	walk(args, func(c *ts.Node) bool {
		if spec != "" {
			return false
		}
		if c.Kind() == "string" {
			spec = unquote(c.Utf8Text(src))
			return false
		}
		return true
	})
	return spec
}

// zigCalls walks a function body and records each call site, attributing it to
// callerID. Direct calls (`f()`) record the identifier; field/method calls
// (`x.f()`) record the trailing member name.
func (b *builder) zigCalls(body *ts.Node, callerID string, src []byte) {
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
			if m := fn.ChildByFieldName("member"); m != nil {
				b.call(callerID, m.Utf8Text(src), line(c))
			}
		}
		return true
	})
}
