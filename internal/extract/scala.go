package extract

import (
	ts "github.com/tree-sitter/go-tree-sitter"
	tsscala "github.com/tree-sitter/tree-sitter-scala/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractScala pulls top-level functions, types (class/object/trait/enum) and
// their methods, imports, and call edges out of a .scala/.sc file. Top-level
// function and type definitions become definitions; methods declared in a
// type's template body are nested under their type. Calls are recorded by the
// called name and resolved against the corpus by Resolve.
func extractScala(rel string, src []byte) Result {
	root, done := parseRoot(src, tsscala.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		b.scalaItem(root.Child(i), src)
	}
	return b.res
}

// scalaItem handles one compilation-unit-level item, descending into braced
// package clauses so package-organised sources still surface their definitions.
func (b *builder) scalaItem(n *ts.Node, src []byte) {
	switch n.Kind() {
	case "function_definition":
		b.scalaFunc(n, src)
	case "class_definition", "object_definition", "trait_definition", "enum_definition", "package_object":
		b.scalaType(n, src)
	case "import_declaration":
		b.scalaImport(n, src)
	case "package_clause":
		// Braced `package foo { ... }` nests definitions in a template body.
		if body := n.ChildByFieldName("body"); body != nil {
			for i := uint(0); i < body.ChildCount(); i++ {
				b.scalaItem(body.Child(i), src)
			}
		}
	}
}

func (b *builder) scalaFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.scalaCalls(n.ChildByFieldName("body"), id, src)
}

// scalaType records a class/object/trait/enum and the methods in its template
// body, scoping each method under the type's name.
func (b *builder) scalaType(n *ts.Node, src []byte) {
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
		if m.Kind() != "function_definition" {
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
		b.scalaCalls(m.ChildByFieldName("body"), mid, src)
	}
}

// scalaImport records the dotted path of an `import` declaration, e.g.
// `scala.collection.mutable.Map` for `import scala.collection.mutable.Map`, or
// `a.b` for `import a.b.{c, d}`. Selector groups and wildcards (`{...}`, `_`,
// `*`) are dropped so the import resolves to the qualified path being pulled
// from. Path segments and their `.` separators are anonymous/named children of
// the declaration, so iterate raw children (not named-only).
func (b *builder) scalaImport(n *ts.Node, src []byte) {
	var path string
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "identifier", "operator_identifier", ".":
			path += c.Utf8Text(src)
		case "namespace_selectors", "namespace_wildcard", "as_renamed_identifier":
			// Stop at the first selector clause; everything before it is the path.
			b.imp(trimTrailingDot(path), line(n))
			return
		}
	}
	b.imp(trimTrailingDot(path), line(n))
}

func trimTrailingDot(s string) string {
	if len(s) > 0 && s[len(s)-1] == '.' {
		return s[:len(s)-1]
	}
	return s
}

// scalaCalls walks a definition body and records each call site. Direct calls
// (`f()`) record the identifier; field calls (`x.f()`) record the trailing
// field name, mirroring the Python/Go extractors.
func (b *builder) scalaCalls(body *ts.Node, callerID string, src []byte) {
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
		// `f[T]()` wraps the callee in a generic_function.
		if fn.Kind() == "generic_function" {
			if g := fn.ChildByFieldName("function"); g != nil {
				fn = g
			}
		}
		switch fn.Kind() {
		case "identifier":
			b.call(callerID, fn.Utf8Text(src), line(c))
		case "field_expression":
			if f := fn.ChildByFieldName("field"); f != nil {
				b.call(callerID, f.Utf8Text(src), line(c))
			}
		}
		return true
	})
}
