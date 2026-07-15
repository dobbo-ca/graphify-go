package extract

import (
	"path"
	"path/filepath"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsbash "github.com/tree-sitter/tree-sitter-bash/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// bashScriptRunners are the interpreters whose first .sh argument names an
// invoked script: `bash x.sh`, `sh x.sh`, … (mirrors upstream _BASH_SCRIPT_RUNNERS).
var bashScriptRunners = map[string]bool{
	"bash": true, "sh": true, "zsh": true, "ksh": true, "dash": true,
}

// extractBash pulls function definitions, `source`/`.` includes, and call edges
// out of a shell script. Bash has no class/struct constructs, so only top-level
// (and nested) function definitions become definition nodes. A command whose
// name is `source` or `.` is recorded as an import of the sourced file; a command
// that runs another script (`bash x.sh` or a bare `./x.sh`) records a
// `script_invocation` calls edge to that script's file node; every other command
// inside a function body is recorded as a call by its leading word and resolved
// against the corpus by Resolve.
func extractBash(rel string, src []byte) Result {
	root, done := parseRoot(src, tsbash.Language())
	defer done()
	b := newBuilder(rel)

	// Collect all function names first so a command that invokes a script is not
	// mistaken for one, and a forward-defined function still shadows a runner.
	funcs := collectBashFuncs(root, src)
	b.bashItems(root, src, "", funcs)
	return b.res
}

// collectBashFuncs returns the set of function names defined anywhere in the file.
func collectBashFuncs(n *ts.Node, src []byte) map[string]bool {
	funcs := map[string]bool{}
	var walk func(*ts.Node)
	walk = func(n *ts.Node) {
		if n.Kind() == "function_definition" {
			if name := fieldText(n, "name", src); name != "" {
				funcs[name] = true
			}
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(n)
	return funcs
}

// bashItems walks the statements directly under n. callerID is the id of the
// enclosing function ("" at file scope). Source/`.` commands always record an
// import; other commands record a call only when inside a function.
func (b *builder) bashItems(n *ts.Node, src []byte, callerID string, funcs map[string]bool) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "function_definition":
			b.bashFunc(c, src, funcs)
		case "command":
			b.bashCommand(c, src, callerID, funcs)
		default:
			// Descend through control-flow wrappers (if/for/while/etc.) so
			// commands nested in them are still seen.
			b.bashItems(c, src, callerID, funcs)
		}
	}
}

func (b *builder) bashFunc(n *ts.Node, src []byte, funcs map[string]bool) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	id := idutil.MakeID(b.stem, name)
	b.def(id, name, name+"()", line(n))
	if body := n.ChildByFieldName("body"); body != nil {
		b.bashItems(body, src, id, funcs)
	}
}

// bashCommand handles one command. `source x` / `. x` records an import of x; a
// script runner (`bash x.sh`) or a bare `./x.sh` records a script_invocation
// calls edge to the invoked script's file node; any other command inside a
// function (callerID != "") records a call by the command's leading word.
func (b *builder) bashCommand(n *ts.Node, src []byte, callerID string, funcs map[string]bool) {
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
	// A command whose name shadows a defined function is that function, not a
	// program invoking another script — skip the script-invocation edge (#1756).
	if !funcs[name] {
		if tgt := b.scriptInvocationTarget(n, name, src); tgt != "" {
			caller := callerID
			if caller == "" {
				caller = b.fileID // at file scope, the script's own file node is the caller
			}
			b.res.Edges = append(b.res.Edges, model.Edge{
				Source: caller, Target: tgt, Relation: "calls",
				Confidence: "EXTRACTED", Weight: 1.0,
				SourceFile: b.file, SourceLocation: line(n),
			})
		}
	}
	if callerID != "" {
		b.call(callerID, name, line(n))
	}
}

// scriptInvocationTarget returns the file-node id of a script this command runs,
// or "" when the command is not a static .sh invocation. It handles a bare
// `./x.sh` (the command name is the script) and a runner form `bash x.sh` (the
// first literal argument is the script). The target is resolved relative to the
// invoking script's own directory. A dynamic target (`bash "./$X.sh"`) is not a
// literal and yields "". The returned node id may not exist in the corpus; a
// dangling script_invocation edge is pruned when the graph is built.
func (b *builder) scriptInvocationTarget(n *ts.Node, cmdName string, src []byte) string {
	var raw string
	switch {
	case strings.HasSuffix(cmdName, ".sh"):
		// Bare `./x.sh`: the command name is the script and must be a literal
		// (no $VAR/command-substitution in the name).
		if nameNode := n.ChildByFieldName("name"); nameNode == nil || bashHasExpansion(nameNode) {
			return ""
		}
		raw = cmdName
	case bashScriptRunners[cmdName]:
		lit, ok := firstArgLiteral(n, src)
		if !ok {
			return ""
		}
		raw = lit
	default:
		return ""
	}
	if !strings.HasSuffix(raw, ".sh") {
		return ""
	}
	targetRel := path.Join(path.Dir(filepath.ToSlash(b.file)), raw)
	return idutil.MakeID(targetRel)
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

// firstArgLiteral returns the first argument's text and true only when that
// argument is a static literal — a word or a quoted string with no shell
// expansion. `bash "./$X.sh"` (an expansion) yields ok=false so dynamic targets
// are not resolved (mirrors upstream literal()).
func firstArgLiteral(n *ts.Node, src []byte) (string, bool) {
	arg := n.ChildByFieldName("argument")
	if arg == nil || bashHasExpansion(arg) {
		return "", false
	}
	return unquote(arg.Utf8Text(src)), true
}

// bashHasExpansion reports whether n contains any shell expansion / substitution
// (so its text is not a compile-time literal).
func bashHasExpansion(n *ts.Node) bool {
	switch n.Kind() {
	case "expansion", "simple_expansion", "command_substitution", "arithmetic_expansion", "process_substitution":
		return true
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		if bashHasExpansion(n.Child(i)) {
			return true
		}
	}
	return false
}
