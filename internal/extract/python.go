package extract

import (
	"strings"

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
