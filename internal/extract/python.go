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
	case "import_statement", "import_from_statement", "future_import_statement":
		b.pyImports(n, src)
	}
}

func (b *builder) pyFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.pyCalls(n.ChildByFieldName("body"), id, src)
}

func (b *builder) pyClass(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	classID := idutil.MakeID(b.stem, name)
	b.def(classID, name, name, line(n))

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
		b.pyCalls(m.ChildByFieldName("body"), mid, src)
	}
}

// pyImports records each imported module. `import a.b` and `from a.b import c`
// both record the module path a.b; the dotted name resolves to an external
// dependency node (Python relative imports stay external for now).
func (b *builder) pyImports(n *ts.Node, src []byte) {
	switch n.Kind() {
	case "import_statement", "future_import_statement":
		for i := uint(0); i < n.ChildCount(); i++ {
			c := n.Child(i)
			switch c.Kind() {
			case "dotted_name":
				b.imp(c.Utf8Text(src), line(n))
			case "aliased_import":
				if name := c.ChildByFieldName("name"); name != nil {
					b.imp(name.Utf8Text(src), line(n))
				}
			}
		}
	case "import_from_statement":
		mod := n.ChildByFieldName("module_name")
		if mod == nil {
			return
		}
		b.imp(mod.Utf8Text(src), line(n))
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

// pyCalls walks a function body and records each call site. Direct calls
// (`f()`) record the identifier; attribute calls (`x.f()`) record the
// attribute name.
func (b *builder) pyCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
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
				b.callMember(callerID, a.Utf8Text(src), line(c))
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
	for i := uint(0); i < body.ChildCount(); i++ {
		stmt := body.Child(i)
		if stmt.Kind() == "expression_statement" {
			for j := uint(0); j < stmt.ChildCount(); j++ {
				sub := stmt.Child(j)
				if sub.Kind() == "string" || sub.Kind() == "concatenated_string" {
					text := stripDocstring(sub.Utf8Text(src))
					if utf8.RuneCountInString(text) > 20 {
						return text, int(stmt.StartPosition().Row) + 1, true
					}
				}
			}
		}
		break
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
