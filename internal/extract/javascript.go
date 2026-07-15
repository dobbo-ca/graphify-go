package extract

import (
	"regexp"
	"strings"
	"unsafe"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractJS pulls functions, classes (+ methods), interfaces, type aliases,
// enums, imports, and call edges out of a JS/TS/TSX file. The same walker
// serves all three because TypeScript's grammar is a superset of JavaScript's.
func extractJS(rel string, src []byte, langPtr unsafe.Pointer) Result {
	root, done := parseRoot(src, langPtr)
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		b.jsStatement(root.Child(i), src)
	}
	b.jsRationale(src)
	return b.res
}

// jsStatement handles one top-level statement, unwrapping `export ...` first.
func (b *builder) jsStatement(n *ts.Node, src []byte) {
	switch n.Kind() {
	case "export_statement":
		if d := n.ChildByFieldName("declaration"); d != nil {
			b.jsStatement(d, src)
		}
	case "import_statement":
		if s := n.ChildByFieldName("source"); s != nil {
			b.imp(unquote(s.Utf8Text(src)), line(n))
		}
	case "function_declaration", "generator_function_declaration":
		b.jsFunc(n, src)
	case "class_declaration", "abstract_class_declaration":
		b.jsClass(n, src)
	case "interface_declaration", "type_alias_declaration", "enum_declaration":
		b.jsNamedType(n, src)
	case "lexical_declaration", "variable_declaration":
		b.jsVarFuncs(n, src)
	}
}

// degenerateName reports whether a symbol name carries no identifier signal — it
// normalizes to an empty ID (a minified `$`, a JSONC `//` comment key). MakeID
// would collapse such a name to the bare file stem, producing a label-only noise
// node that can also collide with an extensionless sibling file. Skip it (#1899).
func degenerateName(name string) bool { return idutil.NormalizeID(name) == "" }

func (b *builder) jsFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if degenerateName(name) {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.jsCalls(n.ChildByFieldName("body"), id, src)
}

func (b *builder) jsClass(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if degenerateName(name) {
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
		if m.Kind() != "method_definition" {
			continue
		}
		mname := fieldText(m, "name", src)
		if degenerateName(mname) {
			continue
		}
		mid := idutil.MakeID(b.stem, name, mname)
		b.addNode(mid, name+"."+mname+"()", line(m))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: classID, Target: mid, Relation: "contains",
			Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(m),
		})
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.jsCalls(m.ChildByFieldName("body"), mid, src)
	}
}

func (b *builder) jsNamedType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if degenerateName(name) {
		return
	}
	b.def(idutil.MakeID(b.stem, name), name, name, line(n))
}

// jsVarFuncs captures `const foo = () => {}` and `const foo = function() {}`,
// the dominant way functions are defined in modern JS/TS.
func (b *builder) jsVarFuncs(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		d := n.Child(i)
		if d.Kind() != "variable_declarator" {
			continue
		}
		val := d.ChildByFieldName("value")
		if val == nil || (val.Kind() != "arrow_function" && val.Kind() != "function_expression" && val.Kind() != "function") {
			continue
		}
		name := fieldText(d, "name", src)
		if degenerateName(name) {
			continue
		}
		id := idutil.MakeID(b.stem, name)
		b.def(id, name, name+"()", line(d))
		b.jsCalls(val.ChildByFieldName("body"), id, src)
	}
}

func (b *builder) jsCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		if c.Kind() != "call_expression" {
			return true
		}
		fn := c.ChildByFieldName("function")
		if fn == nil {
			return true
		}
		switch fn.Kind() {
		case "identifier":
			b.call(callerID, fn.Utf8Text(src), line(c))
		case "member_expression":
			if p := fn.ChildByFieldName("property"); p != nil {
				b.call(callerID, p.Utf8Text(src), line(c))
			}
		}
		return true
	})
}

// jsRationalePrefixes are the leading comment tokens that mark an explanatory
// comment worth capturing as a rationale node (mirrors upstream
// _JS_RATIONALE_PREFIXES; covers `//` line comments and `*`-prefixed lines
// inside block comments).
var jsRationalePrefixes = []string{
	"// NOTE:", "// IMPORTANT:", "// HACK:", "// WHY:", "// RATIONALE:",
	"// TODO:", "// FIXME:",
	"* NOTE:", "* IMPORTANT:", "* HACK:", "* WHY:", "* RATIONALE:",
	"* TODO:", "* FIXME:",
}

// jsCommentLineRe matches a line that begins (after optional indent) with a
// comment marker, so doc references are only harvested from comments.
var jsCommentLineRe = regexp.MustCompile(`^\s*(//|/\*|\*)`)

// jsDocRefRe finds architecture-decision / RFC references (ADR-0011, RFC 793) in
// a comment; jsDocRefParseRe splits a matched token into its kind and number.
var (
	jsDocRefRe      = regexp.MustCompile(`(?i)\b(ADR[- ]?\d{1,5}|RFC[- ]?\d{1,5})\b`)
	jsDocRefParseRe = regexp.MustCompile(`([A-Za-z]+)[- ]?(\d+)`)
)

// jsRationale is a deterministic post-pass mirroring upstream
// _extract_js_rationale: `// NOTE:`-style comments become "rationale" nodes
// (rationale_for edge to the file) and ADR/RFC tokens in comments become
// "doc_ref" nodes (cites edge from the file). No LLM is involved.
func (b *builder) jsRationale(src []byte) {
	seenDocRefs := map[string]bool{}
	for i, lineText := range strings.Split(string(src), "\n") {
		lineNum := i + 1
		stripped := strings.TrimSpace(lineText)
		for _, p := range jsRationalePrefixes {
			if strings.HasPrefix(stripped, p) {
				b.addRationale(strings.TrimLeft(stripped, "/* "), lineNum, b.fileID)
				break
			}
		}
		if jsCommentLineRe.MatchString(lineText) {
			for _, m := range jsDocRefRe.FindAllStringSubmatch(stripped, -1) {
				b.addDocRef(m[1], lineNum, seenDocRefs)
			}
		}
	}
}

// addDocRef records a "doc_ref" node for a normalized ADR/RFC reference and a
// cites edge from the file to it. Labels are normalized (ADR-0011, RFC-793) and
// deduped per file so repeated citations collapse to one node. Mirrors upstream
// _add_doc_ref.
func (b *builder) addDocRef(token string, lineNum int, seenDocRefs map[string]bool) {
	m := jsDocRefParseRe.FindStringSubmatch(token)
	if m == nil {
		return
	}
	kind := strings.ToUpper(m[1])
	label := kind + "-" + m[2]
	if kind == "ADR" {
		label = kind + "-" + zfill(m[2], 4)
	}
	if seenDocRefs[label] {
		return
	}
	seenDocRefs[label] = true
	rid := idutil.MakeID("docref", label)
	loc := "L" + itoa(lineNum)
	if !b.seen[rid] {
		b.seen[rid] = true
		b.res.Nodes = append(b.res.Nodes, model.Node{
			ID: rid, Label: label, FileType: "doc_ref",
			SourceFile: b.file, SourceLocation: loc,
		})
	}
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: b.fileID, Target: rid, Relation: "cites",
		Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: loc,
	})
}

// zfill left-pads s with zeros to at least width characters (Python str.zfill for
// unsigned numeric strings).
func zfill(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat("0", width-len(s)) + s
}
