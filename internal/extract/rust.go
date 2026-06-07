package extract

import (
	ts "github.com/tree-sitter/go-tree-sitter"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractRust pulls functions, types (struct/enum/union/trait/type alias),
// inherent/trait impl methods, `use` imports, and call edges out of a .rs file.
// Items nested in `mod` blocks are descended into so module-organised crates
// still surface their definitions.
func extractRust(rel string, src []byte) Result {
	root, done := parseRoot(src, tsrust.Language())
	defer done()
	b := newBuilder(rel)

	b.rustItems(root, src)
	return b.res
}

// rustItems handles each item directly under n (a source_file or a mod's
// declaration_list), recursing into nested modules.
func (b *builder) rustItems(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "function_item":
			b.rustFunc(c, src)
		case "struct_item", "enum_item", "union_item", "trait_item", "type_item":
			b.rustType(c, src)
		case "impl_item":
			b.rustImpl(c, src)
		case "use_declaration":
			b.rustUse(c, src)
		case "mod_item":
			if body := c.ChildByFieldName("body"); body != nil {
				b.rustItems(body, src)
			}
		}
	}
}

func (b *builder) rustFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.rustCalls(n.ChildByFieldName("body"), id, src)
}

func (b *builder) rustType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	b.def(idutil.MakeID(b.stem, name), name, name, line(n))
}

// rustImpl records the methods of an `impl Type { ... }` (or `impl Trait for
// Type`) block, scoping each method under the implemented type's name.
func (b *builder) rustImpl(n *ts.Node, src []byte) {
	typeName := rustTypeName(n.ChildByFieldName("type"), src)
	if typeName == "" {
		return
	}
	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		if m.Kind() != "function_item" {
			continue
		}
		mname := fieldText(m, "name", src)
		if mname == "" {
			continue
		}
		mid := idutil.MakeID(b.stem, typeName, mname)
		b.addNode(mid, typeName+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: idutil.MakeID(b.stem, typeName), Target: mid, Relation: "contains",
			Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(m),
		})
		// Register under the bare method name so `x.method()` call sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.rustCalls(m.ChildByFieldName("body"), mid, src)
	}
}

// rustUse records the module path of a `use` declaration as an import. Grouped
// (`use a::{b, c}`) and aliased uses record the leading path.
func (b *builder) rustUse(n *ts.Node, src []byte) {
	arg := n.ChildByFieldName("argument")
	if arg == nil {
		return
	}
	if p := arg.ChildByFieldName("path"); p != nil {
		b.imp(p.Utf8Text(src), line(n))
		return
	}
	b.imp(arg.Utf8Text(src), line(n))
}

// rustTypeName returns a bare type name for impl targets, ignoring generic
// arguments and references (e.g. "Server" for `impl<T> Server<T>`).
func rustTypeName(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "type_identifier", "identifier":
		return n.Utf8Text(src)
	case "generic_type":
		return rustTypeName(n.ChildByFieldName("type"), src)
	case "scoped_type_identifier":
		return rustTypeName(n.ChildByFieldName("name"), src)
	case "reference_type":
		return rustTypeName(n.ChildByFieldName("type"), src)
	}
	// Fall back to the first type/identifier descendant.
	var name string
	walk(n, func(c *ts.Node) bool {
		if name != "" {
			return false
		}
		if c.Kind() == "type_identifier" {
			name = c.Utf8Text(src)
			return false
		}
		return true
	})
	return name
}

// rustCalls walks a function body and records each call site. Direct calls
// (`f()`), path calls (`mod::f()`), and method calls (`x.f()`) all record the
// trailing name; macro invocations (`f!()`) record the macro name.
func (b *builder) rustCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		switch c.Kind() {
		case "call_expression":
			fn := c.ChildByFieldName("function")
			if fn == nil {
				return true
			}
			switch fn.Kind() {
			case "identifier":
				b.call(callerID, fn.Utf8Text(src), line(c))
			case "scoped_identifier":
				if name := fn.ChildByFieldName("name"); name != nil {
					b.call(callerID, name.Utf8Text(src), line(c))
				}
			case "field_expression":
				if f := fn.ChildByFieldName("field"); f != nil {
					b.call(callerID, f.Utf8Text(src), line(c))
				}
			}
		case "macro_invocation":
			m := c.ChildByFieldName("macro")
			if m == nil {
				return true
			}
			if m.Kind() == "scoped_identifier" {
				m = m.ChildByFieldName("name")
			}
			if m != nil {
				b.call(callerID, m.Utf8Text(src), line(c))
			}
		}
		return true
	})
}
