package extract

import (
	ts "github.com/tree-sitter/go-tree-sitter"
	tsruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// rubyRequires are the method names that act like imports: their first string
// argument names the required file/library.
var rubyRequires = map[string]bool{
	"require":          true,
	"require_relative": true,
	"load":             true,
	"autoload":         true,
}

// extractRuby pulls top-level methods, classes/modules (+ their methods),
// require-style imports, and call edges out of a .rb file. Top-level methods and
// classes/modules become definitions; methods defined inside a class/module are
// nested under it. Calls are recorded by the called method name and resolved
// against the corpus by Resolve.
func extractRuby(rel string, src []byte) Result {
	root, done := parseRoot(src, tsruby.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		b.rubyStatement(root.Child(i), src)
	}
	return b.res
}

// rubyStatement handles one top-level statement.
func (b *builder) rubyStatement(n *ts.Node, src []byte) {
	switch n.Kind() {
	case "method":
		b.rubyMethod(n, src)
	case "class", "module":
		b.rubyType(n, src)
	case "call":
		b.rubyRequire(n, "", src)
	}
}

func (b *builder) rubyMethod(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.rubyCalls(n.ChildByFieldName("body"), id, src)
}

// rubyType records a class or module and the methods defined directly in its
// body, scoping each method under the type's bare name.
func (b *builder) rubyType(n *ts.Node, src []byte) {
	name := rubyConstName(n.ChildByFieldName("name"), src)
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
		switch m.Kind() {
		case "method":
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
			b.rubyCalls(m.ChildByFieldName("body"), mid, src)
		case "call":
			// require/include inside a class body still counts as a file import.
			b.rubyRequire(m, typeID, src)
		}
	}
}

// rubyRequire records require-style imports. `require 'x'` / `require_relative
// 'x'` record the quoted path as the import spec. If callerID is non-empty, a
// non-require call is treated as a call site (covers calls in a class body).
func (b *builder) rubyRequire(n *ts.Node, callerID string, src []byte) {
	method := n.ChildByFieldName("method")
	if method == nil {
		return
	}
	name := method.Utf8Text(src)
	if rubyRequires[name] {
		if spec := rubyFirstStringArg(n, src); spec != "" {
			b.imp(spec, line(n))
		}
		return
	}
	if callerID != "" {
		b.rubyRecordCall(n, callerID, src)
	}
}

// rubyConstName returns the bare name of a class/module's name node, taking the
// trailing constant of a scope_resolution (e.g. "Bar" for `Foo::Bar`).
func rubyConstName(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "constant":
		return n.Utf8Text(src)
	case "scope_resolution":
		if name := n.ChildByFieldName("name"); name != nil {
			return name.Utf8Text(src)
		}
	}
	return ""
}

// rubyFirstStringArg returns the text of the first string-literal argument of a
// call, without its surrounding quotes.
func rubyFirstStringArg(n *ts.Node, src []byte) string {
	args := n.ChildByFieldName("arguments")
	if args == nil {
		return ""
	}
	for i := uint(0); i < args.ChildCount(); i++ {
		a := args.Child(i)
		if a.Kind() != "string" {
			continue
		}
		for j := uint(0); j < a.ChildCount(); j++ {
			if c := a.Child(j); c.Kind() == "string_content" {
				return c.Utf8Text(src)
			}
		}
		// Empty string literal.
		return ""
	}
	return ""
}

// rubyCalls walks a method body and records each call site.
func (b *builder) rubyCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		if c.Kind() == "call" {
			b.rubyRecordCall(c, callerID, src)
		}
		return true
	})
}

// rubyRecordCall records a single `call` node's callee. Both bare calls
// (`f(...)`) and method calls (`x.f(...)`) record the method name from the
// `method` field.
func (b *builder) rubyRecordCall(c *ts.Node, callerID string, src []byte) {
	method := c.ChildByFieldName("method")
	if method == nil {
		return
	}
	switch method.Kind() {
	case "identifier", "constant":
		b.call(callerID, method.Utf8Text(src), line(c))
	}
}
