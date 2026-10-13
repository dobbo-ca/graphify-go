package extract

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"

	tspy "github.com/dobbo-ca/gositter/grammars/python"
	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractPython pulls functions, classes (+ methods), imports, and call edges
// out of a .py file. Top-level functions/classes become definitions; methods
// are nested under their class. Calls are recorded by the called name and
// resolved against the corpus by Resolve.
func extractPython(rel string, src []byte) Result {
	root, done := parseRoot(src, tspy.Language())
	defer done()
	b := newBuilder(rel)
	b.pyMods = pyModules(root, src)
	b.pyTypeVars = pyTypeVarNames(root, src)

	for i := uint(0); i < root.ChildCount(); i++ {
		b.pyStatement(root.Child(i), src)
	}
	// The names a module-level def, class or from-import of the file binds.
	b.pyOwn = map[string]bool{}
	for _, d := range b.res.Defs {
		if d.ID == b.id(d.Name) {
			b.pyOwn[d.Name] = true
		}
	}
	for _, a := range b.res.ImportAliases {
		if !a.InFunc {
			b.pyOwn[a.Local] = true
		}
	}
	dyn := pyDynamic(root, src)
	for i := range b.pyBodies {
		b.pyCalls(&b.pyBodies[i].fn, b.pyBodies[i].id, !dyn && !b.pyBodies[i].method, src)
	}
	b.pyRationale(root, src)
	return b.res
}

// pyStatement handles one module-level statement, unwrapping decorators first.
func (b *builder) pyStatement(n *ts.Node, src []byte) {
	switch n.Kind() {
	case "decorated_definition":
		if d := n.ChildByFieldName("definition"); d != nil && d.Kind() == "function_definition" {
			b.pyFunc(d, n, src)
		} else if d != nil && d.Kind() == "class_definition" {
			b.pyClass(d, n, src)
		}
	case "function_definition":
		b.pyFunc(n, nil, src)
	case "class_definition":
		b.pyClass(n, nil, src)
	case "import_statement", "import_from_statement", "if_statement", "try_statement", "with_statement",
		"for_statement", "while_statement", "match_statement":
		b.pyNestedImports(n, src, false)
	}
}

// pyNestedImports records the imports under n, including ones guarded by an
// if/try/with block. Imports in the body of `if TYPE_CHECKING:` (or
// `<x>.TYPE_CHECKING`) never run, so they are type-only; else branches are not.
func (b *builder) pyNestedImports(n *ts.Node, src []byte, typeOnly bool) {
	guard := false
	switch n.Kind() {
	case "import_statement", "import_from_statement":
		b.pyImports(n, src, typeOnly)
		return
	case "function_definition", "class_definition", "decorated_definition":
		return
	case "if_statement", "elif_clause":
		cond := n.ChildByFieldName("condition")
		if cond != nil && cond.Kind() == "attribute" {
			cond = cond.ChildByFieldName("attribute")
		}
		guard = cond != nil && cond.Kind() == "identifier" && cond.Utf8Text(src) == "TYPE_CHECKING"
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		b.pyNestedImports(n.Child(i), src, typeOnly || guard && n.FieldNameForChild(uint32(i)) == "consequence")
	}
}

// pyFunc and pyClass take the decorated_definition around n, or nil.
func (b *builder) pyFunc(n, deco *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := b.id(name)
	b.def(id, name, name+"()", line(n))
	b.pyRefs(id, n, deco, src)
	b.pyBodies = append(b.pyBodies, pyBody{*n, id, false})
}

func (b *builder) pyClass(n, deco *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	classID := b.id(name)
	b.def(classID, name, name, line(n))
	b.pyDecorators(classID, deco, src)
	b.pySuperclasses(classID, n, src)

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		var mdeco *ts.Node
		if m.Kind() == "decorated_definition" {
			mdeco = m
			m = m.ChildByFieldName("definition")
			if m == nil {
				continue
			}
		}
		if m.Kind() != "function_definition" {
			pyImportNames(m, src, func(local, val string) { b.pyInFuncAlias(local, val, line(m)) })
			continue
		}
		mname := fieldText(m, "name", src)
		if mname == "" {
			continue
		}
		mid := b.id(name, mname)
		b.addNode(mid, name+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: classID, Target: mid, Relation: model.RelContains,
			Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(m),
		})
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.pyRefs(mid, m, mdeco, src)
		b.pyBodies = append(b.pyBodies, pyBody{*m, mid, true})
	}
}

// pyImports records each imported module. `import a.b` and `from a.b import c`
// both record the module path a.b; a from-import also keeps its imported names,
// since `from . import b` names a module. Resolve binds them to corpus files.
func (b *builder) pyImports(n *ts.Node, src []byte, typeOnly bool) {
	switch n.Kind() {
	case "import_statement":
		for i := uint(0); i < n.ChildCount(); i++ {
			c := n.Child(i)
			switch c.Kind() {
			case "dotted_name":
				b.impTyped(c.Utf8Text(src), line(n), typeOnly)
			case "aliased_import":
				if name := c.ChildByFieldName("name"); name != nil {
					b.impTyped(name.Utf8Text(src), line(n), typeOnly)
				}
			}
		}
	case "import_from_statement":
		mod := n.ChildByFieldName("module_name")
		if mod == nil {
			return
		}
		b.impTyped(mod.Utf8Text(src), line(n), typeOnly)
		imp := &b.res.Imps[len(b.res.Imps)-1]
		for i := uint(0); i < n.ChildCount(); i++ {
			if n.FieldNameForChild(uint32(i)) != "name" {
				continue
			}
			c := n.Child(i)
			if c.Kind() == "aliased_import" {
				c = c.ChildByFieldName("name")
			}
			if c != nil {
				imp.Names = append(imp.Names, c.Utf8Text(src))
			}
		}
		b.pyImportAliases(n, mod, src)
	}
}

// pyImportAliases captures `from M import N [as L]` evidence for the
// import-guided call resolver. stem is the final component of the module name;
// each imported name records local -> imported under that stem. `import *`
// (wildcard_import, no name field) carries no usable alias and is skipped.
func (b *builder) pyImportAliases(n, mod *ts.Node, src []byte) {
	stem := pyModuleStem(mod, src) // "" for `from . import N`
	loc := line(n)
	for i := uint(0); i < n.ChildCount(); i++ {
		if n.FieldNameForChild(uint32(i)) != "name" {
			continue
		}
		c := n.Child(i)
		var local, imported string
		switch c.Kind() {
		case "dotted_name":
			imported, local = c.Utf8Text(src), c.Utf8Text(src)
		case "aliased_import":
			name, alias := c.ChildByFieldName("name"), c.ChildByFieldName("alias")
			if name == nil || alias == nil {
				continue
			}
			imported, local = name.Utf8Text(src), alias.Utf8Text(src)
		default:
			continue
		}
		if local == "" || imported == "" {
			continue
		}
		b.res.ImportAliases = append(b.res.ImportAliases, ImportAlias{
			Local: local, Imported: imported, ModuleStem: stem, Spec: mod.Utf8Text(src), Loc: loc,
		})
	}
}

// pyModuleStem returns the final component of a `from ... import` module name,
// mirroring upstream _module_stem: `util.math` -> `math`, `.helper` -> `helper`.
func pyModuleStem(mod *ts.Node, src []byte) string { return pySpecStem(mod.Utf8Text(src)) }

func pySpecStem(spec string) string {
	text := strings.Trim(spec, ".")
	if i := strings.LastIndex(text, "."); i >= 0 {
		return text[i+1:]
	}
	return text
}

// pyBuiltins are the functions and types of the Python builtins module. A bare
// call of one names the builtin unless its file defines or imports the name.
// exit and quit are those of the site module.
var pyBuiltins = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`abs aiter all anext any ascii bin bool breakpoint bytearray bytes
		callable chr classmethod compile complex delattr dict dir divmod enumerate eval exec filter
		float format frozenset getattr globals hasattr hash help hex id input int isinstance
		issubclass iter len list locals map max memoryview min next object oct open ord pow print
		property range repr reversed round set setattr slice sorted staticmethod str sum super tuple
		type vars zip __import__ exit quit`) {
		pyBuiltins[n] = true
	}
}

// pyClassName matches a bare capitalized name, the only annotation or callee
// taken as a class.
var pyClassName = regexp.MustCompile(`^[A-Z]\w*$`)

// pyLocalTyper returns the visitor that, run over every node of a function,
// maps its locals to their class in types, from a parameter
// annotation (`c: Client`) or a constructor binding (`c = Client()`). A local
// also bound by anything else (assignment, unpacking, for, with, except, `:=`)
// maps to "".
func pyLocalTyper(types map[string]string, src []byte) func(c *ts.Node) bool {
	// untype clears every name a binding target rebinds.
	var untype func(n *ts.Node)
	untype = func(n *ts.Node) {
		if n == nil {
			return
		}
		switch n.Kind() {
		case "identifier":
			bindType(types, n.Utf8Text(src), "")
		case "pattern_list", "tuple_pattern", "list_pattern", "list_splat_pattern", "as_pattern_target":
			for i := uint(0); i < n.NamedChildCount(); i++ {
				untype(n.NamedChild(i))
			}
		}
	}
	return func(c *ts.Node) bool {
		var name *ts.Node
		switch c.Kind() {
		case "typed_parameter":
			name = c.NamedChild(0)
		case "typed_default_parameter":
			name = c.ChildByFieldName("name")
		case "assignment":
			name = c.ChildByFieldName("left")
			if name != nil && name.Kind() != "identifier" {
				untype(name)
			}
		case "for_statement", "for_in_clause":
			untype(c.ChildByFieldName("left"))
		case "as_pattern_target":
			untype(c)
		case "named_expression":
			untype(c.ChildByFieldName("name"))
		}
		if name == nil || name.Kind() != "identifier" {
			return true
		}
		typ := fieldText(c, "type", src)
		if r := c.ChildByFieldName("right"); typ == "" && r != nil {
			if r.Kind() == "none" {
				return true
			}
			if r.Kind() == "call" {
				typ = fieldText(r, "function", src)
			}
		}
		if !pyClassName.MatchString(typ) {
			typ = ""
		}
		bindType(types, name.Utf8Text(src), typ)
		return true
	}
}

// pyModules maps each local name bound by exactly one module-level import,
// and bound nowhere else at module scope, to what it names (pyImportNames).
func pyModules(root *ts.Node, src []byte) map[string]string {
	mods, bound := map[string]string{}, map[string]bool{}
	for i := uint(0); i < root.ChildCount(); i++ {
		st := root.Child(i)
		if !pyImportNames(st, src, func(local, val string) {
			if _, dup := mods[local]; dup || val == "" {
				bound[local] = true
			}
			mods[local] = val
		}) {
			pyBinds(st, src, false, bound)
		}
	}
	// `from m import *` can bind any name.
	if bound["*"] {
		return nil
	}
	for name := range bound {
		delete(mods, name)
	}
	return mods
}

// pyImportNames calls fn for each name the import statement st binds, with
// what it names: "a.b" for the module of `import a.b as x`, "a.b:n" for the n
// of `from a.b import n`, and "" for `import a.b`, which binds a alone. It
// returns false, and calls nothing, for another statement or a star import.
func pyImportNames(st *ts.Node, src []byte, fn func(local, val string)) bool {
	from := ""
	switch st.Kind() {
	case "import_statement":
	case "import_from_statement":
		from = fieldText(st, "module_name", src) + ":"
		for i := uint(0); i < st.ChildCount(); i++ {
			if st.Child(i).Kind() == "wildcard_import" {
				return false
			}
		}
	default:
		return false
	}
	for i := uint(0); i < st.ChildCount(); i++ {
		if st.FieldNameForChild(uint32(i)) != "name" {
			continue
		}
		c := st.Child(i)
		switch c.Kind() {
		case "dotted_name":
			// `import a.b` binds a, which is not the module imported.
			local, val := c.NamedChild(0).Utf8Text(src), ""
			if c.NamedChildCount() == 1 || from != "" {
				local = c.Utf8Text(src)
				val = from + local
			}
			fn(local, val)
		case "aliased_import":
			fn(fieldText(c, "alias", src), from+fieldText(c, "name", src))
		}
	}
	return true
}

// The kinds pyBind acts on, then those of the pyCalls walk: pyBind's, the
// local typer's and the call itself.
var (
	pyGlobalKinds = tspy.Language().KindMask("global_statement")
	pyBindKinds   = tspy.Language().KindMask("function_definition", "class_definition", "lambda",
		"parameters", "lambda_parameters", "assignment", "augmented_assignment", "for_statement",
		"for_in_clause", "named_expression", "as_pattern", "global_statement", "nonlocal_statement",
		"delete_statement", "case_pattern", "import_statement", "import_from_statement", "wildcard_import")
	pyCallKinds = pyBindKinds | tspy.Language().KindMask("typed_parameter", "typed_default_parameter",
		"as_pattern_target", "call")
	pyDefKinds = tspy.Language().KindMask("function_definition", "class_definition")
	pyDynKinds = tspy.Language().KindMask("global_statement", "nonlocal_statement", "identifier")
)

// pyBinds adds the names bound under n to bound. Unless deep, the body of a
// def, class or lambda is skipped: it is another scope, and only a `global`
// reaches out of it.
func pyBinds(n *ts.Node, src []byte, deep bool, bound map[string]bool) {
	walkKinds(n, pyBindKinds, func(c ts.Node) bool { return pyBind(&c, src, deep, bound) })
}

// pyBind is pyBinds for the single node c; false means skip its children.
func pyBind(c *ts.Node, src []byte, deep bool, bound map[string]bool) bool {
	switch c.Kind() {
	case "function_definition", "class_definition", "lambda":
		pyTargets(c.ChildByFieldName("name"), src, bound)
		if !deep {
			walkKinds(c, pyGlobalKinds, func(gv ts.Node) bool {
				pyTargets(&gv, src, bound)
				return true
			})
			return false
		}
	case "parameters", "lambda_parameters":
		for i := uint(0); i < c.NamedChildCount(); i++ {
			p := c.NamedChild(i)
			if name := p.ChildByFieldName("name"); name != nil {
				p = name
			} else if p.Kind() == "typed_parameter" {
				p = p.NamedChild(0)
			}
			pyTargets(p, src, bound)
		}
	case "assignment", "augmented_assignment", "for_statement", "for_in_clause":
		pyTargets(c.ChildByFieldName("left"), src, bound)
	case "named_expression":
		pyTargets(c.ChildByFieldName("name"), src, bound)
	case "as_pattern":
		pyTargets(c.ChildByFieldName("alias"), src, bound)
	case "global_statement", "nonlocal_statement", "delete_statement", "case_pattern":
		pyTargets(c, src, bound)
	case "import_statement", "import_from_statement":
		for i := uint(0); i < c.ChildCount(); i++ {
			if c.FieldNameForChild(uint32(i)) != "name" {
				continue
			}
			// `import a.b` binds a alone.
			name := c.Child(i)
			if alias := name.ChildByFieldName("alias"); alias != nil {
				name = alias
			} else {
				name = name.NamedChild(0)
			}
			pyTargets(name, src, bound)
		}
	case "wildcard_import":
		bound["*"] = true
	}
	return true
}

// pyTargets adds the names an assignment target t rebinds or mutates: the
// identifiers of a pattern, and the root object of an attribute or subscript.
func pyTargets(t *ts.Node, src []byte, bound map[string]bool) {
	if t == nil {
		return
	}
	switch t.Kind() {
	case "identifier":
		bound[t.Utf8Text(src)] = true
	case "attribute":
		pyTargets(t.ChildByFieldName("object"), src, bound)
	case "subscript":
		pyTargets(t.ChildByFieldName("value"), src, bound)
	default:
		for i := uint(0); i < t.NamedChildCount(); i++ {
			pyTargets(t.NamedChild(i), src, bound)
		}
	}
}

// pyScope is the function or class the pyCalls walk is inside. A class adds
// to the ids below it alone: fn and the body are those of the function around.
// outer is the function a def is made in, which runs its defaults; local marks
// a method of a class made in a function, where self is not that of fn.
type pyScope struct {
	id, fn, outer  string
	end            uint
	bodyLo, bodyHi uint
	local          bool
}

// pyBody is a function whose calls pyCalls records once the file's own
// definitions have their ids.
type pyBody struct {
	fn     ts.Node
	id     string
	method bool
}

// pyDynNames are the identifiers upstream takes as a way to rebind a module
// name without an assignment (_python_receiver_shadow_checker).
var pyDynNames = map[string]bool{
	"exec": true, "eval": true, "globals": true, "locals": true, "vars": true, "setattr": true, "delattr": true,
}

// pyDynamic reports whether the file has a global or nonlocal statement or an
// identifier of pyDynNames, anywhere. Upstream then gives no call the node of
// an external module.
func pyDynamic(root *ts.Node, src []byte) bool {
	found := false
	walkKinds(root, pyDynKinds, func(c ts.Node) bool {
		found = found || c.Kind() != "identifier" || pyDynNames[c.Utf8Text(src)]
		return !found
	})
	return found
}

// pyInFuncAlias records the name a from-import inside a function or class
// binds, for the edge from the file to what it names.
func (b *builder) pyInFuncAlias(local, val string, loc model.Loc) {
	if mod, name, ok := strings.Cut(val, ":"); ok {
		b.res.ImportAliases = append(b.res.ImportAliases, ImportAlias{
			Local: local, Imported: name, Spec: mod, Loc: loc, InFunc: true,
		})
	}
}

// pyCalls records each call site in the body of function fn. Direct
// calls (`f()`) record the identifier; attribute calls (`x.f()`) record the
// attribute name, with the receiver's class in place of a typed local. One walk
// of fn collects the local types, the bound names and the call sites. A def
// inside fn is a node the function around it contains, and the caller of the
// calls in its own body. ext is false when no call of fn can name an external
// module: upstream gives that edge to a function of the file alone, and to no
// function of a file pyDynamic holds for.
// ponytail: the types and bound names are one flat scope for fn and the defs
// inside it; add scopes if a repo shows wrong edges.
func (b *builder) pyCalls(fn *ts.Node, callerID string, ext bool, src []byte) {
	body := fn.ChildByFieldName("body")
	if body == nil {
		return
	}
	params := fn.ChildByFieldName("parameters")
	within := func(c, of *ts.Node) bool {
		return of != nil && c.StartByte() >= of.StartByte() && c.EndByte() <= of.EndByte()
	}
	types, bound := map[string]string{}, map[string]bool{}
	mods := map[[2]string]string{} // function, name -> what an import inside it binds
	made := map[string]bool{}      // ids of the defs inside fn
	// The defs inside fn by function and name, and the function around each.
	// They are of this walk alone: a later function can have the id of fn.
	nested, parent := map[[2]string]string{}, map[string]string{}
	localType := pyLocalTyper(types, src)
	// Types and bound names cover the whole function, so a receiver is
	// resolved only after the walk.
	type site struct {
		caller, callee, recv string
		loc                  model.Loc
		member, local        bool
	}
	var sites []site
	scopes := []pyScope{{id: callerID, fn: callerID, end: fn.EndByte(), bodyLo: body.StartByte(), bodyHi: body.EndByte()}}
	walkKinds(fn, pyCallKinds, func(cv ts.Node) bool {
		c := &cv
		localType(c)
		for len(scopes) > 1 && c.StartByte() >= scopes[len(scopes)-1].end {
			scopes = scopes[:len(scopes)-1]
		}
		in := scopes[len(scopes)-1]
		if within(c, body) && pyImportNames(c, src, func(local, val string) {
			b.pyInFuncAlias(local, val, line(c))
			// A name of a class body is no name of the functions.
			if in.id != in.fn {
				return
			}
			key := [2]string{in.fn, local}
			if old, dup := mods[key]; dup && old != val {
				val = ""
			}
			mods[key] = val
		}) {
			return true
		}
		if within(c, body) || within(c, params) {
			pyBind(c, src, true, bound)
		}
		switch c.Kind() {
		case "function_definition", "class_definition":
			name, cbody := fieldText(c, "name", src), c.ChildByFieldName("body")
			if c.StartByte() == fn.StartByte() || name == "" || cbody == nil {
				return true
			}
			sc := in
			sc.id, sc.end = idutil.MakeID(in.id, name), c.EndByte()
			if c.Kind() == "function_definition" {
				// ponytail: one more part keeps the def off the id of a
				// module-level def or method `outer_name`; a def of that
				// longer name still collides.
				if b.seen[sc.id] && !made[sc.id] {
					sc.id = idutil.MakeID(in.id, name, "nested")
				}
				sc.fn, sc.outer, sc.bodyLo, sc.bodyHi = sc.id, in.fn, cbody.StartByte(), cbody.EndByte()
				sc.local = in.local || in.id != in.fn
				if !b.seen[sc.id] {
					made[sc.id] = true
					b.addNode(sc.id, name+"()", line(c))
					b.res.Edges = append(b.res.Edges, model.Edge{
						Source: in.fn, Target: sc.id, Relation: model.RelContains,
						Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(c),
					})
				}
				// A method of a local class is no name of the scope around
				// it. Two names of one id (`_f`, `f`) both reach its node.
				parent[sc.id] = in.fn
				if in.id == in.fn {
					nested[[2]string{in.fn, name}] = sc.id
				}
			}
			scopes = append(scopes, sc)
			return true
		}
		if c.Kind() != "call" {
			return true
		}
		caller := in.fn
		if c.StartByte() < in.bodyLo || c.EndByte() > in.bodyHi {
			// A default or an annotation of a def runs in the function the
			// def is made in; fn itself has none here.
			if caller = in.outer; caller == "" {
				return true
			}
		}
		f := c.ChildByFieldName("function")
		if f == nil {
			return true
		}
		switch f.Kind() {
		case "identifier":
			sites = append(sites, site{caller: caller, callee: f.Utf8Text(src), loc: line(c)})
		case "attribute":
			if a := f.ChildByFieldName("attribute"); a != nil {
				sites = append(sites, site{caller, a.Utf8Text(src), recvText(f, a, src), line(c), true, in.local})
			}
		}
		return true
	})
	// imported is what name names by an import no other binding hides: one
	// inside the caller or a function around it before one of the module.
	imported := func(caller, name string) (val string, inFn bool) {
		if bound[name] {
			return "", false
		}
		for s, ok := caller, true; ok; s, ok = parent[s] {
			if val, ok := mods[[2]string{s, name}]; ok {
				return val, true
			}
		}
		return b.pyMods[name], false
	}
sites:
	for _, c := range sites {
		n, mod := len(b.res.Calls), ""
		if !c.member {
			// A def of an enclosing function hides every other of its name.
			for s, ok := c.caller, true; ok; s, ok = parent[s] {
				if id, ok := nested[[2]string{s, c.callee}]; ok {
					b.jsNestedCall(c.caller, id, c.loc)
					continue sites
				}
			}
			val, inFn := imported(c.caller, c.callee)
			// A builtin names nothing of another file unless an import or a
			// def of this file has its name. A star import can bind any name.
			if val == "" && pyBuiltins[c.callee] && b.pyMods != nil && !b.pyOwn[c.callee] {
				// An import of a function around the call can name it even
				// when another binding hides which one does.
				own := false
				for s, ok := c.caller, true; ok && !own; s, ok = parent[s] {
					_, own = mods[[2]string{s, c.callee}]
				}
				if !own {
					continue
				}
			}
			b.call(c.caller, c.callee, c.loc)
			if mod = val; !strings.Contains(val, ":") {
				mod = ""
			}
			// An import in the function binds the callee to that and to
			// nothing else, even when it names no definition.
			if inFn {
				mod += ":"
			}
		} else {
			recv := c.recv
			// The self of a local class has no node to bind on.
			if _, isSelf := selfRecv[recv]; isSelf && c.local {
				continue
			}
			val, inFn := imported(c.caller, recv)
			// Upstream gives a call on an external module to a top-level
			// function, and to a plain module-level import, alone.
			if mod = val; mod != "" && (!ext || inFn || c.caller != callerID || strings.Contains(val, ":")) {
				mod += ":"
			}
			if t := types[recv]; t != "" {
				recv = t
			}
			b.callRecv(c.caller, c.callee, recv, c.loc)
		}
		if len(b.res.Calls) > n && mod != "" {
			for len(b.res.CallMods) < n {
				b.res.CallMods = append(b.res.CallMods, "")
			}
			b.res.CallMods = append(b.res.CallMods, mod)
		}
	}
}

// pyRationalePrefixes are the leading comment tokens that mark an explanatory
// comment worth capturing as a rationale node (mirrors upstream _RATIONALE_PREFIXES).
var pyRationalePrefixes = []string{
	"# NOTE:", "# IMPORTANT:", "# HACK:", "# WHY:", "# RATIONALE:", "# TODO:", "# FIXME:",
}

// pyRevisionRe matches an Alembic/Flask-Migrate `revision = "..."` header line.
var pyRevisionRe = regexp.MustCompile(`(?m)^revision\s*[:=]`)

// pyRationale is a deterministic post-pass mirroring upstream
// _extract_python_rationale: it captures module/class/function docstrings and
// `# NOTE:`-style comments as "rationale" nodes, each with a rationale_for edge
// to the file or the enclosing definition. No LLM is involved.
func (b *builder) pyRationale(root *ts.Node, src []byte) {
	// Module docstring — skipped for auto-generated files (Alembic, Django
	// migrations, protobuf stubs) whose module docstring is a revision
	// annotation, not architectural rationale.
	if !isAutogeneratedPython(src) {
		if text, ln, ok := pyDocstring(root, src); ok {
			b.addRationale(text, ln, b.fileID)
		}
	}
	// Class and function docstrings.
	b.pyWalkDocstrings(root, b.fileID, src)

	// Rationale comments (# NOTE:, # IMPORTANT:, ...).
	for lineNum := 1; ; lineNum++ {
		line, rest, more := bytes.Cut(src, []byte("\n"))
		if t := bytes.TrimSpace(line); len(t) > 0 && t[0] == '#' {
			stripped := string(t)
			for _, p := range pyRationalePrefixes {
				if strings.HasPrefix(stripped, p) {
					b.addRationale(stripped, lineNum, b.fileID)
					break
				}
			}
		}
		if !more {
			return
		}
		src = rest
	}
}

// pyWalkDocstrings recurses the AST attaching each class/function docstring to
// its definition node (rationale_for), mirroring upstream walk_docstrings. It
// descends into class bodies but not function bodies, so nested definitions
// inside a method are not visited (matching upstream).
func (b *builder) pyWalkDocstrings(n *ts.Node, parentID string, src []byte) {
	walkKinds(n, pyDefKinds, func(cv ts.Node) bool {
		n := &cv
		switch n.Kind() {
		case "class_definition":
			name := fieldText(n, "name", src)
			body := n.ChildByFieldName("body")
			if name != "" && body != nil {
				nid := b.id(name)
				if text, ln, ok := pyDocstring(body, src); ok {
					b.addRationale(text, ln, nid)
				}
				for _, c := range body.Children() {
					b.pyWalkDocstrings(&c, nid, src)
				}
			}
			return false
		case "function_definition":
			name := fieldText(n, "name", src)
			body := n.ChildByFieldName("body")
			if name != "" && body != nil {
				nid := b.id(name)
				if parentID != b.fileID {
					nid = idutil.MakeID(parentID, name)
				}
				if text, ln, ok := pyDocstring(body, src); ok {
					b.addRationale(text, ln, nid)
				}
			}
			return false
		}
		return true
	})
}

// pyDocstring returns a module/class/function body's leading docstring and its
// 1-based line. It mirrors upstream _get_docstring: only the first statement is
// inspected, it must be a bare string expression, and the unquoted text must
// exceed 20 characters.
func pyDocstring(body *ts.Node, src []byte) (string, int, bool) {
	if body == nil {
		return "", 0, false
	}
	// Find the first non-comment statement. tree-sitter includes comment nodes
	// as body children, whereas upstream's AST body[0] omits them; skip leading
	// comments so a comment before the docstring does not hide it.
	var stmt *ts.Node
	for i := uint(0); i < body.ChildCount(); i++ {
		child := body.Child(i)
		if child.Kind() == "comment" {
			continue
		}
		stmt = child
		break
	}
	if stmt == nil || stmt.Kind() != "expression_statement" {
		return "", 0, false
	}
	for j := uint(0); j < stmt.ChildCount(); j++ {
		sub := stmt.Child(j)
		if sub.Kind() == "string" || sub.Kind() == "concatenated_string" {
			text := stripDocstring(sub.Utf8Text(src))
			if utf8.RuneCountInString(text) > 20 {
				return text, int(stmt.StartPosition().Row) + 1, true
			}
		}
	}
	return "", 0, false
}

// stripDocstring peels surrounding quote characters and whitespace off a raw
// tree-sitter string node, mirroring upstream's strip chain.
func stripDocstring(s string) string {
	s = strings.Trim(s, "\"'")
	s = strings.Trim(s, "\"")
	s = strings.Trim(s, "'")
	return strings.TrimSpace(s)
}

// isAutogeneratedPython reports whether a file's first 2 KiB marks it as
// generated (protobuf/gRPC/OpenAPI) or a migration/revision (Alembic, Django),
// whose module docstring is boilerplate rather than rationale. Mirrors upstream
// _is_autogenerated_python.
func isAutogeneratedPython(src []byte) bool {
	head := src
	if len(head) > 2048 {
		head = head[:2048]
	}
	h := string(head)
	for _, m := range []string{"DO NOT EDIT", "@generated", "Generated by the protocol buffer"} {
		if strings.Contains(h, m) {
			return true
		}
	}
	if strings.Contains(h, "def upgrade(") && strings.Contains(h, "down_revision") && pyRevisionRe.MatchString(h) {
		return true
	}
	if strings.Contains(h, "class Migration(migrations.Migration)") && strings.Contains(h, "operations") {
		return true
	}
	return false
}

// pySuperclasses records an inherits edge per positional base class. Keyword
// arguments (`metaclass=M`) are not bases and are skipped.
func (b *builder) pySuperclasses(classID string, n *ts.Node, src []byte) {
	args := n.ChildByFieldName("superclasses")
	if args == nil {
		return
	}
	for i := uint(0); i < args.NamedChildCount(); i++ {
		arg := args.NamedChild(i)
		if arg.Kind() == "keyword_argument" {
			continue
		}
		// Only a plain name gets a stub when no definition binds it.
		if arg.Kind() == "identifier" {
			b.ref(classID, arg.Utf8Text(src), "inherits", false, line(n))
			continue
		}
		b.typeRef(classID, baseTypeName(arg.Utf8Text(src)), "inherits", line(n))
	}
}

// pyTypeSkip holds the names an annotation mentions that are not corpus
// symbols: typing containers, scalar builtins and unittest.mock.
var pyTypeSkip = set("list", "dict", "set", "tuple", "frozenset", "type",
	"List", "Dict", "Set", "Tuple", "FrozenSet", "Type",
	"Optional", "Union", "Sequence", "Iterable", "Mapping", "MutableMapping",
	"Iterator", "Callable", "Awaitable", "AsyncIterable", "AsyncIterator", "Coroutine",
	"Generator", "AsyncGenerator", "ContextManager", "AsyncContextManager",
	"Annotated", "ClassVar", "Final", "Literal", "Concatenate", "ParamSpec", "TypeVar",
	"None", "Ellipsis", "Self",
	"str", "int", "float", "bool", "bytes", "bytearray", "complex", "object", "True", "False",
	"MagicMock", "Mock", "AsyncMock", "NonCallableMock", "NonCallableMagicMock", "PropertyMock", "patch", "sentinel")

// pyDecoSkip holds the builtin and stdlib decorators.
var pyDecoSkip = set("property", "staticmethod", "classmethod", "abstractmethod",
	"abstractproperty", "cached_property", "wraps", "lru_cache", "cache",
	"singledispatch", "singledispatchmethod", "total_ordering",
	"contextmanager", "asynccontextmanager", "overload", "override",
	"final", "no_type_check", "runtime_checkable", "dataclass")

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// pyRefs records a references edge from the function or method id to each
// type its parameter and return annotations name, and to its decorators.
func (b *builder) pyRefs(id string, fn, deco *ts.Node, src []byte) {
	b.pyDecorators(id, deco, src)
	if params := fn.ChildByFieldName("parameters"); params != nil {
		for i := uint(0); i < params.NamedChildCount(); i++ {
			if p := params.NamedChild(i); p.Kind() == "typed_parameter" || p.Kind() == "typed_default_parameter" {
				b.pyTypeNames(id, p.ChildByFieldName("type"), src, line(fn))
			}
		}
	}
	b.pyTypeNames(id, fn.ChildByFieldName("return_type"), src, line(fn))
}

// pyTypeNames walks an annotation. A string annotation names nothing.
func (b *builder) pyTypeNames(id string, n *ts.Node, src []byte, loc model.Loc) {
	if n == nil {
		return
	}
	switch n.Kind() {
	case "identifier", "attribute":
		// A module variable made by TypeVar is a type parameter, not a type.
		if name := baseTypeName(n.Utf8Text(src)); !pyTypeSkip[name] && !b.pyTypeVars[name] {
			b.ref(id, name, "references", false, loc)
		}
		return
	case "keyword_argument":
		// Annotated[int, Field(gt=0)]: gt is no type.
		b.pyTypeNames(id, n.ChildByFieldName("value"), src, loc)
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		b.pyTypeNames(id, n.NamedChild(i), src, loc)
	}
}

// pyDecorators records a references edge from id to each decorator of the
// decorated_definition n.
func (b *builder) pyDecorators(id string, n *ts.Node, src []byte) {
	if n == nil {
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		d := n.NamedChild(i)
		if d.Kind() != "decorator" || d.NamedChildCount() == 0 {
			continue
		}
		expr := d.NamedChild(0)
		// @x.setter is the property protocol.
		text := expr.Utf8Text(src)
		if strings.HasSuffix(text, ".setter") || strings.HasSuffix(text, ".getter") || strings.HasSuffix(text, ".deleter") {
			continue
		}
		if expr.Kind() == "call" {
			if f := expr.ChildByFieldName("function"); f != nil {
				expr = f
			}
		}
		if expr.Kind() == "attribute" {
			expr = expr.ChildByFieldName("attribute")
		}
		if expr == nil || expr.Kind() != "identifier" {
			continue
		}
		if name := expr.Utf8Text(src); !pyDecoSkip[name] {
			b.ref(id, name, "references", true, line(d))
		}
	}
}

// pyTypeVarNames returns the module-level names bound by `T = TypeVar(...)`,
// ParamSpec or TypeVarTuple.
func pyTypeVarNames(root *ts.Node, src []byte) map[string]bool {
	var out map[string]bool
	for i := uint(0); i < root.NamedChildCount(); i++ {
		a := root.NamedChild(i)
		if a.Kind() != "expression_statement" || a.NamedChildCount() != 1 {
			continue
		}
		a = a.NamedChild(0)
		l, r := a.ChildByFieldName("left"), a.ChildByFieldName("right")
		if a.Kind() != "assignment" || l == nil || r == nil || l.Kind() != "identifier" || r.Kind() != "call" {
			continue
		}
		switch baseTypeName(fieldText(r, "function", src)) {
		case "TypeVar", "ParamSpec", "TypeVarTuple":
			if out == nil {
				out = map[string]bool{}
			}
			out[l.Utf8Text(src)] = true
		}
	}
	return out
}
