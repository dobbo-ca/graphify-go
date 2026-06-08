// Package extract turns a source file into graph fragments using tree-sitter.
// Each extractor emits definition nodes (file, function, type, method) plus the
// structural edges it can see locally (contains, imports). Calls and imports
// that cross files are recorded raw and stitched together by Resolve, which has
// the whole-corpus view needed to point a call at the right definition.
package extract

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tstsx "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// Def records a named definition so cross-file calls can resolve to it by name.
type Def struct {
	ID, Name, File string
}

// Call is an unresolved call site: CallerID invoked a symbol named Callee.
type Call struct {
	CallerID, Callee, File, Loc string
}

// Imp is an unresolved import: File imported the module named Spec.
type Imp struct {
	FileID, File, Spec, Loc string
}

// Result is one file's extraction before cross-file resolution.
type Result struct {
	Nodes []model.Node
	Edges []model.Edge
	Defs  []Def
	Calls []Call
	Imps  []Imp
}

// File extracts rel (a path relative to root). Unsupported extensions return an
// empty result, not an error, so the caller can skip them silently.
func File(root, rel string) (Result, error) {
	src, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return Result{}, err
	}
	rel = filepath.ToSlash(rel)
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".go":
		return extractGo(rel, src), nil
	case ".js", ".jsx", ".mjs", ".cjs":
		return extractJS(rel, src, tsjs.Language()), nil
	case ".tsx":
		return extractJS(rel, src, tstsx.LanguageTSX()), nil
	case ".ts":
		return extractJS(rel, src, tstsx.LanguageTypescript()), nil
	case ".tf", ".tfvars", ".hcl":
		return extractTerraform(rel, src), nil
	case ".py":
		return extractPython(rel, src), nil
	case ".rs":
		return extractRust(rel, src), nil
	case ".c", ".h":
		return extractC(rel, src), nil
	case ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx":
		return extractCpp(rel, src), nil
	case ".java":
		return extractJava(rel, src), nil
	case ".cs":
		return extractCSharp(rel, src), nil
	case ".rb":
		return extractRuby(rel, src), nil
	case ".php", ".phtml":
		return extractPHP(rel, src), nil
	case ".sh", ".bash":
		return extractBash(rel, src), nil
	case ".scala", ".sc":
		return extractScala(rel, src), nil
	case ".jl":
		return extractJulia(rel, src), nil
	case ".v", ".sv", ".svh", ".vh":
		return extractVerilog(rel, src), nil
	case ".kt", ".kts":
		return extractKotlin(rel, src), nil
	case ".lua":
		return extractLua(rel, src), nil
	case ".zig":
		return extractZig(rel, src), nil
	}
	return Result{}, nil
}

// parseRoot parses src with the given grammar and returns the root node plus a
// close func that frees the tree. Callers must hold the tree until they finish
// walking, then call close.
func parseRoot(src []byte, langPtr unsafe.Pointer) (*ts.Node, func()) {
	parser := ts.NewParser()
	_ = parser.SetLanguage(ts.NewLanguage(langPtr))
	tree := parser.Parse(src, nil)
	return tree.RootNode(), func() { tree.Close(); parser.Close() }
}

// fileStem qualifies a file's stem with its parent directory name so identically
// named files in different directories get distinct node IDs (mirrors Python
// _file_stem). Top-level files collapse to a bare stem.
func fileStem(rel string) string {
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	if dir := filepath.Base(filepath.Dir(rel)); dir != "." && dir != "/" && dir != "" {
		return dir + "." + base
	}
	return base
}

func line(n *ts.Node) string {
	return "L" + itoa(int(n.StartPosition().Row)+1)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}

// builder accumulates nodes/edges/defs for one file, de-duplicating node IDs.
type builder struct {
	file   string
	fileID string
	stem   string
	res    Result
	seen   map[string]bool
}

func newBuilder(rel string) *builder {
	fileID := idutil.MakeID(rel)
	b := &builder{file: rel, fileID: fileID, stem: fileStem(rel), seen: map[string]bool{}}
	b.addNode(fileID, filepath.Base(rel), "L1")
	return b
}

func (b *builder) addNode(id, label, loc string) {
	if b.seen[id] {
		return
	}
	b.seen[id] = true
	b.res.Nodes = append(b.res.Nodes, model.Node{
		ID: id, Label: label, FileType: "code", SourceFile: b.file, SourceLocation: loc,
	})
}

func (b *builder) contains(childID, loc string) {
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: b.fileID, Target: childID, Relation: "contains",
		Confidence: "EXTRACTED", SourceFile: b.file, SourceLocation: loc,
	})
}

// def registers a definition: adds its node, a contains edge from the file, and
// records it for cross-file call resolution.
func (b *builder) def(id, name, displayLabel, loc string) {
	b.addNode(id, displayLabel, loc)
	b.contains(id, loc)
	b.res.Defs = append(b.res.Defs, Def{ID: id, Name: name, File: b.file})
}

func (b *builder) call(callerID, callee, loc string) {
	if callee == "" || callerID == "" {
		return
	}
	b.res.Calls = append(b.res.Calls, Call{CallerID: callerID, Callee: callee, File: b.file, Loc: loc})
}

func (b *builder) imp(spec, loc string) {
	if spec != "" {
		b.res.Imps = append(b.res.Imps, Imp{FileID: b.fileID, File: b.file, Spec: spec, Loc: loc})
	}
}

// walk visits every descendant of n, calling fn on each. fn returns false to
// stop descending into that node's children.
func walk(n *ts.Node, fn func(*ts.Node) bool) {
	if !fn(n) {
		return
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		walk(n.Child(i), fn)
	}
}
