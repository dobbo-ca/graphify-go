package extract

import (
	"unsafe"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractJS pulls functions, classes (+ methods), interfaces, type aliases,
// enums, imports, and call edges out of a JS/TS/TSX file. The same walker
// serves all three because TypeScript's grammar is a superset of JavaScript's.
func extractJS(rel string, src []byte, langPtr unsafe.Pointer) Result {
	root, done := parseRoot(src, langPtr)
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		b.jsStatement(root.Child(i), src)
	}
	return b.res
}

// jsStatement handles one top-level statement, unwrapping `export ...` first.
func (b *builder) jsStatement(n *ts.Node, src []byte) {
	switch n.Kind() {
	case "export_statement":
		if d := n.ChildByFieldName("declaration"); d != nil {
			b.jsStatement(d, src)
		}
	case "import_statement":
		if s := n.ChildByFieldName("source"); s != nil {
			b.imp(unquote(s.Utf8Text(src)), line(n))
		}
	case "function_declaration", "generator_function_declaration":
		b.jsFunc(n, src)
	case "class_declaration", "abstract_class_declaration":
		b.jsClass(n, src)
	case "interface_declaration", "type_alias_declaration", "enum_declaration":
		b.jsNamedType(n, src)
	case "lexical_declaration", "variable_declaration":
		b.jsVarFuncs(n, src)
	}
}

func (b *builder) jsFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.jsCalls(n.ChildByFieldName("body"), id, src)
}

func (b *builder) jsClass(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	classID := idutil.MakeID(b.stem, name)
	b.def(classID, name, name, line(n))

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		if m.Kind() != "method_definition" {
			continue
		}
		mname := fieldText(m, "name", src)
		if mname == "" {
			continue
		}
		mid := idutil.MakeID(b.stem, name, mname)
		b.addNode(mid, name+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: classID, Target: mid, Relation: "contains",
			Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(m),
		})
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.jsCalls(m.ChildByFieldName("body"), mid, src)
	}
}

func (b *builder) jsNamedType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	b.def(idutil.MakeID(b.stem, name), name, name, line(n))
}

// jsVarFuncs captures `const foo = () => {}` and `const foo = function() {}`,
// the dominant way functions are defined in modern JS/TS.
func (b *builder) jsVarFuncs(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		d := n.Child(i)
		if d.Kind() != "variable_declarator" {
			continue
		}
		val := d.ChildByFieldName("value")
		if val == nil || (val.Kind() != "arrow_function" && val.Kind() != "function_expression" && val.Kind() != "function") {
			continue
		}
		name := fieldText(d, "name", src)
		if name == "" {
			continue
		}
		id := idutil.MakeID(b.stem, name)
		b.def(id, name, name+"()", line(d))
		b.jsCalls(val.ChildByFieldName("body"), id, src)
	}
}

func (b *builder) jsCalls(body *ts.Node, callerID string, src []byte) {
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
		case "member_expression":
			if p := fn.ChildByFieldName("property"); p != nil {
				b.call(callerID, p.Utf8Text(src), line(c))
			}
		}
		return true
	})
}
