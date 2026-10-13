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

	b.goPkgs = goPkgPaths(root, src)
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
	b.goSignature(id, n, goTypeParams(n.ChildByFieldName("type_parameters"), "identifier", src), src)
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
	// `func (g *Gen[T]) M()`: T is the receiver's type parameter.
	var skip map[string]bool
	walk(n.ChildByFieldName("receiver"), func(cv ts.Node) bool {
		if cv.Kind() == "type_arguments" {
			skip = goTypeParams(&cv, "type_identifier", src)
		}
		return true
	})
	b.goSignature(id, n, skip, src)
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
		id := b.id(name)
		b.def(id, name, name, line(c))
		skip := goTypeParams(c.ChildByFieldName("type_parameters"), "identifier", src)
		t := c.ChildByFieldName("type")
		if t == nil {
			continue
		}
		switch t.Kind() {
		case "struct_type":
			walk(firstNamed(t), func(fv ts.Node) bool {
				f := &fv
				if f.Kind() != "field_declaration" {
					return f.Kind() == "field_declaration_list"
				}
				// An unnamed field embeds its type (graphify-go-vi5.11) and
				// references only that type's arguments.
				if f.ChildByFieldName("name") != nil {
					b.goRefs(id, f.ChildByFieldName("type"), skip, src, line(f))
				} else {
					b.goTypeArgs(id, f, skip, src)
				}
				return false
			})
		case "interface_type":
			// `~int | Early` and `~Early` are type sets; a lone type embeds.
			for j := uint(0); j < t.NamedChildCount(); j++ {
				e := t.NamedChild(j)
				if e.Kind() != "type_elem" {
					continue
				}
				if e.NamedChildCount() > 1 || e.NamedChild(0).Kind() == "negated_type" {
					b.goRefs(id, e, skip, src, line(e))
				} else {
					b.goTypeArgs(id, e, skip, src)
				}
			}
		}
	}
}

// goPredeclared holds the predeclared types, which are no corpus symbols.
var goPredeclared = set("bool", "byte", "complex64", "complex128", "error", "float32", "float64",
	"int", "int8", "int16", "int32", "int64", "rune", "string",
	"uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "any", "comparable")

var goTypeKinds = tsgo.Language().KindMask("type_identifier", "qualified_type")

// goTypeArgs records the types in the type arguments under n.
func (b *builder) goTypeArgs(id string, n *ts.Node, skip map[string]bool, src []byte) {
	walk(n, func(cv ts.Node) bool {
		if cv.Kind() != "type_arguments" {
			return true
		}
		b.goRefs(id, &cv, skip, src, line(n))
		return false
	})
}

// goSignature records the types the parameters and results of fn name.
func (b *builder) goSignature(id string, fn *ts.Node, skip map[string]bool, src []byte) {
	b.goRefs(id, fn.ChildByFieldName("parameters"), skip, src, line(fn))
	b.goRefs(id, fn.ChildByFieldName("result"), skip, src, line(fn))
}

// goRefs records a references edge from id to each type named under n, but
// for the predeclared types and the type parameters in skip. `pkg.T` carries
// the import path of pkg as its qualifier.
func (b *builder) goRefs(id string, n *ts.Node, skip map[string]bool, src []byte, loc model.Loc) {
	walkKinds(n, goTypeKinds, func(cv ts.Node) bool {
		c := &cv
		if c.Kind() == "qualified_type" {
			pkg := fieldText(c, "package", src)
			before := len(b.res.TypeRefs)
			b.ref(id, fieldText(c, "name", src), "references", false, loc)
			if len(b.res.TypeRefs) > before && pkg != "" {
				if p := b.goPkgs[pkg]; p != "" {
					pkg = p
				}
				b.res.TypeRefs[before].Qual = pkg
			}
			return false
		}
		if name := c.Utf8Text(src); !goPredeclared[name] && !skip[name] {
			b.ref(id, name, "references", false, loc)
		}
		return true
	})
}

// goTypeParams returns the names of kind under a type parameter list or a
// receiver's type arguments.
func goTypeParams(n *ts.Node, kind string, src []byte) map[string]bool {
	var out map[string]bool
	walk(n, func(cv ts.Node) bool {
		if cv.Kind() == kind {
			if out == nil {
				out = map[string]bool{}
			}
			out[cv.Utf8Text(src)] = true
		}
		return true
	})
	return out
}

// goPkgPaths maps the name each import binds in the file to its import path.
func goPkgPaths(root *ts.Node, src []byte) map[string]string {
	out := map[string]string{}
	for i := uint(0); i < root.ChildCount(); i++ {
		if root.Child(i).Kind() != "import_declaration" {
			continue
		}
		walk(root.Child(i), func(cv ts.Node) bool {
			c := &cv
			if c.Kind() != "import_spec" {
				return true
			}
			p := unquote(fieldText(c, "path", src))
			name := fieldText(c, "name", src)
			if name == "" {
				// The package of example.com/m/v2 is m by convention.
				seg := strings.Split(p, "/")
				name = seg[len(seg)-1]
				if len(seg) > 1 && len(name) > 1 && name[0] == 'v' && strings.Trim(name[1:], "0123456789") == "" {
					name = seg[len(seg)-2]
				}
			}
			out[name] = p
			return false
		})
	}
	return out
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
