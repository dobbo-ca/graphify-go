package extract

import (
	"regexp"
	"strings"
	"unicode/utf8"

	ts "github.com/tree-sitter/go-tree-sitter"
	tspy "github.com/tree-sitter/tree-sitter-python/bindings/go"

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

	for i := uint(0); i < root.ChildCount(); i++ {
		b.pyStatement(root.Child(i), src)
	}
	b.pyRationale(root, src)
	return b.res
}

// pyStatement handles one module-level statement, unwrapping decorators first.
func (b *builder) pyStatement(n *ts.Node, src []byte) {
	switch n.Kind() {
	case "decorated_definition":
		if d := n.ChildByFieldName("definition"); d != nil {
			b.pyStatement(d, src)
		}
	case "function_definition":
		b.pyFunc(n, src)
	case "class_definition":
		b.pyClass(n, src)
	case "import_statement", "import_from_statement", "if_statement", "try_statement", "with_statement":
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

func (b *builder) pyFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.pyCalls(n, id, pyLocalTypes(n, src), src)
}

func (b *builder) pyClass(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	classID := idutil.MakeID(b.stem, name)
	b.def(classID, name, name, line(n))
	b.pySuperclasses(classID, n, src)

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		if m.Kind() == "decorated_definition" {
			m = m.ChildByFieldName("definition")
			if m == nil {
				continue
			}
		}
		if m.Kind() != "function_definition" {
			continue
		}
		mname := fieldText(m, "name", src)
		if mname == "" {
			continue
		}
		mid := idutil.MakeID(b.stem, name, mname)
		b.addNode(mid, name+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: classID, Target: mid, Relation: "contains",
			Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(m),
		})
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.pyCalls(m, mid, pyLocalTypes(m, src), src)
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
	stem := pyModuleStem(mod, src)
	if stem == "" {
		return
	}
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
			Local: local, Imported: imported, ModuleStem: stem, Loc: loc,
		})
	}
}

// pyModuleStem returns the final component of a `from ... import` module name,
// mirroring upstream _module_stem: `util.math` -> `math`, `.helper` -> `helper`.
func pyModuleStem(mod *ts.Node, src []byte) string {
	text := strings.Trim(mod.Utf8Text(src), ".")
	if i := strings.LastIndex(text, "."); i >= 0 {
		return text[i+1:]
	}
	return text
}

// pyClassName matches a bare capitalized name, the only annotation or callee
// taken as a class.
var pyClassName = regexp.MustCompile(`^[A-Z]\w*$`)

// pyLocalTypes maps the locals of function fn to their class, from a parameter
// annotation (`c: Client`) or a constructor binding (`c = Client()`). A local
// also bound by anything else (assignment, unpacking, for, with, except, `:=`)
// maps to "".
// ponytail: one flat scope per function, nested defs are not tracked; add
// scopes if a repo shows wrong edges.
func pyLocalTypes(fn *ts.Node, src []byte) map[string]string {
	types := map[string]string{}
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
	walk(fn, func(c *ts.Node) bool {
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
	})
	return types
}

// pyModules maps each local name bound by exactly one module-level `import x`
// or `import x as y`, and bound nowhere else at module scope, to its module.
func pyModules(root *ts.Node, src []byte) map[string]string {
	mods, bound := map[string]string{}, map[string]bool{}
	for i := uint(0); i < root.ChildCount(); i++ {
		st := root.Child(i)
		if st.Kind() != "import_statement" {
			pyBinds(st, src, false, bound)
			continue
		}
		for j := uint(0); j < st.ChildCount(); j++ {
			c := st.Child(j)
			local, spec := "", ""
			switch c.Kind() {
			case "dotted_name":
				// `import a.b` binds a, which is not the module imported.
				local = c.NamedChild(0).Utf8Text(src)
				if c.NamedChildCount() == 1 {
					spec = local
				}
			case "aliased_import":
				local, spec = fieldText(c, "alias", src), fieldText(c, "name", src)
			default:
				continue
			}
			if _, dup := mods[local]; dup || spec == "" {
				bound[local] = true
			}
			mods[local] = spec
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

// pyBinds adds the names bound under n to bound. Unless deep, the body of a
// def, class or lambda is skipped: it is another scope, and only a `global`
// reaches out of it.
func pyBinds(n *ts.Node, src []byte, deep bool, bound map[string]bool) {
	walk(n, func(c *ts.Node) bool {
		switch c.Kind() {
		case "function_definition", "class_definition", "lambda":
			pyTargets(c.ChildByFieldName("name"), src, bound)
			if !deep {
				walk(c, func(g *ts.Node) bool {
					if g.Kind() == "global_statement" {
						pyTargets(g, src, bound)
					}
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
	})
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

// pyCalls walks the body of function fn and records each call site. Direct
// calls (`f()`) record the identifier; attribute calls (`x.f()`) record the
// attribute name, with the receiver's class in place of a typed local.
func (b *builder) pyCalls(fn *ts.Node, callerID string, types map[string]string, src []byte) {
	body := fn.ChildByFieldName("body")
	if body == nil {
		return
	}
	bound := map[string]bool{}
	pyBinds(fn.ChildByFieldName("parameters"), src, true, bound)
	pyBinds(body, src, true, bound)
	walk(body, func(c *ts.Node) bool {
		if c.Kind() != "call" {
			return true
		}
		fn := c.ChildByFieldName("function")
		if fn == nil {
			return true
		}
		switch fn.Kind() {
		case "identifier":
			b.call(callerID, fn.Utf8Text(src), line(c))
		case "attribute":
			if a := fn.ChildByFieldName("attribute"); a != nil {
				recv := recvText(fn, a, src)
				mod := b.pyMods[recv]
				if bound[recv] {
					mod = ""
				}
				if t := types[recv]; t != "" {
					recv = t
				}
				n := len(b.res.Calls)
				b.callRecv(callerID, a.Utf8Text(src), recv, line(c))
				if len(b.res.Calls) > n {
					b.res.Calls[n].Module = mod
				}
			}
		}
		return true
	})
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
	for i, lineText := range strings.Split(string(src), "\n") {
		stripped := strings.TrimSpace(lineText)
		for _, p := range pyRationalePrefixes {
			if strings.HasPrefix(stripped, p) {
				b.addRationale(stripped, i+1, b.fileID)
				break
			}
		}
	}
}

// pyWalkDocstrings recurses the AST attaching each class/function docstring to
// its definition node (rationale_for), mirroring upstream walk_docstrings. It
// descends into class bodies but not function bodies, so nested definitions
// inside a method are not visited (matching upstream).
func (b *builder) pyWalkDocstrings(n *ts.Node, parentID string, src []byte) {
	switch n.Kind() {
	case "class_definition":
		name := fieldText(n, "name", src)
		body := n.ChildByFieldName("body")
		if name != "" && body != nil {
			nid := idutil.MakeID(b.stem, name)
			if text, ln, ok := pyDocstring(body, src); ok {
				b.addRationale(text, ln, nid)
			}
			for i := uint(0); i < body.ChildCount(); i++ {
				b.pyWalkDocstrings(body.Child(i), nid, src)
			}
		}
		return
	case "function_definition":
		name := fieldText(n, "name", src)
		body := n.ChildByFieldName("body")
		if name != "" && body != nil {
			nid := idutil.MakeID(b.stem, name)
			if parentID != b.fileID {
				nid = idutil.MakeID(parentID, name)
			}
			if text, ln, ok := pyDocstring(body, src); ok {
				b.addRationale(text, ln, nid)
			}
		}
		return
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		b.pyWalkDocstrings(n.Child(i), parentID, src)
	}
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
	if pyRevisionRe.MatchString(h) && strings.Contains(h, "def upgrade(") && strings.Contains(h, "down_revision") {
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
		b.typeRef(classID, baseTypeName(arg.Utf8Text(src)), "inherits", line(n))
	}
}
