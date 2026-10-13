package extract

import (
	"strings"

	tsrust "github.com/dobbo-ca/gositter/grammars/rust"
	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractRust pulls functions, types (struct/enum/union/trait/type alias),
// inherent/trait impl methods, `use` imports, and call edges out of a .rs file.
// Items nested in `mod` blocks are descended into so module-organised crates
// still surface their definitions.
func extractRust(rel string, src []byte) Result {
	root, done := parseRoot(src, tsrust.Language())
	defer done()
	b := newBuilder(rel)

	b.rsLocal = map[string]bool{}
	walk(root, func(cv ts.Node) bool {
		switch cv.Kind() {
		case "struct_item", "enum_item", "trait_item":
			b.rsLocal[fieldText(&cv, "name", src)] = true
		}
		return true
	})
	b.rustItems(root, src)
	// An impl of a type declared elsewhere has no node to start from.
	refs := b.res.TypeRefs[:0]
	for _, r := range b.res.TypeRefs {
		if b.seen[r.FromID] {
			refs = append(refs, r)
		}
	}
	b.res.TypeRefs = refs
	return b.res
}

// rustBuiltin lists the prelude and std names that name no type of the corpus.
var rustBuiltin = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`String str Option Result Vec VecDeque Box Rc Arc Weak
		RefCell Cell Cow Pin HashMap HashSet BTreeMap BTreeSet BinaryHeap Path PathBuf OsStr
		OsString CStr CString Duration Instant SystemTime Ordering Range RangeInclusive RangeFrom
		RangeTo RangeFull PhantomData ManuallyDrop NonZeroU8 NonZeroU16 NonZeroU32 NonZeroU64
		NonZeroUsize Some None Ok Err Self Default Clone Copy Debug Display Error From Into
		TryFrom TryInto AsRef AsMut Iterator IntoIterator Extend PartialEq Eq PartialOrd Ord Hash
		Send Sync Sized Drop Deref DerefMut Future Fn FnMut FnOnce`) {
		rustBuiltin[n] = true
	}
}

// rustTParams returns outer plus the type parameter names of the
// type_parameters node n.
func rustTParams(n *ts.Node, outer map[string]bool, src []byte) map[string]bool {
	out := map[string]bool{}
	for k := range outer {
		out[k] = true
	}
	for i := uint(0); n != nil && i < n.NamedChildCount(); i++ {
		p := n.NamedChild(i)
		if p.Kind() != "type_parameter" && p.Kind() != "constrained_type_parameter" && p.Kind() != "optional_type_parameter" {
			continue
		}
		if name := p.ChildByFieldName("name"); name != nil {
			out[name.Utf8Text(src)] = true
		} else if left := p.ChildByFieldName("left"); left != nil {
			out[left.Utf8Text(src)] = true
		}
	}
	return out
}

// rustTypeRefs records a reference from id to the types t names and to its
// generic arguments. rel applies to the outermost name only: a supertrait
// is inherited, what it takes as arguments is referenced. A primitive, a
// prelude name, a type parameter and an associated type name no type.
func (b *builder) rustTypeRefs(id string, t *ts.Node, rel string, src []byte) {
	if t == nil {
		return
	}
	switch t.Kind() {
	case "primitive_type":
	case "type_identifier":
		b.rustRef(id, t.Utf8Text(src), rel, line(t))
	case "scoped_type_identifier":
		if p := t.ChildByFieldName("path"); p != nil && p.Kind() != "bracketed_type" {
			first, _, _ := strings.Cut(p.Utf8Text(src), "::")
			if first != "Self" && !b.rsTParams[first] {
				b.rustRef(id, fieldText(t, "name", src), rel, line(t))
			}
		}
	case "generic_type":
		b.rustTypeRefs(id, t.ChildByFieldName("type"), rel, src)
		b.rustTypeRefs(id, t.ChildByFieldName("type_arguments"), "references", src)
	case "ordered_field_declaration_list":
		// A tuple field is a type, a path, a reference, a tuple or an array.
		for i := uint(0); i < t.NamedChildCount(); i++ {
			switch c := t.NamedChild(i); c.Kind() {
			case "type_identifier", "generic_type", "scoped_type_identifier", "reference_type", "tuple_type", "array_type":
				b.rustTypeRefs(id, c, "references", src)
			}
		}
	case "type_binding":
		// `Item = X` names the associated type, not a type.
		b.rustTypeRefs(id, t.ChildByFieldName("type"), "references", src)
	default:
		for i := uint(0); i < t.NamedChildCount(); i++ {
			b.rustTypeRefs(id, t.NamedChild(i), "references", src)
		}
	}
}

func (b *builder) rustRef(id, name, rel string, loc model.Loc) {
	if !b.rsTParams[name] && (!rustBuiltin[name] || b.rsLocal[name]) {
		b.ref(id, name, rel, false, loc)
	}
}

// rustSignature records the types the parameters and result of fn name.
func (b *builder) rustSignature(id string, fn *ts.Node, src []byte) {
	outer := b.rsTParams
	b.rsTParams = rustTParams(fn.ChildByFieldName("type_parameters"), outer, src)
	if ps := fn.ChildByFieldName("parameters"); ps != nil {
		for i := uint(0); i < ps.NamedChildCount(); i++ {
			if p := ps.NamedChild(i); p.Kind() == "parameter" {
				b.rustTypeRefs(id, p.ChildByFieldName("type"), "references", src)
			}
		}
	}
	b.rustTypeRefs(id, fn.ChildByFieldName("return_type"), "references", src)
	b.rsTParams = outer
}

// rustItems handles each item directly under n (a source_file or a mod's
// declaration_list), recursing into nested modules.
func (b *builder) rustItems(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "function_item":
			b.rustFunc(c, src)
		case "struct_item", "enum_item", "union_item", "trait_item", "type_item":
			b.rustType(c, src)
		case "impl_item":
			b.rustImpl(c, src)
		case "use_declaration":
			b.rustUse(c, src)
		case "mod_item":
			if body := c.ChildByFieldName("body"); body != nil {
				b.rustItems(body, src)
			} else if name := fieldText(c, "name", src); name != "" {
				// `mod x;` loads x.rs, so the file depends on it.
				b.imp("self::"+name, line(c))
			}
		}
	}
}

func (b *builder) rustFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := b.id(name)
	b.def(id, name, name+"()", line(n))
	b.rustSignature(id, n, src)
	b.rustCalls(n.ChildByFieldName("body"), id, src)
}

func (b *builder) rustType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := b.id(name)
	b.def(id, name, name, line(n))
	outer := b.rsTParams
	b.rsTParams = rustTParams(n.ChildByFieldName("type_parameters"), outer, src)
	if n.Kind() != "trait_item" {
		b.rustTypeRefs(id, n.ChildByFieldName("body"), "references", src)
	} else {
		bounds := n.ChildByFieldName("bounds")
		for i := uint(0); bounds != nil && i < bounds.NamedChildCount(); i++ {
			b.rustTypeRefs(id, bounds.NamedChild(i), "inherits", src)
		}
	}
	b.rsTParams = outer
}

// rustImpl records the methods of an `impl Type { ... }` (or `impl Trait for
// Type`) block, scoping each method under the implemented type's name.
func (b *builder) rustImpl(n *ts.Node, src []byte) {
	typeName := rustTypeName(n.ChildByFieldName("type"), src)
	if typeName == "" {
		return
	}
	outer := b.rsTParams
	b.rsTParams = rustTParams(n.ChildByFieldName("type_parameters"), outer, src)
	defer func() { b.rsTParams = outer }()
	b.rustTypeRefs(b.id(typeName), n.ChildByFieldName("trait"), "implements", src)
	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		if m.Kind() != "function_item" {
			continue
		}
		mname := fieldText(m, "name", src)
		if mname == "" {
			continue
		}
		mid := b.id(typeName, mname)
		b.addNode(mid, typeName+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: b.id(typeName), Target: mid, Relation: model.RelContains,
			Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(m),
		})
		// Register under the bare method name so `x.method()` call sites resolve.
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.rustSignature(mid, m, src)
		b.rustCalls(m.ChildByFieldName("body"), mid, src)
	}
}

// rustUse records the module path of a `use` declaration as an import. Grouped
// (`use a::{b, c}`) and aliased uses record the leading path; the names bound
// under it are kept too, since `use crate::a` names a module.
func (b *builder) rustUse(n *ts.Node, src []byte) {
	arg := n.ChildByFieldName("argument")
	if arg == nil {
		return
	}
	p := arg.ChildByFieldName("path")
	if p == nil {
		b.imp(arg.Utf8Text(src), line(n))
		return
	}
	b.imp(p.Utf8Text(src), line(n))
	imp := &b.res.Imps[len(b.res.Imps)-1]
	switch arg.Kind() {
	case "scoped_identifier":
		imp.Names = []string{fieldText(arg, "name", src)}
	case "scoped_use_list":
		list := arg.ChildByFieldName("list")
		for i := uint(0); list != nil && i < list.ChildCount(); i++ {
			if c := list.Child(i); c.Kind() == "identifier" || c.Kind() == "scoped_identifier" {
				imp.Names = append(imp.Names, c.Utf8Text(src))
			}
		}
	}
}

// rustTypeName returns a bare type name for impl targets, ignoring generic
// arguments and references (e.g. "Server" for `impl<T> Server<T>`).
func rustTypeName(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "type_identifier", "identifier":
		return n.Utf8Text(src)
	case "generic_type":
		return rustTypeName(n.ChildByFieldName("type"), src)
	case "scoped_type_identifier":
		return rustTypeName(n.ChildByFieldName("name"), src)
	case "reference_type":
		return rustTypeName(n.ChildByFieldName("type"), src)
	}
	// Fall back to the first type/identifier descendant.
	var name string
	walk(n, func(cv ts.Node) bool {
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

var rustCallKinds = tsrust.Language().KindMask("call_expression", "macro_invocation")

// rustCalls walks a function body and records each call site. Direct calls
// (`f()`), path calls (`mod::f()`), and method calls (`x.f()`) all record the
// trailing name; macro invocations (`f!()`) record the macro name.
func (b *builder) rustCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walkKinds(body, rustCallKinds, func(cv ts.Node) bool {
		c := &cv
		switch c.Kind() {
		case "call_expression":
			fn := c.ChildByFieldName("function")
			if fn == nil {
				return true
			}
			switch fn.Kind() {
			case "identifier":
				b.call(callerID, fn.Utf8Text(src), line(c))
			case "scoped_identifier":
				if name := fn.ChildByFieldName("name"); name != nil {
					b.callRecv(callerID, name.Utf8Text(src), recvText(fn, name, src), line(c))
				}
			case "field_expression":
				if f := fn.ChildByFieldName("field"); f != nil {
					b.callRecv(callerID, f.Utf8Text(src), recvText(fn, f, src), line(c))
				}
			}
		case "macro_invocation":
			m := c.ChildByFieldName("macro")
			if m == nil {
				return true
			}
			if m.Kind() == "scoped_identifier" {
				m = m.ChildByFieldName("name")
			}
			if m != nil {
				b.call(callerID, m.Utf8Text(src), line(c))
			}
		}
		return true
	})
}
