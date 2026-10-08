package extract

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
)

// extractGo pulls functions, methods, type declarations, imports, and call
// edges out of a .go file.
func extractGo(rel string, src []byte) Result {
	root, done := parseRoot(src, tsgo.Language())
	defer done()
	b := newBuilder(rel)

	// Single named return type of each function in this file.
	rets := map[string]string{}
	for i := uint(0); i < root.ChildCount(); i++ {
		if n := root.Child(i); n.Kind() == "function_declaration" {
			rets[fieldText(n, "name", src)] = goTypeName(n.ChildByFieldName("result"), src)
		}
	}

	for i := uint(0); i < root.ChildCount(); i++ {
		n := root.Child(i)
		switch n.Kind() {
		case "function_declaration":
			b.goFunc(n, src, rets)
		case "method_declaration":
			b.goMethod(n, src, rets)
		case "type_declaration":
			b.goTypes(n, src)
		case "import_declaration":
			b.goImports(n, src)
		}
	}
	return b.res
}

func (b *builder) goFunc(n *ts.Node, src []byte, rets map[string]string) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.goCalls(n.ChildByFieldName("body"), id, "", goLocalTypes(n, src, rets), src)
}

func (b *builder) goMethod(n *ts.Node, src []byte, rets map[string]string) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	recv := goReceiverType(n, src)
	id := idutil.MakeID(b.stem, recv, name)
	label := name + "()"
	if recv != "" {
		label = recv + "." + name + "()"
	}
	// Register under the bare method name so `x.Method()` call sites resolve.
	b.def(id, name, label, line(n))
	b.res.Defs[len(b.res.Defs)-1].Owner = recv
	self := ""
	if p := firstNamed(n.ChildByFieldName("receiver")); p != nil {
		self = fieldText(p, "name", src)
	}
	b.goCalls(n.ChildByFieldName("body"), id, self, goLocalTypes(n, src, rets), src)
}

// goReceiverType returns the bare type name of a method receiver, e.g. "Server"
// for `func (s *Server) Handle()`.
func goReceiverType(n *ts.Node, src []byte) string {
	recv := n.ChildByFieldName("receiver")
	if recv == nil {
		return ""
	}
	var name string
	walk(recv, func(c *ts.Node) bool {
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

func (b *builder) goTypes(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() != "type_spec" {
			continue
		}
		name := fieldText(c, "name", src)
		if name == "" {
			continue
		}
		b.def(idutil.MakeID(b.stem, name), name, name, line(c))
	}
}

func (b *builder) goImports(n *ts.Node, src []byte) {
	walk(n, func(c *ts.Node) bool {
		if c.Kind() == "import_spec" || (c.Kind() == "import_declaration" && c.ChildByFieldName("path") != nil) {
			if path := c.ChildByFieldName("path"); path != nil {
				b.imp(unquote(path.Utf8Text(src)), line(c))
			}
		}
		return true
	})
}

// goLocalTypes maps the locals of function fn to their type, from a parameter
// or `var` (`x Foo`, `x *Foo`, `x pkg.Foo`), a composite literal (`x := Foo{}`,
// `x := &Foo{}`) or a call to a function in rets (`x := newFoo()`). A local also
// declared with anything else maps to "".
// ponytail: one flat scope per function, and rets only knows this file, so
// `x := pkg.New()` is untyped; that needs return types across files.
func goLocalTypes(fn *ts.Node, src []byte, rets map[string]string) map[string]string {
	types := map[string]string{}
	walk(fn, func(c *ts.Node) bool {
		switch c.Kind() {
		case "parameter_declaration", "var_spec":
			typ := goTypeName(c.ChildByFieldName("type"), src)
			for i := uint(0); i < c.NamedChildCount(); i++ {
				if p := c.NamedChild(i); p.Kind() == "identifier" {
					bindType(types, p.Utf8Text(src), typ)
				}
			}
		case "short_var_declaration":
			l, r := c.ChildByFieldName("left"), c.ChildByFieldName("right")
			if l == nil || r == nil {
				return true
			}
			for i := uint(0); i < l.NamedChildCount(); i++ {
				v := l.NamedChild(i)
				if v.Kind() != "identifier" {
					continue
				}
				typ := ""
				if l.NamedChildCount() == r.NamedChildCount() {
					e := r.NamedChild(i)
					if e.Kind() == "unary_expression" && fieldText(e, "operator", src) == "&" {
						e = e.ChildByFieldName("operand")
					}
					if e != nil && e.Kind() == "composite_literal" {
						typ = goTypeName(e.ChildByFieldName("type"), src)
					} else if e != nil && e.Kind() == "call_expression" {
						typ = rets[fieldText(e, "function", src)]
					}
				}
				bindType(types, v.Utf8Text(src), typ)
			}
		}
		return true
	})
	return types
}

// goTypeName returns the name of a plain or pointer named type, else "". A
// package-qualified type yields its bare name, which is what a method's owner
// is keyed by.
// ponytail: the package is dropped, so `r *http.Request` can bind to a corpus
// Request when it is the only one with that method; carry the package in Recv
// if that shows up.
func goTypeName(t *ts.Node, src []byte) string {
	if t != nil && t.Kind() == "pointer_type" {
		t = t.NamedChild(0)
	}
	if t != nil && t.Kind() == "qualified_type" {
		t = t.ChildByFieldName("name")
	}
	if t == nil || t.Kind() != "type_identifier" {
		return ""
	}
	return t.Utf8Text(src)
}

// goCalls walks a function body and records each call site, attributing it to
// callerID. Both direct calls (`f()`) and method/selector calls (`x.f()`) are
// recorded by the called name. self is the enclosing method's receiver name; a
// call on it is recorded with the receiver "self". A call on a typed local is
// recorded with its type as the receiver.
func (b *builder) goCalls(body *ts.Node, callerID, self string, types map[string]string, src []byte) {
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
		case "selector_expression":
			if f := fn.ChildByFieldName("field"); f != nil {
				recv := recvText(fn, f, src)
				if recv == self {
					recv = "self"
				} else if t := types[recv]; t != "" {
					recv = t
				}
				b.callRecv(callerID, f.Utf8Text(src), recv, line(c))
			}
		}
		return true
	})
}

func fieldText(n *ts.Node, field string, src []byte) string {
	if c := n.ChildByFieldName(field); c != nil {
		return c.Utf8Text(src)
	}
	return ""
}

func unquote(s string) string {
	return strings.Trim(s, "`\"'")
}
