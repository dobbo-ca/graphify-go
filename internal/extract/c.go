package extract

import (
	ts "github.com/tree-sitter/go-tree-sitter"
	tsc "github.com/tree-sitter/tree-sitter-c/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractC pulls functions, aggregate types (struct/union/enum), their
// function-pointer "methods", #include imports, and call edges out of a C file.
// C has no methods, so a function-pointer field of a struct/union is treated as
// a method of that aggregate (the common C idiom for behaviour on a type).
func extractC(rel string, src []byte) Result {
	root, done := parseRoot(src, tsc.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		n := root.Child(i)
		switch n.Kind() {
		case "function_definition":
			b.cFunc(n, src)
		case "struct_specifier", "union_specifier", "enum_specifier":
			b.cType(n, src)
		case "preproc_include":
			b.cInclude(n, src)
		case "declaration", "type_definition":
			// A struct/union/enum can be introduced inside a declaration
			// (`struct S {...} x;`) or a typedef (`typedef struct {...} S;`).
			if spec := n.ChildByFieldName("type"); spec != nil {
				switch spec.Kind() {
				case "struct_specifier", "union_specifier", "enum_specifier":
					b.cType(spec, src)
				}
			}
		}
	}
	return b.res
}

func (b *builder) cFunc(n *ts.Node, src []byte) {
	name := cDeclName(n.ChildByFieldName("declarator"), src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.cCalls(n.ChildByFieldName("body"), id, src)
}

// cType records a struct/union/enum definition and any function-pointer fields
// in its body as methods scoped under the type.
func (b *builder) cType(n *ts.Node, src []byte) {
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
		f := body.Child(i)
		if f.Kind() != "field_declaration" {
			continue
		}
		// A function-pointer field declares a callable member; its declarator
		// unwraps to a function_declarator over a field_identifier.
		decl := f.ChildByFieldName("declarator")
		if !cIsFuncPointer(decl) {
			continue
		}
		mname := cDeclName(decl, src)
		if mname == "" {
			continue
		}
		mid := idutil.MakeID(b.stem, name, mname)
		b.addNode(mid, name+"."+mname+"()", line(f))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: typeID, Target: mid, Relation: "contains",
			Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(f),
		})
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
	}
}

// cInclude records the included header path as an import. `<stdio.h>` and
// `"util.h"` both record the header path; neither is corpus-relative so both
// resolve to external dependency nodes.
func (b *builder) cInclude(n *ts.Node, src []byte) {
	p := n.ChildByFieldName("path")
	if p == nil {
		return
	}
	switch p.Kind() {
	case "system_lib_string":
		b.imp(trimAngle(p.Utf8Text(src)), line(n))
	case "string_literal":
		b.imp(cStringContent(p, src), line(n))
	}
}

// cCalls walks a function body and records each call site. Direct calls (`f()`)
// record the identifier; member calls (`x.f()`, `x->f()`) record the field name.
func (b *builder) cCalls(body *ts.Node, callerID string, src []byte) {
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
				b.call(callerID, f.Utf8Text(src), line(c))
			}
		}
		return true
	})
}

// cDeclName unwraps pointer/parenthesized/function declarators to the innermost
// identifier (or field_identifier) that names a function or pointer field.
func cDeclName(n *ts.Node, src []byte) string {
	for n != nil {
		switch n.Kind() {
		case "identifier", "field_identifier":
			return n.Utf8Text(src)
		case "pointer_declarator", "function_declarator",
			"array_declarator", "init_declarator":
			n = n.ChildByFieldName("declarator")
		default:
			// parenthesized_declarator and anything unexpected: fall back to
			// the first identifier descendant. For `int (*start)(...)` the
			// parenthesized subtree excludes the parameter list, so the first
			// identifier is the field name, not a parameter.
			var name string
			walk(n, func(c *ts.Node) bool {
				if name != "" {
					return false
				}
				if c.Kind() == "identifier" || c.Kind() == "field_identifier" {
					name = c.Utf8Text(src)
					return false
				}
				return true
			})
			return name
		}
	}
	return ""
}

// cIsFuncPointer reports whether a field declarator is a function pointer, i.e.
// it contains a function_declarator (e.g. `int (*start)(struct S *s)`).
func cIsFuncPointer(n *ts.Node) bool {
	if n == nil {
		return false
	}
	found := false
	walk(n, func(c *ts.Node) bool {
		if c.Kind() == "function_declarator" {
			found = true
			return false
		}
		return true
	})
	return found
}

// trimAngle strips the surrounding < > from a system include path.
func trimAngle(s string) string {
	if len(s) >= 2 && s[0] == '<' && s[len(s)-1] == '>' {
		return s[1 : len(s)-1]
	}
	return s
}

// cStringContent returns the inner text of a "..." string_literal include path,
// reading its string_content child and falling back to a quote-trimmed literal.
func cStringContent(p *ts.Node, src []byte) string {
	for i := uint(0); i < p.ChildCount(); i++ {
		if c := p.Child(i); c.Kind() == "string_content" {
			return c.Utf8Text(src)
		}
	}
	return unquote(p.Utf8Text(src))
}
