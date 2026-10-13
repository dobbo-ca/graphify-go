package extract

import (
	"strings"

	tskotlin "github.com/dobbo-ca/gositter/grammars/kotlin"
	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractKotlin pulls top-level functions, types (class/interface/enum/object)
// with their methods, imports, and call edges out of a .kt/.kts file. Kotlin
// interfaces and enum classes are both `class_declaration` nodes; objects are
// `object_declaration`. Methods nested in a type body are scoped under the type.
func extractKotlin(rel string, src []byte) Result {
	root, done := parseRoot(src, tskotlin.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		n := root.Child(i)
		switch n.Kind() {
		case "function_declaration":
			b.ktFunc(n, src)
		case "class_declaration", "object_declaration":
			b.ktType(n, src)
		case "import":
			b.ktImport(n, src)
		}
	}
	return b.res
}

func (b *builder) ktFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := b.id(name)
	b.def(id, name, name+"()", line(n))
	b.ktSignature(id, n, nil, src)
	b.ktCalls(ktBody(n), id, src)
}

// ktType records a class/interface/enum/object and the methods declared in its
// body. Both `class_body` and `enum_class_body` hold `function_declaration`
// members directly (the grammar inlines the member-declaration supertype).
func (b *builder) ktType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	typeID := b.id(name)
	b.def(typeID, name, name, line(n))
	skip := listTypeParams(n, "type_parameters", ktNestedTypes(n, src, map[string]bool{}), src)
	b.ktSupertypes(typeID, n, skip, src)
	b.ktAnnotations(typeID, n, true, src)
	b.ktClassParams(typeID, n, skip, src)

	for i := uint(0); i < n.ChildCount(); i++ {
		body := n.Child(i)
		if body.Kind() != "class_body" && body.Kind() != "enum_class_body" {
			continue
		}
		for j := uint(0); j < body.ChildCount(); j++ {
			m := body.Child(j)
			switch m.Kind() {
			case "function_declaration":
				b.ktMember(typeID, name, m, skip, src)
			case "property_declaration":
				b.ktProperty(typeID, m, skip, src)
			case "enum_entry":
				b.ktEnumEntry(typeID, m, skip, src)
			case "companion_object":
				// Its properties belong to the enclosing type.
				for k := uint(0); k < m.NamedChildCount(); k++ {
					cb := m.NamedChild(k)
					for l := uint(0); cb.Kind() == "class_body" && l < cb.NamedChildCount(); l++ {
						if p := cb.NamedChild(l); p.Kind() == "property_declaration" {
							b.ktProperty(typeID, p, skip, src)
						}
					}
				}
			}
		}
	}
}

// ktMember records a method scoped under ownerID (a type or an enum entry with
// an anonymous body), labelled ownerName.method(). It is registered under the
// bare method name so `x.method()` call sites resolve.
func (b *builder) ktMember(ownerID, ownerName string, m *ts.Node, skip map[string]bool, src []byte) {
	mname := fieldText(m, "name", src)
	if mname == "" {
		return
	}
	mid := idutil.MakeID(ownerID, mname)
	b.addNode(mid, ownerName+"."+mname+"()", line(m))
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: ownerID, Target: mid, Relation: model.RelContains,
		Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(m),
	})
	b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
	b.ktSignature(mid, m, skip, src)
	b.ktCalls(ktBody(m), mid, src)
}

// ktEnumEntry records an enum entry as a `case_of` the enum type. An entry with
// an anonymous class body (`GREEN { fun greet(){} }`) descends so the body's
// methods attach to the entry rather than being dropped.
func (b *builder) ktEnumEntry(typeID string, n *ts.Node, skip map[string]bool, src []byte) {
	cname := ktEntryName(n, src)
	if cname == "" {
		return
	}
	cid := idutil.MakeID(typeID, cname)
	b.addNode(cid, cname, line(n))
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: typeID, Target: cid, Relation: model.RelCaseOf,
		Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(n),
	})
	for i := uint(0); i < n.ChildCount(); i++ {
		body := n.Child(i)
		if body.Kind() != "class_body" {
			continue
		}
		for j := uint(0); j < body.ChildCount(); j++ {
			if m := body.Child(j); m.Kind() == "function_declaration" {
				b.ktMember(cid, cname, m, skip, src)
			}
		}
	}
}

// ktEntryName returns an enum entry's name: its first identifier child. The
// grammar exposes it as a plain `simple_identifier`/`identifier` rather than a
// named field.
func ktEntryName(n *ts.Node, src []byte) string {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == "simple_identifier" || c.Kind() == "identifier" {
			return c.Utf8Text(src)
		}
	}
	return ""
}

// ktImport records the dotted path of an `import` directive as an import spec.
// The qualified path (e.g. kotlin.math.sqrt) is a bare specifier, so Resolve
// turns it into an external dependency node.
func (b *builder) ktImport(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() == "qualified_identifier" || c.Kind() == "identifier" {
			b.imp(c.Utf8Text(src), line(n))
			return
		}
	}
}

// ktBody returns the `function_body` child of a function_declaration, or nil for
// abstract/interface methods that have no body.
func ktBody(fn *ts.Node) *ts.Node {
	for i := uint(0); i < fn.ChildCount(); i++ {
		if c := fn.Child(i); c.Kind() == "function_body" {
			return c
		}
	}
	return nil
}

var ktCallKinds = tskotlin.Language().KindMask("call_expression")

// ktCalls walks a function body and records each call site. Direct calls (`f()`)
// record the identifier; navigation calls (`x.f()`) record the trailing member
// name.
func (b *builder) ktCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walkKinds(body, ktCallKinds, func(cv ts.Node) bool {
		c := &cv
		callee := c.Child(0)
		if callee == nil {
			return true
		}
		switch callee.Kind() {
		case "identifier":
			b.call(callerID, callee.Utf8Text(src), line(c))
		case "navigation_expression":
			if nm := ktNavName(callee); nm != nil {
				b.callRecv(callerID, nm.Utf8Text(src), recvText(callee, nm, src), line(c))
			}
		}
		return true
	})
}

// ktNavName returns the trailing member name of a navigation_expression
// (`a.b.c` -> "c"): the last identifier child.
func ktNavName(n *ts.Node) *ts.Node {
	var name *ts.Node
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == "identifier" {
			name = c
		}
	}
	return name
}

// ktBuiltin lists the kotlin.* type names that give no reference; upstream
// adds the java.* names (javaBuiltin).
var ktBuiltin = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`Any Unit Nothing Boolean Byte Short Int Long Float Double
		Char String CharSequence Number Comparable Enum Annotation Pair Triple Lazy Function
		Throwable Exception RuntimeException Error IllegalArgumentException
		IllegalStateException NullPointerException IndexOutOfBoundsException
		ClassCastException NumberFormatException ArithmeticException
		UnsupportedOperationException NoSuchElementException ConcurrentModificationException
		StackOverflowError OutOfMemoryError AssertionError InterruptedException
		Array List MutableList ArrayList Set MutableSet HashSet LinkedHashSet Map MutableMap
		HashMap LinkedHashMap Collection MutableCollection Iterable MutableIterable Iterator
		MutableIterator ListIterator MutableListIterator Sequence Comparator Regex MatchResult
		StringBuilder`) {
		ktBuiltin[n] = true
	}
}

// ktNestedTypes adds the names of the types nested in n to out. They have
// no node, so a ref to one must not bind to a namesake elsewhere.
func ktNestedTypes(n *ts.Node, src []byte, out map[string]bool) map[string]bool {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		body := n.NamedChild(i)
		if body.Kind() != "class_body" && body.Kind() != "enum_class_body" && body.Kind() != "companion_object" {
			continue
		}
		ktNestedTypes(body, src, out)
		for j := uint(0); j < body.NamedChildCount(); j++ {
			if c := body.NamedChild(j); c.Kind() == "class_declaration" || c.Kind() == "object_declaration" {
				out[fieldText(c, "name", src)] = true
				ktNestedTypes(c, src, out)
			}
		}
	}
	return out
}

// ktUserType returns the name a user_type ends in (`a.b.C<D>` -> C).
func ktUserType(t *ts.Node, src []byte) string {
	name := ""
	for i := uint(0); i < t.NamedChildCount(); i++ {
		if c := t.NamedChild(i); c.Kind() == "identifier" {
			name = c.Utf8Text(src)
		}
	}
	return name
}

// ktTypeRefs records a reference from id to each type t names, generic
// arguments included. Builtins and names in skip give nothing.
func (b *builder) ktTypeRefs(id string, t *ts.Node, skip map[string]bool, src []byte) {
	if t == nil {
		return
	}
	switch t.Kind() {
	case "integral_literal", "boolean_literal":
	case "user_type":
		if name := ktUserType(t, src); !skip[name] && !ktBuiltin[name] && !javaBuiltin[name] {
			b.ref(id, name, "references", false, line(t))
		}
		b.ktTypeArgs(id, t, skip, src)
	default:
		for i := uint(0); i < t.NamedChildCount(); i++ {
			if c := t.NamedChild(i); c.Kind() != "identifier" {
				b.ktTypeRefs(id, c, skip, src)
			}
		}
	}
}

// ktTypeArgs records the types in the type_arguments of the user_type t.
func (b *builder) ktTypeArgs(id string, t *ts.Node, skip map[string]bool, src []byte) {
	for i := uint(0); i < t.NamedChildCount(); i++ {
		args := t.NamedChild(i)
		if args.Kind() != "type_arguments" {
			continue
		}
		for j := uint(0); j < args.NamedChildCount(); j++ {
			a := args.NamedChild(j)
			if a.Kind() != "type_projection" {
				b.ktTypeRefs(id, a, skip, src)
				continue
			}
			for k := uint(0); k < a.NamedChildCount(); k++ {
				b.ktTypeRefs(id, a.NamedChild(k), skip, src)
			}
		}
	}
}

// ktTypeOf returns the first type child of n.
func ktTypeOf(n *ts.Node) *ts.Node {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		switch c := n.NamedChild(i); c.Kind() {
		case "user_type", "nullable_type", "type_reference":
			return c
		}
	}
	return nil
}

// ktSupertypes records the delegation specifiers of a type: a constructor
// call is a superclass, anything else an interface. A builtin base still
// gets a stub.
func (b *builder) ktSupertypes(typeID string, n *ts.Node, skip map[string]bool, src []byte) {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		list := n.NamedChild(i)
		if list.Kind() != "delegation_specifiers" {
			continue
		}
		for j := uint(0); j < list.NamedChildCount(); j++ {
			spec := list.NamedChild(j)
			relation := "implements"
			var ut *ts.Node
			for k := uint(0); k < spec.NamedChildCount() && ut == nil; k++ {
				switch c := spec.NamedChild(k); c.Kind() {
				case "constructor_invocation":
					relation = "inherits"
					ut = ktFirst(c, "user_type")
				case "user_type":
					ut = c
				case "explicit_delegation":
					ut = ktFirst(c, "user_type")
				}
			}
			if ut == nil {
				continue
			}
			b.ref(typeID, ktUserType(ut, src), relation, false, line(ut))
			b.ktTypeArgs(typeID, ut, skip, src)
		}
	}
}

// ktFirst returns the first named child of n of the given kind.
func ktFirst(n *ts.Node, kind string) *ts.Node {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c.Kind() == kind {
			return c
		}
	}
	return nil
}

// ktAnnotations records the annotations in the modifiers of declaration n,
// and with lits the class literals in their arguments, once per name.
func (b *builder) ktAnnotations(id string, n *ts.Node, lits bool, src []byte) {
	mods := ktFirst(n, "modifiers")
	if mods == nil {
		return
	}
	seen := map[string]bool{}
	for i := uint(0); i < mods.NamedChildCount(); i++ {
		a := mods.NamedChild(i)
		if a.Kind() != "annotation" {
			continue
		}
		for j := uint(0); j < a.NamedChildCount(); j++ {
			ut, ci := a.NamedChild(j), (*ts.Node)(nil)
			if ut.Kind() == "constructor_invocation" {
				ci, ut = ut, ktFirst(ut, "user_type")
			}
			if ut == nil || ut.Kind() != "user_type" {
				continue
			}
			if name := ktUserType(ut, src); name != "" && !seen[name] {
				seen[name] = true
				b.ref(id, name, "references", false, line(a))
			}
			if lits && ci != nil {
				if args := ktFirst(ci, "value_arguments"); args != nil {
					b.ktClassLiterals(id, args, seen, src)
				}
			}
		}
	}
}

// ktClassLiterals records the types of the `T::class` under n not in seen.
func (b *builder) ktClassLiterals(id string, n *ts.Node, seen map[string]bool, src []byte) {
	if n.Kind() == "navigation_expression" && n.ChildCount() >= 3 {
		var lhs *ts.Node
		colons, class := false, false
		for i := uint(0); i < n.ChildCount(); i++ {
			switch c := n.Child(i); {
			case c.Kind() == "::":
				colons = true
			case colons && c.Kind() == "identifier" && c.Utf8Text(src) == "class":
				class = true
			case !colons && c.IsNamed():
				lhs = c
			}
		}
		if colons && class && lhs != nil {
			text := lhs.Utf8Text(src)
			name := text[strings.LastIndex(text, ".")+1:]
			if name != "" && !seen[name] && !ktBuiltin[name] && !javaBuiltin[name] {
				seen[name] = true
				b.ref(id, name, "references", false, line(n))
			}
			return
		}
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		b.ktClassLiterals(id, n.NamedChild(i), seen, src)
	}
}

// ktClassParams records the types and annotations of the val and var
// parameters of a primary constructor.
func (b *builder) ktClassParams(typeID string, n *ts.Node, skip map[string]bool, src []byte) {
	pc := ktFirst(n, "primary_constructor")
	if pc == nil {
		return
	}
	list := ktFirst(pc, "class_parameters")
	for i := uint(0); list != nil && i < list.NamedChildCount(); i++ {
		p := list.NamedChild(i)
		if p.Kind() != "class_parameter" {
			continue
		}
		field := false
		for j := uint(0); j < p.ChildCount(); j++ {
			if k := p.Child(j).Kind(); k == "val" || k == "var" {
				field = true
			}
		}
		if t := ktTypeOf(p); field && t != nil {
			b.ktTypeRefs(typeID, t, skip, src)
			b.ktAnnotations(typeID, p, true, src)
		}
	}
}

// ktProperty records the type and annotations of a property.
func (b *builder) ktProperty(typeID string, p *ts.Node, skip map[string]bool, src []byte) {
	t := ktTypeOf(p)
	if v := ktFirst(p, "variable_declaration"); v != nil {
		t = ktTypeOf(v)
	}
	b.ktTypeRefs(typeID, t, skip, src)
	b.ktAnnotations(typeID, p, true, src)
}

// ktSignature records the parameter and return types and the annotations of
// function fn; skip holds the type parameters of the enclosing type.
func (b *builder) ktSignature(id string, fn *ts.Node, skip map[string]bool, src []byte) {
	skip = listTypeParams(fn, "type_parameters", skip, src)
	afterParams, afterColon := false, false
	for i := uint(0); i < fn.ChildCount(); i++ {
		c := fn.Child(i)
		switch {
		case c.Kind() == "function_value_parameters":
			afterParams = true
			for j := uint(0); j < c.NamedChildCount(); j++ {
				if p := c.NamedChild(j); p.Kind() == "parameter" {
					b.ktTypeRefs(id, ktTypeOf(p), skip, src)
				}
			}
		case afterParams && c.Kind() == ":":
			afterColon = true
		case afterColon && c.IsNamed():
			b.ktTypeRefs(id, c, skip, src)
			afterColon = false
			afterParams = false
		}
	}
	b.ktAnnotations(id, fn, false, src)
}
