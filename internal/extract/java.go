package extract

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractJava pulls types (class/interface/enum/record/annotation), their
// methods, imports, and call edges out of a .java file. Java has no top-level
// functions, so every method is scoped under its enclosing type. Methods are
// registered under their bare name so `x.method()` call sites resolve.
func extractJava(rel string, src []byte) Result {
	root, done := parseRoot(src, tsjava.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		n := root.Child(i)
		switch n.Kind() {
		case "class_declaration", "interface_declaration", "enum_declaration",
			"record_declaration", "annotation_type_declaration":
			b.javaType(n, src)
		case "import_declaration":
			b.javaImport(n, src)
		}
	}
	return b.res
}

// javaType records a type definition and the methods/constructors in its body,
// scoping each method under the type's name.
func (b *builder) javaType(n *ts.Node, src []byte) {
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
		if m.Kind() != "method_declaration" && m.Kind() != "constructor_declaration" {
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
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.javaCalls(m.ChildByFieldName("body"), mid, src)
	}
}

// javaImport records the imported type or package path. A trailing `.*`
// (on-demand import) is stripped to the package path.
func (b *builder) javaImport(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() == "scoped_identifier" || c.Kind() == "identifier" {
			b.imp(strings.TrimSuffix(c.Utf8Text(src), ".*"), line(n))
			return
		}
	}
}

// javaCalls walks a method body and records each invocation. Both direct calls
// (`f()`) and object/method calls (`x.f()`) are recorded by the invoked name.
func (b *builder) javaCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		if c.Kind() != "method_invocation" {
			return true
		}
		if name := c.ChildByFieldName("name"); name != nil {
			b.call(callerID, name.Utf8Text(src), line(c))
		}
		return true
	})
}
