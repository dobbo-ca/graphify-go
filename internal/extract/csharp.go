package extract

import (
	ts "github.com/tree-sitter/go-tree-sitter"
	tscs "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
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

	b.csItems(root, src)
	return b.res
}

// csItems handles each declaration directly under n (a compilation_unit, a
// namespace's declaration_list, or a file_scoped_namespace_declaration),
// recursing into nested namespaces.
func (b *builder) csItems(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		// Top-level statements wrap their inner statement in a global_statement.
		if c.Kind() == "global_statement" && c.NamedChildCount() > 0 {
			c = c.NamedChild(0)
		}
		switch c.Kind() {
		case "class_declaration", "struct_declaration", "interface_declaration",
			"enum_declaration", "record_declaration":
			b.csType(c, src)
		case "local_function_statement":
			b.csFunc(c, src)
		case "using_directive":
			b.csUsing(c, src)
		case "namespace_declaration":
			if body := c.ChildByFieldName("body"); body != nil {
				b.csItems(body, src)
			}
		case "file_scoped_namespace_declaration":
			// File-scoped namespaces hold their members as unfielded children.
			b.csItems(c, src)
		}
	}
}

// csFunc records a top-level (local) function and its call sites.
func (b *builder) csFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.csCalls(n.ChildByFieldName("body"), id, src)
}

// csType records a type definition and the methods declared in its body, each
// scoped under the type's name.
func (b *builder) csType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
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
		if m.Kind() != "method_declaration" {
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
		// Register under the bare method name so `x.Method()` call sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
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

// csCalls walks a method/function body and records each call site. Direct calls
// (`f()`), member calls (`x.f()`), and qualified calls (`A.B.f()`) all record
// the trailing invoked name.
func (b *builder) csCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		if c.Kind() != "invocation_expression" {
			return true
		}
		fn := c.ChildByFieldName("function")
		if fn == nil {
			return true
		}
		switch fn.Kind() {
		case "identifier":
			b.call(callerID, fn.Utf8Text(src), line(c))
		case "member_access_expression", "qualified_name":
			if name := fn.ChildByFieldName("name"); name != nil {
				b.call(callerID, name.Utf8Text(src), line(c))
			}
		}
		return true
	})
}
