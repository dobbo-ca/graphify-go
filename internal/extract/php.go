package extract

import (
	ts "github.com/tree-sitter/go-tree-sitter"
	tsphp "github.com/tree-sitter/tree-sitter-php/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractPHP pulls functions, types (class/interface/trait/enum + methods),
// `use` imports, and call edges out of a .php file. Top-level functions and
// types become definitions; methods are nested under their type. Calls are
// recorded by the called name and resolved against the corpus by Resolve.
func extractPHP(rel string, src []byte) Result {
	root, done := parseRoot(src, tsphp.LanguagePHP())
	defer done()
	b := newBuilder(rel)

	b.phpItems(root, src)
	return b.res
}

// phpItems handles each statement directly under n (the program root or a
// namespace_definition body), recursing into braced namespace blocks.
func (b *builder) phpItems(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "function_definition":
			b.phpFunc(c, src)
		case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration":
			b.phpType(c, src)
		case "namespace_use_declaration":
			b.phpUse(c, src)
		case "namespace_definition":
			if body := c.ChildByFieldName("body"); body != nil {
				b.phpItems(body, src)
			}
		}
	}
}

func (b *builder) phpFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.phpCalls(n.ChildByFieldName("body"), id, src)
}

// phpType records a class/interface/trait/enum and the methods in its body,
// scoping each method under the type's name.
func (b *builder) phpType(n *ts.Node, src []byte) {
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
		m := body.Child(i)
		if m.Kind() != "method_declaration" {
			continue
		}
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
		// Register under the bare method name so `$x->method()` call sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.phpCalls(m.ChildByFieldName("body"), mid, src)
	}
}

// phpUse records the qualified path of each clause in a `use` declaration as an
// import (e.g. `use App\Models\User;` records App\Models\User).
func (b *builder) phpUse(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() != "namespace_use_clause" {
			continue
		}
		for j := uint(0); j < c.ChildCount(); j++ {
			nm := c.Child(j)
			if nm.Kind() == "qualified_name" || nm.Kind() == "name" {
				b.imp(nm.Utf8Text(src), line(n))
				break
			}
		}
	}
}

// phpCalls walks a function/method body and records each call site. Direct
// calls (`f()`), method calls (`$x->f()`, `$x?->f()`), and static calls
// (`C::f()`) all record the trailing simple name.
func (b *builder) phpCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		switch c.Kind() {
		case "function_call_expression":
			if fn := c.ChildByFieldName("function"); fn != nil {
				b.call(callerID, phpName(fn, src), line(c))
			}
		case "member_call_expression", "nullsafe_member_call_expression", "scoped_call_expression":
			if nm := c.ChildByFieldName("name"); nm != nil && nm.Kind() == "name" {
				b.call(callerID, nm.Utf8Text(src), line(c))
			}
		}
		return true
	})
}

// phpName returns the simple callee name for a function call target, taking the
// trailing segment of qualified/relative names (e.g. "add" for `App\Math\add`).
func phpName(n *ts.Node, src []byte) string {
	switch n.Kind() {
	case "name":
		return n.Utf8Text(src)
	case "qualified_name", "relative_name":
		var last string
		for i := uint(0); i < n.ChildCount(); i++ {
			if c := n.Child(i); c.Kind() == "name" {
				last = c.Utf8Text(src)
			}
		}
		return last
	}
	return ""
}
