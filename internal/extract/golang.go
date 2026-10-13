package extract

import (
	"strings"

	tsgo "github.com/dobbo-ca/gositter/grammars/golang"
	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/model"
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
	id := b.id(name)
	b.def(id, name, name+"()", line(n))
	b.goCalls(n, id, "", src, rets)
}

func (b *builder) goMethod(n *ts.Node, src []byte, rets map[string]string) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	recv := goReceiverType(n, src)
	id := b.id(recv, name)
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
	b.goCalls(n, id, self, src, rets)
}

// goReceiverType returns the bare type name of a method receiver, e.g. "Server"
// for `func (s *Server) Handle()`.
func goReceiverType(n *ts.Node, src []byte) string {
	recv := n.ChildByFieldName("receiver")
	if recv == nil {
		return ""
	}
	var name string
	walk(recv, func(cv ts.Node) bool {
		c := &cv
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
		b.def(b.id(name), name, name, line(c))
	}
}

func (b *builder) goImports(n *ts.Node, src []byte) {
	walk(n, func(cv ts.Node) bool {
		c := &cv
		if c.Kind() == "import_spec" || (c.Kind() == "import_declaration" && c.ChildByFieldName("path") != nil) {
			if path := c.ChildByFieldName("path"); path != nil {
				b.imp(unquote(path.Utf8Text(src)), line(c))
			}
		}
		return true
	})
}

// The kinds the goCalls walk acts on.
var goCallKinds = tsgo.Language().KindMask("call_expression", "parameter_declaration", "var_spec",
	"range_clause", "type_switch_statement", "short_var_declaration")

// goCalls records each call site in the body of function fn, attributing it to
// callerID. Both direct calls (`f()`) and method/selector calls (`x.f()`) are
// recorded by the called name. self is the enclosing method's receiver name; a
// call on it is recorded with the receiver "self". A call on a typed local is
// recorded with its type as the receiver.
//
// The same walk maps the locals of fn to their type, from a parameter
// or `var` (`x Foo`, `x *Foo`, `x pkg.Foo`), a composite literal (`x := Foo{}`,
// `x := &Foo{}`) or a call to a function in rets (`x := newFoo()`). A local also
// declared with anything else, range and type switch included, maps to "".
// ponytail: one flat scope per function, and rets only knows this file, so
// `x := pkg.New()` is untyped; that needs return types across files.
func (b *builder) goCalls(fn *ts.Node, callerID, self string, src []byte, rets map[string]string) {
	// Calls count only inside the body, which is the last child of fn.
	bodyStart := ^uint(0)
	if body := fn.ChildByFieldName("body"); body != nil {
		bodyStart = body.StartByte()
	}
	// The scope is flat, so a receiver is typed only once every local is seen.
	type site struct {
		callee, recv string
		loc          model.Loc
		member       bool
	}
	var sites []site
	types := map[string]string{}
	walkKinds(fn, goCallKinds, func(cv ts.Node) bool {
		c := &cv
		switch c.Kind() {
		case "call_expression":
			f := c.ChildByFieldName("function")
			if f == nil || c.StartByte() < bodyStart {
				return true
			}
			switch f.Kind() {
			case "identifier":
				sites = append(sites, site{callee: f.Utf8Text(src), loc: line(c)})
			case "selector_expression":
				if name := f.ChildByFieldName("field"); name != nil {
					sites = append(sites, site{name.Utf8Text(src), recvText(f, name, src), line(c), true})
				}
			}
		case "parameter_declaration", "var_spec":
			typ := goTypeName(c.ChildByFieldName("type"), src)
			for i := uint(0); i < c.NamedChildCount(); i++ {
				if p := c.NamedChild(i); p.Kind() == "identifier" {
					bindType(types, p.Utf8Text(src), typ)
				}
			}
		case "range_clause", "type_switch_statement":
			// A range or type-switch variable shadows a typed local.
			l := c.ChildByFieldName("left")
			if c.Kind() == "type_switch_statement" {
				l = c.ChildByFieldName("alias")
			}
			for i := uint(0); l != nil && i < l.NamedChildCount(); i++ {
				if v := l.NamedChild(i); v.Kind() == "identifier" {
					bindType(types, v.Utf8Text(src), "")
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
	for _, c := range sites {
		if !c.member {
			b.call(callerID, c.callee, c.loc)
			continue
		}
		recv := c.recv
		if recv == self {
			recv = "self"
		} else if t := types[recv]; t != "" {
			recv = t
		}
		b.callRecv(callerID, c.callee, recv, c.loc)
	}
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

func fieldText(n *ts.Node, field string, src []byte) string {
	if c := n.ChildByFieldName(field); c != nil {
		return c.Utf8Text(src)
	}
	return ""
}

func unquote(s string) string {
	return strings.Trim(s, "`\"'")
}
