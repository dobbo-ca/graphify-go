package extract

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsverilog "github.com/tree-sitter/tree-sitter-verilog/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractVerilog pulls modules/programs/interfaces/packages (as types), classes
// (+ methods), top-level functions/tasks, `include` directives (as imports), and
// call edges out of a Verilog / SystemVerilog file. The grammar exposes no field
// names, so names are read from the first identifier descendant of each
// declaration; calls are recorded by the called name and resolved by Resolve.
func extractVerilog(rel string, src []byte) Result {
	root, done := parseRoot(src, tsverilog.Language())
	defer done()
	b := newBuilder(rel)

	b.verilogItems(root, src)
	return b.res
}

// verilogItems handles each item directly under n (a source_file, a package
// body, or a wrapper node), unwrapping package/item wrappers so the declarations
// they hold still surface.
func (b *builder) verilogItems(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "module_declaration", "program_declaration", "interface_declaration":
			b.verilogModule(c, src)
		case "package_declaration":
			b.verilogPackage(c, src)
		case "class_declaration":
			b.verilogClass(c, src)
		case "function_declaration", "task_declaration":
			b.verilogSubroutine(c, src)
		case "include_compiler_directive":
			b.verilogInclude(c, src)
		case "package_or_generate_item_declaration", "package_export_declaration":
			// Top-level functions/tasks/classes are wrapped in this node.
			b.verilogItems(c, src)
		}
	}
}

// verilogModule records a module/program/interface as a type definition. Its
// name is the first simple_identifier in its header. Nested functions/tasks and
// call sites inside the body are recorded under the module.
func (b *builder) verilogModule(n *ts.Node, src []byte) {
	name := headerName(n, src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name, line(n))
	b.verilogCalls(n, id, src)
}

// verilogPackage records a package as a type definition (its name is the first
// package_identifier) and descends into its items so packaged functions, tasks
// and classes surface.
func (b *builder) verilogPackage(n *ts.Node, src []byte) {
	name := firstIdentIn(n, "package_identifier", src)
	if name == "" {
		return
	}
	b.def(idutil.MakeID(b.stem, name), name, name, line(n))
	b.verilogItems(n, src)
}

// verilogClass records a class as a type definition and its function/task
// methods scoped under the class name, each with a contains edge.
func (b *builder) verilogClass(n *ts.Node, src []byte) {
	name := firstIdentIn(n, "class_identifier", src)
	if name == "" {
		return
	}
	classID := idutil.MakeID(b.stem, name)
	b.def(classID, name, name, line(n))

	for i := uint(0); i < n.ChildCount(); i++ {
		item := n.Child(i)
		if item.Kind() != "class_item" {
			continue
		}
		for j := uint(0); j < item.ChildCount(); j++ {
			cm := item.Child(j)
			if cm.Kind() != "class_method" {
				continue
			}
			b.verilogMethod(cm, name, classID, src)
		}
	}
}

// verilogMethod records a single function/task method of a class.
func (b *builder) verilogMethod(cm *ts.Node, typeName, classID string, src []byte) {
	for k := uint(0); k < cm.ChildCount(); k++ {
		sub := cm.Child(k)
		if sub.Kind() != "function_declaration" && sub.Kind() != "task_declaration" {
			continue
		}
		mname := subroutineName(sub, src)
		if mname == "" {
			continue
		}
		mid := idutil.MakeID(b.stem, typeName, mname)
		b.addNode(mid, typeName+"."+mname+"()", line(sub))
		b.res.Edges = append(b.res.Edges, model.Edge{
			Source: classID, Target: mid, Relation: "contains",
			Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: line(sub),
		})
		b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
		b.verilogCalls(sub, mid, src)
	}
}

// verilogSubroutine records a top-level function/task as a definition and its
// internal call sites.
func (b *builder) verilogSubroutine(n *ts.Node, src []byte) {
	name := subroutineName(n, src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	b.verilogCalls(n, id, src)
}

// verilogInclude records the path of an `include "x.svh"` directive as an
// import. The quotes are stripped so the spec is a plain path (an external
// dependency, since it is not a relative ./ path).
func (b *builder) verilogInclude(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() == "double_quoted_string" {
			spec := strings.Trim(c.Utf8Text(src), `"`)
			b.imp(spec, line(n))
			return
		}
	}
}

// verilogCalls walks a declaration body and records each user call site. Direct
// task/function calls (`f(...)`) come from tf_call; method calls (`x.f(...)`)
// come from method_call. System tasks (`$display`) are skipped as builtins.
func (b *builder) verilogCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walk(body, func(c *ts.Node) bool {
		switch c.Kind() {
		case "tf_call":
			if name := firstChildIdent(c, src); name != "" {
				b.call(callerID, name, line(c))
			}
		case "method_call":
			if mb := childOfKind(c, "method_call_body"); mb != nil {
				if mi := childOfKind(mb, "method_identifier"); mi != nil {
					b.call(callerID, firstSimpleIdent(mi, src), line(c))
				}
			}
		}
		return true
	})
}

// headerName returns the first simple_identifier inside the *_header child of a
// module/program/interface declaration (its declared name).
func headerName(n *ts.Node, src []byte) string {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if strings.HasSuffix(c.Kind(), "_header") {
			return firstSimpleIdent(c, src)
		}
	}
	return ""
}

// subroutineName returns the declared name of a function/task by digging into
// its *_identifier node (function_identifier / task_identifier), skipping any
// leading return-type identifier.
func subroutineName(n *ts.Node, src []byte) string {
	var name string
	walk(n, func(c *ts.Node) bool {
		if name != "" {
			return false
		}
		if c.Kind() == "function_identifier" || c.Kind() == "task_identifier" {
			name = firstSimpleIdent(c, src)
			return false
		}
		return true
	})
	return name
}

// firstIdentIn returns the simple_identifier text of the first descendant of n
// whose kind is wrapperKind (e.g. package_identifier, class_identifier).
func firstIdentIn(n *ts.Node, wrapperKind string, src []byte) string {
	var name string
	walk(n, func(c *ts.Node) bool {
		if name != "" {
			return false
		}
		if c.Kind() == wrapperKind {
			name = firstSimpleIdent(c, src)
			return false
		}
		return true
	})
	return name
}

// firstSimpleIdent returns the text of the first simple_identifier at or below n.
func firstSimpleIdent(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	var name string
	walk(n, func(c *ts.Node) bool {
		if name != "" {
			return false
		}
		if c.Kind() == "simple_identifier" {
			name = c.Utf8Text(src)
			return false
		}
		return true
	})
	return name
}

// firstChildIdent returns the text of the first direct simple_identifier child
// of n (the callee name of a tf_call, before its argument list).
func firstChildIdent(n *ts.Node, src []byte) string {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() == "simple_identifier" {
			return c.Utf8Text(src)
		}
	}
	return ""
}

// childOfKind returns the first direct child of n with the given kind, or nil.
func childOfKind(n *ts.Node, kind string) *ts.Node {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == kind {
			return c
		}
	}
	return nil
}
