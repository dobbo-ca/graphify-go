package extract

import (
	"bytes"
	"regexp"
	"strings"

	tscpp "github.com/dobbo-ca/gositter/grammars/cpp"
	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractCpp pulls free functions, types (class/struct/enum + methods),
// #includes / using-declarations, and call edges out of a C++ file. Items
// nested in `namespace` blocks are descended into so namespaced code still
// surfaces its definitions. Methods are scoped under their enclosing type,
// whether defined inline in the class body or out-of-line as `Type::method`.
func extractCpp(rel string, src []byte) Result {
	src = blankCppExportMacros(src)
	root, done := parseRoot(src, tscpp.Language())
	defer done()
	b := newBuilder(rel)

	b.cppItems(root, src)
	return b.res
}

// Anchored: it runs only where a keyword starts, not over the whole file.
var cppExportMacroRe = regexp.MustCompile(`^(?:class|struct)\s+((?:[A-Z][A-Z0-9_]*\s+)+)([A-Za-z_]\w*(?:\s+final)?\s*)([:{])`)

// blankCppExportMacros overwrites the ALL-CAPS macros between `class`/`struct`
// and the type name (`class Q_CORE_EXPORT Widget`) with spaces: the grammar
// otherwise reads the macro as the type and the class as a function. Newlines
// are kept so offsets and line numbers hold.
func blankCppExportMacros(src []byte) []byte {
	var out []byte
	next := func(kw string, from int) int {
		if i := bytes.Index(src[from:], []byte(kw)); i >= 0 {
			return from + i
		}
		return len(src)
	}
	ci, si := next("class", 0), next("struct", 0)
	for pos := 0; ; {
		if ci < pos {
			ci = next("class", pos)
		}
		if si < pos {
			si = next("struct", pos)
		}
		p := min(ci, si)
		if p == len(src) {
			break
		}
		pos = p + 1
		// The \b the unanchored pattern began with.
		if p > 0 && (src[p-1] == '_' || src[p-1]|0x20 >= 'a' && src[p-1]|0x20 <= 'z' || src[p-1] >= '0' && src[p-1] <= '9') {
			continue
		}
		m := cppExportMacroRe.FindSubmatchIndex(src[p:])
		if m == nil {
			continue
		}
		for i := range m {
			m[i] += p
		}
		pos = m[1]
		// `for (class API v : items)` is a variable, not a type.
		if strings.HasSuffix(strings.TrimSpace(string(src[:m[0]])), "(") {
			continue
		}
		// `class API v{1};` is a brace initialiser.
		if rest := strings.TrimSpace(string(src[m[7]:])); src[m[6]] == '{' && rest != "" && strings.ContainsRune("0123456789\"'-", rune(rest[0])) {
			continue
		}
		end := m[3]
		// `class API WIDGET final :` has no lookahead: WIDGET is the name.
		if strings.TrimSpace(string(src[m[4]:m[5]])) == "final" {
			end = m[2] + strings.LastIndexAny(strings.TrimSpace(string(src[m[2]:m[3]])), " \t\r\n") + 1
		}
		if out == nil {
			out = append([]byte(nil), src...)
		}
		for i := m[2]; i < end; i++ {
			if out[i] != '\r' && out[i] != '\n' {
				out[i] = ' '
			}
		}
	}
	if out == nil {
		return src
	}
	return out
}

// cppItems handles each item directly under n (a translation_unit or a
// namespace's declaration_list), recursing into nested namespaces.
func (b *builder) cppItems(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "function_definition":
			b.cppFunc(c, src)
		case "class_specifier", "struct_specifier", "union_specifier", "enum_specifier":
			b.cppType(c, src)
		case "preproc_include":
			b.cppInclude(c, src)
		case "using_declaration":
			b.cppUsing(c, src)
		case "namespace_definition":
			if body := c.ChildByFieldName("body"); body != nil {
				b.cppItems(body, src)
			}
		case "template_declaration":
			// Unwrap `template<...>` and re-dispatch on the templated item.
			// Its parameters name no type while it lasts.
			outer := b.cppTParams
			b.cppTParams = cppTemplateParams(c, outer, src)
			b.cppItems(c, src)
			b.cppTParams = outer
		case "preproc_if", "preproc_ifdef", "preproc_else", "preproc_elif", "preproc_elifdef",
			"linkage_specification", "declaration_list", "ERROR":
			b.cppItems(c, src)
		case "type_definition":
			// `typedef struct S {...} T;` defines S.
			if spec := c.ChildByFieldName("type"); spec != nil && spec.ChildByFieldName("body") != nil {
				switch spec.Kind() {
				case "struct_specifier", "union_specifier", "enum_specifier":
					b.cppType(spec, src)
				}
			}
		}
	}
}

// cppFunc records a free function (or an out-of-line method definition such as
// `Server::start`). Out-of-line definitions whose declarator names a scope are
// scoped under that type so they share the type's method node.
func (b *builder) cppFunc(n *ts.Node, src []byte) {
	scope, name := cppDeclaratorName(n.ChildByFieldName("declarator"), src)
	if name == "" {
		return
	}
	if scope != "" {
		mid := b.id(scope, name)
		b.addNode(mid, scope+"."+name+"()", line(n))
		typeID := b.id(scope)
		if !b.seen[typeID] {
			// class is in a header: Resolve finds it
			b.contains(mid, line(n))
			b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: name, File: b.file, Owner: scope})
			b.cppSignature(mid, n, src)
			b.cppCalls(n.ChildByFieldName("body"), mid, src)
			return
		}
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: typeID, Target: mid, Relation: model.RelContains,
			Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(n),
		})
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: name, File: b.file})
		b.cppSignature(mid, n, src)
		b.cppCalls(n.ChildByFieldName("body"), mid, src)
		return
	}
	id := b.id(name)
	b.def(id, name, name+"()", line(n))
	b.cppSignature(id, n, src)
	b.cppCalls(n.ChildByFieldName("body"), id, src)
}

// cppSignature records a references edge from the function id to the types
// its return type and its parameters name.
func (b *builder) cppSignature(id string, fn *ts.Node, src []byte) {
	sigTypes(fn, func(t *ts.Node) { b.cppTypeRefs(id, t, src, line(fn)) })
}

// cppTypeRefs records a references edge from id to the type t names and to
// its template arguments. `ns::T` gives T, `struct S` and a primitive type
// give nothing, and a template parameter in scope names no type.
func (b *builder) cppTypeRefs(id string, t *ts.Node, src []byte, loc model.Loc) {
	if t == nil {
		return
	}
	switch t.Kind() {
	case "type_identifier":
		if name := t.Utf8Text(src); !b.cppTParams[name] {
			b.ref(id, name, "references", false, loc)
		}
	case "qualified_identifier":
		b.cppTypeRefs(id, t.ChildByFieldName("name"), src, loc)
	case "template_type":
		b.cppTypeRefs(id, t.ChildByFieldName("name"), src, loc)
		b.cppTemplateArgs(id, t, src, loc)
	}
}

// cppTemplateArgs records the types in the template arguments of t.
func (b *builder) cppTemplateArgs(id string, t *ts.Node, src []byte, loc model.Loc) {
	args := t.ChildByFieldName("arguments")
	if args == nil {
		return
	}
	for i := uint(0); i < args.NamedChildCount(); i++ {
		if a := args.NamedChild(i); a.Kind() == "type_descriptor" {
			b.cppTypeRefs(id, a.ChildByFieldName("type"), src, loc)
		}
	}
}

// cppTemplateParams returns outer plus the type parameter names of the
// template_declaration n.
func cppTemplateParams(n *ts.Node, outer map[string]bool, src []byte) map[string]bool {
	out := map[string]bool{}
	for k := range outer {
		out[k] = true
	}
	list := n.ChildByFieldName("parameters")
	for i := uint(0); list != nil && i < list.NamedChildCount(); i++ {
		p := list.NamedChild(i)
		if !strings.HasSuffix(p.Kind(), "type_parameter_declaration") {
			continue
		}
		// `class T = Default`: the first type name is the parameter.
		for j := uint(0); j < p.NamedChildCount(); j++ {
			if c := p.NamedChild(j); c.Kind() == "type_identifier" {
				out[c.Utf8Text(src)] = true
				break
			}
		}
	}
	return out
}

// cppType records a class/struct/union/enum and the methods inside its body.
// Inline methods appear as function_definition children of the
// field_declaration_list and are scoped under the type's name.
func (b *builder) cppType(n *ts.Node, src []byte) {
	name := cppNameText(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	typeID := b.id(name)
	b.def(typeID, name, name, line(n))

	for i := uint(0); i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c.Kind() == "base_class_clause" {
			for j := uint(0); j < c.NamedChildCount(); j++ {
				if base := c.NamedChild(j); base.Kind() != "access_specifier" {
					// `: Base<Dep>` inherits Base and references Dep.
					if name := cppNameText(base, src); !b.cppTParams[name] {
						b.ref(typeID, name, "inherits", false, line(n))
					}
					if base.Kind() == "qualified_identifier" {
						base = base.ChildByFieldName("name")
					}
					if base != nil && base.Kind() == "template_type" {
						b.cppTemplateArgs(typeID, base, src, line(n))
					}
				}
			}
		}
	}

	if body := n.ChildByFieldName("body"); body != nil {
		b.cppMembers(body, typeID, name, src)
	}
}

// cppMembers records the inline methods directly under body, and those behind
// a preprocessor conditional in it.
func (b *builder) cppMembers(body *ts.Node, typeID, name string, src []byte) {
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		if strings.HasPrefix(m.Kind(), "preproc_") {
			b.cppMembers(m, typeID, name, src)
			continue
		}
		if m.Kind() == "field_declaration" && !cppIsMethodDecl(m.ChildByFieldName("declarator")) {
			b.cppTypeRefs(typeID, m.ChildByFieldName("type"), src, line(m))
		}
		if m.Kind() != "function_definition" {
			continue
		}
		_, mname := cppDeclaratorName(m.ChildByFieldName("declarator"), src)
		if mname == "" {
			continue
		}
		mid := b.id(name, mname)
		b.addNode(mid, name+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: typeID, Target: mid, Relation: model.RelContains,
			Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(m),
		})
		// Register under the bare method name so `x.method()` sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.cppSignature(mid, m, src)
		b.cppCalls(m.ChildByFieldName("body"), mid, src)
	}
}

// cppIsMethodDecl reports whether the declarator of a class member declares a
// method (`T* m(int);`) and not a field or a function pointer (`T (*cb)(int);`).
func cppIsMethodDecl(d *ts.Node) bool {
	for d != nil && (d.Kind() == "pointer_declarator" || d.Kind() == "reference_declarator") {
		d = firstNamed(d)
	}
	if d == nil || d.Kind() != "function_declarator" {
		return false
	}
	inner := d.ChildByFieldName("declarator")
	return inner == nil || inner.Kind() != "parenthesized_declarator"
}

// cppInclude records the included header. `#include "x.h"` records the path
// without quotes; `#include <x>` records the bare header name.
func (b *builder) cppInclude(n *ts.Node, src []byte) {
	p := n.ChildByFieldName("path")
	if p == nil {
		return
	}
	spec := p.Utf8Text(src)
	switch p.Kind() {
	case "string_literal":
		b.impRel(trimDelims(spec, '"', '"'), line(n))
		return
	case "system_lib_string":
		spec = trimDelims(spec, '<', '>')
	}
	b.imp(spec, line(n))
}

// cppUsing records a `using` declaration's qualified path as an import.
func (b *builder) cppUsing(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() == "identifier" || c.Kind() == "qualified_identifier" {
			b.imp(c.Utf8Text(src), line(n))
			return
		}
	}
}

// cppDeclaratorName descends a function_definition's declarator chain to its
// innermost name, returning (scope, name). scope is non-empty only for
// out-of-line `Scope::name` definitions.
func cppDeclaratorName(n *ts.Node, src []byte) (scope, name string) {
	for n != nil {
		switch n.Kind() {
		case "function_declarator", "reference_declarator", "pointer_declarator",
			"parenthesized_declarator":
			n = n.ChildByFieldName("declarator")
		case "qualified_identifier":
			return cppNameText(n.ChildByFieldName("scope"), src),
				cppNameText(n.ChildByFieldName("name"), src)
		case "identifier", "field_identifier", "type_identifier",
			"destructor_name", "operator_name":
			return "", n.Utf8Text(src)
		default:
			return "", ""
		}
	}
	return "", ""
}

// cppNameText returns the simple text of a name node, unwrapping a nested
// qualified_identifier down to its trailing component.
func cppNameText(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "qualified_identifier":
		return cppNameText(n.ChildByFieldName("name"), src)
	case "template_type", "template_function", "template_method":
		return cppNameText(n.ChildByFieldName("name"), src)
	}
	return n.Utf8Text(src)
}

var cppCallKinds = tscpp.Language().KindMask("call_expression")

// cppCalls walks a function body and records each call site. Direct calls
// (`f()`), member calls (`x.f()` / `p->f()`), and scoped calls (`N::f()`) all
// record the trailing called name.
func (b *builder) cppCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walkKinds(body, cppCallKinds, func(cv ts.Node) bool {
		c := &cv
		fn := c.ChildByFieldName("function")
		if fn == nil {
			return true
		}
		switch fn.Kind() {
		case "identifier":
			b.call(callerID, fn.Utf8Text(src), line(c))
		case "field_expression":
			if f := fn.ChildByFieldName("field"); f != nil {
				b.callRecv(callerID, cppNameText(f, src), recvText(fn, f, src), line(c))
			}
		case "qualified_identifier":
			nm := fn.ChildByFieldName("name")
			for nm != nil && nm.Kind() == "qualified_identifier" {
				nm = nm.ChildByFieldName("name")
			}
			if nm != nil {
				b.callRecv(callerID, cppNameText(nm, src), recvText(fn, nm, src), line(c))
			}
		case "template_function":
			b.call(callerID, cppNameText(fn.ChildByFieldName("name"), src), line(c))
		}
		return true
	})
}

// trimDelims strips a single leading open and trailing close delimiter if both
// are present (e.g. `"foo.h"` -> foo.h, `<vector>` -> vector).
func trimDelims(s string, open, close byte) string {
	if len(s) >= 2 && s[0] == open && s[len(s)-1] == close {
		return s[1 : len(s)-1]
	}
	return s
}
