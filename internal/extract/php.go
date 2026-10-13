package extract

import (
	"strings"

	tsphp "github.com/dobbo-ca/gositter/grammars/php"
	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractPHP pulls functions, types (class/interface/trait/enum + methods),
// `use` imports, and call edges out of a .php file. Top-level functions and
// types become definitions; methods are nested under their type. Calls are
// recorded by the called name and resolved against the corpus by Resolve.
func extractPHP(rel string, src []byte) Result {
	root, done := parseRoot(src, tsphp.Language())
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
	id := b.id(name)
	b.def(id, name, name+"()", line(n))
	b.phpSignature(id, "", n, src)
	b.phpCalls(n.ChildByFieldName("body"), id, src)
}

// phpType records a class/interface/trait/enum and the methods in its body,
// scoping each method under the type's name.
func (b *builder) phpType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	typeID := b.id(name)
	b.def(typeID, name, name, line(n))
	b.phpBases(typeID, n, src)

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		if m.Kind() == "property_declaration" {
			b.phpTypeRefs(typeID, m.ChildByFieldName("type"), src)
		}
		if m.Kind() != "method_declaration" {
			continue
		}
		mname := fieldText(m, "name", src)
		if mname == "" {
			continue
		}
		mid := b.id(name, mname)
		b.addNode(mid, name+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: typeID, Target: mid, Relation: model.RelContains,
			Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(m),
		})
		// Register under the bare method name so `$x->method()` call sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.phpSignature(mid, typeID, m, src)
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

// phpConstructs are language constructs that parse like calls.
var phpConstructs = map[string]bool{"array": true, "die": true, "empty": true, "eval": true, "exit": true, "isset": true, "list": true, "unset": true}

var phpCallKinds = tsphp.Language().KindMask("function_call_expression", "member_call_expression",
	"nullsafe_member_call_expression", "scoped_call_expression")

// phpCalls walks a function/method body and records each call site. Direct
// calls (`f()`), method calls (`$x->f()`, `$x?->f()`), and static calls
// (`C::f()`) all record the trailing simple name.
func (b *builder) phpCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walkKinds(body, phpCallKinds, func(cv ts.Node) bool {
		c := &cv
		switch c.Kind() {
		case "function_call_expression":
			if fn := c.ChildByFieldName("function"); fn != nil {
				if name := phpName(fn, src); !phpConstructs[strings.ToLower(name)] {
					b.call(callerID, name, line(c))
				}
			}
		case "member_call_expression", "nullsafe_member_call_expression", "scoped_call_expression":
			if nm := c.ChildByFieldName("name"); nm != nil && nm.Kind() == "name" {
				b.callRecv(callerID, nm.Utf8Text(src), recvText(c, nm, src), line(c))
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

// phpBases records extends as inherits, implements, and trait use as
// mixes_in.
func (b *builder) phpBases(typeID string, n *ts.Node, src []byte) {
	base := func(list *ts.Node, rel string) {
		for i := uint(0); i < list.NamedChildCount(); i++ {
			if c := list.NamedChild(i); c.Kind() == "name" || c.Kind() == "qualified_name" {
				b.ref(typeID, phpLast(c, src), rel, false, line(list))
			}
		}
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		switch c := n.NamedChild(i); c.Kind() {
		case "base_clause":
			base(c, "inherits")
		case "class_interface_clause":
			base(c, "implements")
		}
	}
	if body := n.ChildByFieldName("body"); body != nil {
		for i := uint(0); i < body.NamedChildCount(); i++ {
			if c := body.NamedChild(i); c.Kind() == "use_declaration" {
				base(c, "mixes_in")
			}
		}
	}
}

// phpSignature records the parameter and return types of a function. A
// promoted constructor parameter is also a field of ownerID.
func (b *builder) phpSignature(id, ownerID string, n *ts.Node, src []byte) {
	if params := n.ChildByFieldName("parameters"); params != nil {
		for i := uint(0); i < params.NamedChildCount(); i++ {
			p := params.NamedChild(i)
			switch p.Kind() {
			case "simple_parameter":
			case "property_promotion_parameter":
				b.phpTypeRefs(ownerID, p.ChildByFieldName("type"), src)
			default:
				continue
			}
			b.phpTypeRefs(id, p.ChildByFieldName("type"), src)
		}
	}
	b.phpTypeRefs(id, n.ChildByFieldName("return_type"), src)
}

// phpTypeRefs records a reference from id to each class t names. Builtin
// types and the self, static and parent keywords give nothing.
func (b *builder) phpTypeRefs(id string, t *ts.Node, src []byte) {
	if t == nil || id == "" {
		return
	}
	switch t.Kind() {
	case "primitive_type":
	case "name", "qualified_name":
		switch name := phpLast(t, src); strings.ToLower(name) {
		case "self", "static", "parent":
		default:
			b.ref(id, name, "references", false, line(t))
		}
	default:
		for i := uint(0); i < t.NamedChildCount(); i++ {
			b.phpTypeRefs(id, t.NamedChild(i), src)
		}
	}
}

// phpLast returns the last segment of a possibly qualified name.
func phpLast(n *ts.Node, src []byte) string {
	t := n.Utf8Text(src)
	return t[strings.LastIndexByte(t, '\\')+1:]
}
