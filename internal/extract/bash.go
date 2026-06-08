package extract

import (
	ts "github.com/tree-sitter/go-tree-sitter"
	tsbash "github.com/tree-sitter/tree-sitter-bash/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
)

// extractBash pulls function definitions, `source`/`.` includes, and call edges
// out of a shell script. Bash has no class/struct constructs, so only top-level
// (and nested) function definitions become definition nodes. A command whose
// name is `source` or `.` is recorded as an import of the sourced file; every
// other command inside a function body is recorded as a call by its leading
// word and resolved against the corpus by Resolve.
func extractBash(rel string, src []byte) Result {
	root, done := parseRoot(src, tsbash.Language())
	defer done()
	b := newBuilder(rel)

	b.bashItems(root, src, "")
	return b.res
}

// bashItems walks the statements directly under n. callerID is the id of the
// enclosing function ("" at file scope). Source/`.` commands always record an
// import; other commands record a call only when inside a function.
func (b *builder) bashItems(n *ts.Node, src []byte, callerID string) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "function_definition":
			b.bashFunc(c, src)
		case "command":
			b.bashCommand(c, src, callerID)
		default:
			// Descend through control-flow wrappers (if/for/while/etc.) so
			// commands nested in them are still seen.
			b.bashItems(c, src, callerID)
		}
	}
}

func (b *builder) bashFunc(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	if body := n.ChildByFieldName("body"); body != nil {
		b.bashItems(body, src, id)
	}
}

// bashCommand handles one command. `source x` / `. x` records an import of x;
// any other command inside a function (callerID != "") records a call by the
// command's leading word.
func (b *builder) bashCommand(n *ts.Node, src []byte, callerID string) {
	name := commandName(n, src)
	if name == "" {
		return
	}
	if name == "source" || name == "." {
		if spec := firstArg(n, src); spec != "" {
			b.imp(spec, line(n))
		}
		return
	}
	if callerID != "" {
		b.call(callerID, name, line(n))
	}
}

// commandName returns the leading word of a command (its program/function name),
// e.g. "greet" for `greet "$1"`.
func commandName(n *ts.Node, src []byte) string {
	cn := n.ChildByFieldName("name")
	if cn == nil {
		return ""
	}
	return cn.Utf8Text(src)
}

// firstArg returns the text of a command's first argument, stripping surrounding
// quotes, e.g. "lib/util.sh" for `source "lib/util.sh"`.
func firstArg(n *ts.Node, src []byte) string {
	arg := n.ChildByFieldName("argument")
	if arg == nil {
		return ""
	}
	return unquote(arg.Utf8Text(src))
}
