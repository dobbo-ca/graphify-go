package extract

import (
	tskotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractKotlin pulls top-level functions, types (class/interface/enum/object)
// with their methods, imports, and call edges out of a .kt/.kts file. Kotlin
// interfaces and enum classes are both `class_declaration` nodes; objects are
// `object_declaration`. Methods nested in a type body are scoped under the type.
func extractKotlin(rel string, src []byte) Result {
	root, done := parseRoot(src, tskotlin.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		n := root.Child(i)
		switch n.Kind() {
		case "function_declaration":
			b.ktFunc(n, src)
		case "class_declaration", "object_declaration":
			b.ktType(n, src)
		case "import":
			b.ktImport(n, src)
		}
	}
	return b.res
}

func (b *builder) ktFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.ktCalls(ktBody(n), id, src)
}

// ktType records a class/interface/enum/object and the methods declared in its
// body. Both `class_body` and `enum_class_body` hold `function_declaration`
// members directly (the grammar inlines the member-declaration supertype).
func (b *builder) ktType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	typeID := idutil.MakeID(b.stem, name)
	b.def(typeID, name, name, line(n))

	for i := uint(0); i < n.ChildCount(); i++ {
		body := n.Child(i)
		if body.Kind() != "class_body" && body.Kind() != "enum_class_body" {
			continue
		}
		for j := uint(0); j < body.ChildCount(); j++ {
			m := body.Child(j)
			switch m.Kind() {
			case "function_declaration":
				b.ktMember(typeID, name, m, src)
			case "enum_entry":
				b.ktEnumEntry(typeID, m, src)
			}
		}
	}
}

// ktMember records a method scoped under ownerID (a type or an enum entry with
// an anonymous body), labelled ownerName.method(). It is registered under the
// bare method name so `x.method()` call sites resolve.
func (b *builder) ktMember(ownerID, ownerName string, m *ts.Node, src []byte) {
	mname := fieldText(m, "name", src)
	if mname == "" {
		return
	}
	mid := idutil.MakeID(ownerID, mname)
	b.addNode(mid, ownerName+"."+mname+"()", line(m))
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: ownerID, Target: mid, Relation: "contains",
		Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(m),
	})
	b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
	b.ktCalls(ktBody(m), mid, src)
}

// ktEnumEntry records an enum entry as a `case_of` the enum type. An entry with
// an anonymous class body (`GREEN { fun greet(){} }`) descends so the body's
// methods attach to the entry rather than being dropped.
func (b *builder) ktEnumEntry(typeID string, n *ts.Node, src []byte) {
	cname := ktEntryName(n, src)
	if cname == "" {
		return
	}
	cid := idutil.MakeID(typeID, cname)
	b.addNode(cid, cname, line(n))
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: typeID, Target: cid, Relation: "case_of",
		Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(n),
	})
	for i := uint(0); i < n.ChildCount(); i++ {
		body := n.Child(i)
		if body.Kind() != "class_body" {
			continue
		}
		for j := uint(0); j < body.ChildCount(); j++ {
			if m := body.Child(j); m.Kind() == "function_declaration" {
				b.ktMember(cid, cname, m, src)
			}
		}
	}
}

// ktEntryName returns an enum entry's name: its first identifier child. The
// grammar exposes it as a plain `simple_identifier`/`identifier` rather than a
// named field.
func ktEntryName(n *ts.Node, src []byte) string {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == "simple_identifier" || c.Kind() == "identifier" {
			return c.Utf8Text(src)
		}
	}
	return ""
}

// ktImport records the dotted path of an `import` directive as an import spec.
// The qualified path (e.g. kotlin.math.sqrt) is a bare specifier, so Resolve
// turns it into an external dependency node.
func (b *builder) ktImport(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() == "qualified_identifier" || c.Kind() == "identifier" {
			b.imp(c.Utf8Text(src), line(n))
			return
		}
	}
}

// ktBody returns the `function_body` child of a function_declaration, or nil for
// abstract/interface methods that have no body.
func ktBody(fn *ts.Node) *ts.Node {
	for i := uint(0); i < fn.ChildCount(); i++ {
		if c := fn.Child(i); c.Kind() == "function_body" {
			return c
		}
	}
	return nil
}

// ktCalls walks a function body and records each call site. Direct calls (`f()`)
// record the identifier; navigation calls (`x.f()`) record the trailing member
// name.
func (b *builder) ktCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		if c.Kind() != "call_expression" {
			return true
		}
		callee := c.Child(0)
		if callee == nil {
			return true
		}
		switch callee.Kind() {
		case "identifier":
			b.call(callerID, callee.Utf8Text(src), line(c))
		case "navigation_expression":
			b.call(callerID, ktNavName(callee, src), line(c))
		}
		return true
	})
}

// ktNavName returns the trailing member name of a navigation_expression
// (`a.b.c` -> "c"): the last identifier child.
func ktNavName(n *ts.Node, src []byte) string {
	var name string
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == "identifier" {
			name = c.Utf8Text(src)
		}
	}
	return name
}
