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

	for i := uint(0); i < root.ChildCount(); i++ {
		n := root.Child(i)
		switch n.Kind() {
		case "function_declaration":
			b.goFunc(n, src)
		case "method_declaration":
			b.goMethod(n, src)
		case "type_declaration":
			b.goTypes(n, src)
		case "import_declaration":
			b.goImports(n, src)
		}
	}
	return b.res
}

func (b *builder) goFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.goCalls(n.ChildByFieldName("body"), id, src)
}

func (b *builder) goMethod(n *ts.Node, src []byte) {
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
	b.goCalls(n.ChildByFieldName("body"), id, src)
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

// goCalls walks a function body and records each call site, attributing it to
// callerID. Both direct calls (`f()`) and method/selector calls (`x.f()`) are
// recorded by the called name.
func (b *builder) goCalls(body *ts.Node, callerID string, src []byte) {
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
				b.call(callerID, f.Utf8Text(src), line(c))
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
