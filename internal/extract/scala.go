package extract

import (
	tsscala "github.com/dobbo-ca/gositter/grammars/scala"
	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractScala pulls top-level functions, types (class/object/trait/enum) and
// their methods, imports, and call edges out of a .scala/.sc file. Top-level
// function and type definitions become definitions; methods declared in a
// type's template body are nested under their type. Calls are recorded by the
// called name and resolved against the corpus by Resolve.
func extractScala(rel string, src []byte) Result {
	root, done := parseRoot(src, tsscala.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		b.scalaItem(root.Child(i), src)
	}
	return b.res
}

// scalaItem handles one compilation-unit-level item, descending into braced
// package clauses so package-organised sources still surface their definitions.
func (b *builder) scalaItem(n *ts.Node, src []byte) {
	switch n.Kind() {
	case "function_definition":
		b.scalaFunc(n, src)
	case "class_definition", "object_definition", "trait_definition", "enum_definition", "package_object":
		b.scalaType(n, src)
	case "import_declaration":
		b.scalaImport(n, src)
	case "package_clause":
		// Braced `package foo { ... }` nests definitions in a template body.
		if body := n.ChildByFieldName("body"); body != nil {
			for i := uint(0); i < body.ChildCount(); i++ {
				b.scalaItem(body.Child(i), src)
			}
		}
	}
}

func (b *builder) scalaFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := b.id(name)
	b.def(id, name, name+"()", line(n))
	b.scalaSignature(id, n, nil, src)
	b.scalaCalls(n.ChildByFieldName("body"), id, src)
}

// scalaType records a class/object/trait/enum and the methods in its template
// body, scoping each method under the type's name.
func (b *builder) scalaType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	typeID := b.id(name)
	b.def(typeID, name, name, line(n))

	body := n.ChildByFieldName("body")
	skip := scalaTypeParams(n, scalaNested(body, src, map[string]bool{}), src)
	b.scalaBases(typeID, n, src)
	b.scalaClassRefs(typeID, n, skip, src)
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		switch m.Kind() {
		case "val_definition", "var_definition", "type_definition":
			b.scalaTypeRefs(typeID, m.ChildByFieldName("type"), skip, src)
		}
		if m.Kind() != "function_definition" && m.Kind() != "function_declaration" {
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
		// Register under the bare method name so `x.method()` call sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file, Abstract: m.Kind() == "function_declaration"})
		b.scalaSignature(mid, m, skip, src)
		b.scalaCalls(m.ChildByFieldName("body"), mid, src)
	}
}

// scalaImport records the dotted path of an `import` declaration, e.g.
// `scala.collection.mutable.Map` for `import scala.collection.mutable.Map`, or
// `a.b` for `import a.b.{c, d}`. Selector groups and wildcards (`{...}`, `_`,
// `*`) are dropped so the import resolves to the qualified path being pulled
// from. Path segments and their `.` separators are anonymous/named children of
// the declaration, so iterate raw children (not named-only).
func (b *builder) scalaImport(n *ts.Node, src []byte) {
	var path string
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "identifier", "operator_identifier", ".":
			path += c.Utf8Text(src)
		case "namespace_selectors", "namespace_wildcard", "as_renamed_identifier":
			// Stop at the first selector clause; everything before it is the path.
			b.imp(trimTrailingDot(path), line(n))
			return
		}
	}
	b.imp(trimTrailingDot(path), line(n))
}

func trimTrailingDot(s string) string {
	if len(s) > 0 && s[len(s)-1] == '.' {
		return s[:len(s)-1]
	}
	return s
}

var scalaCallKinds = tsscala.Language().KindMask("call_expression")

// scalaCalls walks a definition body and records each call site. Direct calls
// (`f()`) record the identifier; field calls (`x.f()`) record the trailing
// field name, mirroring the Python/Go extractors.
func (b *builder) scalaCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walkKinds(body, scalaCallKinds, func(cv ts.Node) bool {
		c := &cv
		fn := c.ChildByFieldName("function")
		if fn == nil {
			return true
		}
		// `f[T]()` wraps the callee in a generic_function.
		if fn.Kind() == "generic_function" {
			if g := fn.ChildByFieldName("function"); g != nil {
				fn = g
			}
		}
		switch fn.Kind() {
		case "identifier":
			b.call(callerID, fn.Utf8Text(src), line(c))
		case "field_expression":
			if f := fn.ChildByFieldName("field"); f != nil {
				b.callRecv(callerID, f.Utf8Text(src), recvText(fn, f, src), line(c))
			}
		}
		return true
	})
}

// scalaNested adds the names of the types nested in the template body to
// out. They have no node, so a ref to one must not bind to a namesake.
func scalaNested(body *ts.Node, src []byte, out map[string]bool) map[string]bool {
	if body == nil {
		return out
	}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		switch c := body.NamedChild(i); c.Kind() {
		case "class_definition", "object_definition", "trait_definition", "enum_definition":
			out[fieldText(c, "name", src)] = true
			scalaNested(c.ChildByFieldName("body"), src, out)
		}
	}
	return out
}

// scalaTypeParams returns skip plus the type parameters n declares.
func scalaTypeParams(n *ts.Node, skip map[string]bool, src []byte) map[string]bool {
	out := map[string]bool{}
	for k := range skip {
		out[k] = true
	}
	var add func(list *ts.Node)
	add = func(list *ts.Node) {
		for i := uint(0); i < list.NamedChildCount(); i++ {
			switch c := list.NamedChild(i); c.Kind() {
			case "identifier":
				out[c.Utf8Text(src)] = true
			case "covariant_type_parameter", "contravariant_type_parameter":
				out[fieldText(c, "name", src)] = true
			case "type_parameters":
				add(c)
			}
		}
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c.Kind() == "type_parameters" {
			add(c)
		}
	}
	return out
}

// scalaBases records the first base as inherits and the rest as mixes_in.
func (b *builder) scalaBases(typeID string, n *ts.Node, src []byte) {
	ext := n.ChildByFieldName("extend")
	if ext == nil {
		return
	}
	rel := "inherits"
	for i := uint(0); i < ext.NamedChildCount(); i++ {
		c := ext.NamedChild(i)
		if c.Kind() == "generic_type" {
			if c = c.ChildByFieldName("type"); c == nil {
				continue
			}
		}
		name := ""
		switch c.Kind() {
		case "type_identifier":
			name = c.Utf8Text(src)
		case "stable_type_identifier":
			for j := uint(0); j < c.NamedChildCount(); j++ {
				name = c.NamedChild(j).Utf8Text(src)
			}
		}
		if name != "" {
			b.ref(typeID, name, rel, false, line(c))
			rel = "mixes_in"
		}
	}
}

// scalaClassRefs records the constructor parameter types, context bounds
// and derives clause of a type.
func (b *builder) scalaClassRefs(typeID string, n *ts.Node, skip map[string]bool, src []byte) {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		switch c := n.NamedChild(i); c.Kind() {
		case "class_parameters":
			for j := uint(0); j < c.NamedChildCount(); j++ {
				if p := c.NamedChild(j); p.Kind() == "class_parameter" {
					b.scalaTypeRefs(typeID, p.ChildByFieldName("type"), skip, src)
				}
			}
		case "type_parameters":
			b.scalaBounds(typeID, c, skip, src)
		case "derives_clause":
			for j := uint(0); j < c.NamedChildCount(); j++ {
				d := c.NamedChild(j)
				if d.Kind() != "type_identifier" && d.NamedChildCount() > 0 {
					d = d.NamedChild(d.NamedChildCount() - 1)
				}
				if d.Kind() == "type_identifier" {
					b.ref(typeID, d.Utf8Text(src), "references", false, line(d))
				}
			}
		}
	}
}

// scalaSignature records the types of the first parameter list, the return
// type and the context bounds of a function.
func (b *builder) scalaSignature(id string, n *ts.Node, skip map[string]bool, src []byte) {
	skip = scalaTypeParams(n, skip, src)
	params := false
	for i := uint(0); i < n.NamedChildCount(); i++ {
		switch c := n.NamedChild(i); c.Kind() {
		case "parameters":
			if params {
				continue
			}
			params = true
			for j := uint(0); j < c.NamedChildCount(); j++ {
				if p := c.NamedChild(j); p.Kind() == "parameter" {
					b.scalaTypeRefs(id, p.ChildByFieldName("type"), skip, src)
				}
			}
		case "type_parameters":
			b.scalaBounds(id, c, skip, src)
		}
	}
	b.scalaTypeRefs(id, n.ChildByFieldName("return_type"), skip, src)
}

// scalaBounds records the context bounds (`[A: Ordering]`) of a type
// parameter list, nested lists included.
func (b *builder) scalaBounds(id string, list *ts.Node, skip map[string]bool, src []byte) {
	for i := uint(0); i < list.NamedChildCount(); i++ {
		switch c := list.NamedChild(i); c.Kind() {
		case "type_parameters":
			b.scalaBounds(id, c, skip, src)
		case "context_bound":
			b.scalaTypeRefs(id, c.ChildByFieldName("type"), skip, src)
		}
	}
}

// scalaTypeRefs records a reference from id to each type t names, generic
// arguments included. Names in skip give nothing.
func (b *builder) scalaTypeRefs(id string, t *ts.Node, skip map[string]bool, src []byte) {
	if t == nil {
		return
	}
	switch t.Kind() {
	case "type_identifier":
		if name := t.Utf8Text(src); !skip[name] {
			b.ref(id, name, "references", false, line(t))
		}
	case "generic_type":
		if base := t.ChildByFieldName("type"); base != nil && base.Kind() == "type_identifier" {
			b.scalaTypeRefs(id, base, skip, src)
		}
		for i := uint(0); i < t.NamedChildCount(); i++ {
			if args := t.NamedChild(i); args.Kind() == "type_arguments" {
				for j := uint(0); j < args.NamedChildCount(); j++ {
					b.scalaTypeRefs(id, args.NamedChild(j), skip, src)
				}
			}
		}
	case "compound_type", "infix_type", "function_type", "tuple_type", "annotated_type",
		"projected_type", "match_type", "type_case_clause":
		for i := uint(0); i < t.NamedChildCount(); i++ {
			b.scalaTypeRefs(id, t.NamedChild(i), skip, src)
		}
	}
}
