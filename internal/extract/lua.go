package extract

import (
	tslua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"
	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractLua pulls functions, module/class tables (+ their methods), requires,
// and call edges out of a .lua file. Top-level `function f()` declarations
// become definitions; `function M.f()` / `function M:f()` are scoped under the
// table M. `local M = {}` registers M as a type. `require "m"` records an
// import. Calls are recorded by the called name and resolved by Resolve.
func extractLua(rel string, src []byte) Result {
	root, done := parseRoot(src, tslua.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		b.luaStatement(root.Child(i), src)
	}
	return b.res
}

// luaStatement handles one chunk-level statement.
func (b *builder) luaStatement(n *ts.Node, src []byte) {
	switch n.Kind() {
	case "function_declaration":
		b.luaFunc(n, src)
	case "variable_declaration", "assignment_statement":
		b.luaAssign(n, src)
		b.luaTopCalls(n, src)
	default:
		// Chunk-level expression statements may carry top-level require() calls.
		b.luaTopCalls(n, src)
	}
}

// luaFunc handles a `function ...() ... end` declaration. A plain identifier
// name is a top-level function; a dotted (`M.f`) or method (`M:f`) name is a
// method scoped under the table M.
func (b *builder) luaFunc(n *ts.Node, src []byte) {
	name := n.ChildByFieldName("name")
	if name == nil {
		return
	}
	body := n.ChildByFieldName("body")

	switch name.Kind() {
	case "identifier":
		fn := name.Utf8Text(src)
		id := idutil.MakeID(b.stem, fn)
		b.def(id, fn, fn+"()", line(n))
		b.luaCalls(body, id, src)
	case "dot_index_expression":
		b.luaMethod(name.ChildByFieldName("table"), name.ChildByFieldName("field"), body, n, src)
	case "method_index_expression":
		b.luaMethod(name.ChildByFieldName("table"), name.ChildByFieldName("method"), body, n, src)
	}
}

// luaMethod scopes a method under its receiver table, creating the type node on
// demand (Lua tables are usually declared as `local M = {}` but a method may be
// the first thing we see for M).
func (b *builder) luaMethod(table, field, body, decl *ts.Node, src []byte) {
	typeName := luaSimpleName(table, src)
	mname := luaSimpleName(field, src)
	if typeName == "" || mname == "" {
		return
	}
	b.luaEnsureType(typeName, line(decl))

	mid := idutil.MakeID(b.stem, typeName, mname)
	b.addNode(mid, typeName+"."+mname+"()", line(decl))
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: idutil.MakeID(b.stem, typeName), Target: mid, Relation: "contains",
		Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(decl),
	})
	// Register under the bare method name so `x.method()` call sites resolve.
	b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
	b.luaCalls(body, mid, src)
}

// luaEnsureType registers a table name as a type definition if not already seen.
func (b *builder) luaEnsureType(name, loc string) {
	id := idutil.MakeID(b.stem, name)
	if b.seen[id] {
		return
	}
	b.def(id, name, name, loc)
}

// luaAssign treats `local M = {}` / `M = {}` (an assignment whose value is a
// table constructor) as a module/class table definition.
func (b *builder) luaAssign(n *ts.Node, src []byte) {
	assign := n
	if n.Kind() == "variable_declaration" {
		a := childByKind(n, "assignment_statement")
		if a == nil {
			return
		}
		assign = a
	}
	vars := childByKind(assign, "variable_list")
	vals := childByKind(assign, "expression_list")
	if vars == nil || vals == nil {
		return
	}
	// Pair the first variable with the first value; only a bare identifier
	// assigned a table constructor counts as a type.
	name := luaSimpleName(firstNamed(vars), src)
	val := firstNamed(vals)
	if name == "" || val == nil || val.Kind() != "table_constructor" {
		return
	}
	b.luaEnsureType(name, line(n))
}

// luaSimpleName returns the bare identifier name for a variable / identifier
// node, unwrapping the `variable` wrapper. Non-identifier targets return "".
func luaSimpleName(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "identifier":
		return n.Utf8Text(src)
	case "variable":
		if c := firstNamed(n); c != nil {
			return luaSimpleName(c, src)
		}
	}
	return ""
}

// luaTopCalls records require()/other call sites that sit directly in a
// statement at chunk level (e.g. `local x = require "m"`), attributing them to
// the file node so module-level requires are captured even outside a function.
func (b *builder) luaTopCalls(n *ts.Node, src []byte) {
	walk(n, func(c *ts.Node) bool {
		if c.Kind() == "function_call" {
			b.luaCall(c, b.fileID, src)
		}
		return true
	})
}

// luaCalls walks a function body and records each call site, attributing them to
// callerID.
func (b *builder) luaCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		if c.Kind() == "function_call" {
			b.luaCall(c, callerID, src)
		}
		return true
	})
}

// luaCall records a single function_call. `require "m"` / `require("m")` is
// recorded as an import; everything else records the trailing callee name
// (identifier for `f()`, field for `m.f()`, method for `o:f()`).
func (b *builder) luaCall(c *ts.Node, callerID string, src []byte) {
	name := c.ChildByFieldName("name")
	if name == nil {
		return
	}
	callee := luaCalleeName(name, src)
	if callee == "require" {
		if spec := luaRequireSpec(c, src); spec != "" {
			b.imp(spec, line(c))
		}
		return
	}
	b.call(callerID, callee, line(c))
}

// luaCalleeName returns the trailing simple name of a call target.
func luaCalleeName(name *ts.Node, src []byte) string {
	switch name.Kind() {
	case "variable", "identifier":
		return luaSimpleName(name, src)
	case "dot_index_expression":
		if f := name.ChildByFieldName("field"); f != nil {
			return f.Utf8Text(src)
		}
	case "method_index_expression":
		if m := name.ChildByFieldName("method"); m != nil {
			return m.Utf8Text(src)
		}
	}
	return ""
}

// luaRequireSpec extracts the string literal passed to require, handling both
// `require("m")` and `require "m"`.
func luaRequireSpec(c *ts.Node, src []byte) string {
	args := c.ChildByFieldName("arguments")
	if args == nil {
		return ""
	}
	var spec string
	walk(args, func(n *ts.Node) bool {
		if spec != "" {
			return false
		}
		if n.Kind() == "string_content" {
			spec = n.Utf8Text(src)
			return false
		}
		return true
	})
	return spec
}

// childByKind returns the first direct child of n with the given kind, or nil.
func childByKind(n *ts.Node, kind string) *ts.Node {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == kind {
			return c
		}
	}
	return nil
}

// firstNamed returns the first named child of n, or nil.
func firstNamed(n *ts.Node) *ts.Node {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.IsNamed() {
			return c
		}
	}
	return nil
}
