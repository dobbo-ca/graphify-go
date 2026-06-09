package extract

import (
	ts "github.com/tree-sitter/go-tree-sitter"
	tsjulia "github.com/tree-sitter/tree-sitter-julia/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractJulia pulls functions, types (struct/abstract/primitive), modules with
// their contained functions (treated as methods), imports (`import`/`using`),
// and call edges out of a .jl file. Julia organises code in modules rather than
// classes, so a `module` becomes a type-like node and the `function`s defined
// directly inside it are scoped under it as methods.
func extractJulia(rel string, src []byte) Result {
	root, done := parseRoot(src, tsjulia.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		b.juliaItem(root.Child(i), src)
	}
	return b.res
}

// juliaItem handles one top-level (or module-level) statement.
func (b *builder) juliaItem(n *ts.Node, src []byte) {
	switch n.Kind() {
	case "function_definition", "macro_definition":
		b.juliaFunc(n, src)
	case "struct_definition", "abstract_definition", "primitive_definition":
		b.juliaType(n, src)
	case "module_definition":
		b.juliaModule(n, src)
	case "import_statement", "using_statement":
		b.juliaImports(n, src)
	case "assignment":
		// Short function form: `f(x) = expr`.
		b.juliaShortFunc(n, src)
	}
}

// juliaFunc registers a top-level function/macro definition and records the
// calls in its body.
func (b *builder) juliaFunc(n *ts.Node, src []byte) {
	name := juliaFuncName(n, src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.juliaCalls(juliaBody(n), id, src)
}

// juliaShortFunc handles `f(x) = expr`, where the assignment's left side is a
// call_expression. Other assignments are ignored.
func (b *builder) juliaShortFunc(n *ts.Node, src []byte) {
	lhs := n.NamedChild(0)
	if lhs == nil || lhs.Kind() != "call_expression" {
		return
	}
	name := juliaCalleeName(lhs.NamedChild(0), src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	// Record calls on the right-hand side of the definition.
	if rhs := n.NamedChild(1); rhs != nil {
		b.juliaCalls(rhs, id, src)
	}
}

// juliaType registers a struct/abstract/primitive type definition.
func (b *builder) juliaType(n *ts.Node, src []byte) {
	name := juliaTypeName(n, src)
	if name == "" {
		return
	}
	b.def(idutil.MakeID(b.stem, name), name, name, line(n))
}

// juliaModule registers a module as a type-like node and scopes the functions
// defined directly inside it as methods (Module.fn). Nested non-function items
// are still extracted at the module's level.
func (b *builder) juliaModule(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	modID := idutil.MakeID(b.stem, name)
	b.def(modID, name, name, line(n))

	body := juliaBody(n)
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		switch m.Kind() {
		case "function_definition", "macro_definition":
			mname := juliaFuncName(m, src)
			if mname == "" {
				continue
			}
			mid := idutil.MakeID(b.stem, name, mname)
			b.addNode(mid, name+"."+mname+"()", line(m))
			b.res.Edges = append(b.res.Edges, model.Edge{
				Source: modID, Target: mid, Relation: "contains",
				Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(m),
			})
			b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
			b.juliaCalls(juliaBody(m), mid, src)
		default:
			b.juliaItem(m, src)
		}
	}
}

// juliaImports records each `import`/`using` path. `using A`, `import A.B`, and
// `using A: x` all record the leading module path (A or A.B).
func (b *builder) juliaImports(n *ts.Node, src []byte) {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		switch c.Kind() {
		case "identifier":
			b.imp(c.Utf8Text(src), line(n))
		case "import_path":
			b.imp(c.Utf8Text(src), line(n))
		case "import_alias":
			// `import A as B` / `import A.B as C` — record the source path.
			if p := c.NamedChild(0); p != nil {
				b.imp(p.Utf8Text(src), line(n))
			}
		case "selected_import":
			// `using A: x, y` — record the leading module path only.
			if p := c.NamedChild(0); p != nil {
				switch p.Kind() {
				case "import_path", "identifier":
					b.imp(p.Utf8Text(src), line(n))
				}
			}
		}
	}
}

// juliaBody returns the `block` child of a definition node, or nil.
func juliaBody(n *ts.Node) *ts.Node {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == "block" {
			return c
		}
	}
	return nil
}

// juliaFuncName extracts the defined name from a function/macro definition by
// looking at its `signature` child: the first identifier in the signature's
// call_expression (`f(x)` -> "f"). A bare-identifier signature (`f`) also works.
func juliaFuncName(n *ts.Node, src []byte) string {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == "signature" {
			return juliaSignatureName(c, src)
		}
	}
	return ""
}

// juliaSignatureName unwraps a signature down to the called identifier. The
// signature may be a call_expression, a typed_expression, a where_expression,
// or a bare identifier.
func juliaSignatureName(sig *ts.Node, src []byte) string {
	var name string
	walk(sig, func(c *ts.Node) bool {
		if name != "" {
			return false
		}
		switch c.Kind() {
		case "call_expression":
			name = juliaCalleeName(c.NamedChild(0), src)
			return false
		case "identifier":
			name = c.Utf8Text(src)
			return false
		}
		return true
	})
	return name
}

// juliaTypeName extracts the type name from a definition's `type_head` child:
// the first identifier (the name, before any `<:` supertype or `{T}` params).
func juliaTypeName(n *ts.Node, src []byte) string {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == "type_head" {
			var name string
			walk(c, func(d *ts.Node) bool {
				if name != "" {
					return false
				}
				if d.Kind() == "identifier" {
					name = d.Utf8Text(src)
					return false
				}
				return true
			})
			return name
		}
	}
	return ""
}

// juliaCalleeName returns the simple callee name from the function position of a
// call: a bare identifier, or the trailing field of a `Mod.f` field_expression.
func juliaCalleeName(fn *ts.Node, src []byte) string {
	if fn == nil {
		return ""
	}
	switch fn.Kind() {
	case "identifier":
		return fn.Utf8Text(src)
	case "field_expression":
		// Trailing name is the last identifier child (`x.f` -> "f").
		var name string
		for i := uint(0); i < fn.NamedChildCount(); i++ {
			if c := fn.NamedChild(i); c.Kind() == "identifier" {
				name = c.Utf8Text(src)
			}
		}
		return name
	}
	return ""
}

// juliaCalls walks a definition body and records each call site by its simple
// callee name. Macro invocations (`@m ...`) are ignored as call edges.
func (b *builder) juliaCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		if c.Kind() != "call_expression" {
			return true
		}
		if name := juliaCalleeName(c.NamedChild(0), src); name != "" {
			b.call(callerID, name, line(c))
		}
		return true
	})
}
