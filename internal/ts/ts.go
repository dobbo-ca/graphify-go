// Package ts is the subset of the go-tree-sitter API that internal/extract
// uses, on top of gositter (pure Go, no cgo).
package ts

import (
	"sync"

	"github.com/dobbo-ca/gositter"
)

// Node wraps a gositter node behind a pointer, so nil stands for the null
// node as it does in go-tree-sitter.
type Node struct{ n gositter.Node }

func wrap(n gositter.Node) *Node {
	if n.IsNull() {
		return nil
	}
	return &Node{n}
}

func (n *Node) Kind() string                       { return n.n.Kind() }
func (n *Node) IsNamed() bool                      { return n.n.IsNamed() }
func (n *Node) StartByte() uint                    { return uint(n.n.StartByte()) }
func (n *Node) EndByte() uint                      { return uint(n.n.EndByte()) }
func (n *Node) StartPosition() gositter.Point      { return n.n.StartPosition() }
func (n *Node) ChildCount() uint                   { return uint(n.n.ChildCount()) }
func (n *Node) NamedChildCount() uint              { return uint(n.n.NamedChildCount()) }
func (n *Node) Child(i uint) *Node                 { return wrap(n.n.Child(uint32(i))) }
func (n *Node) NamedChild(i uint) *Node            { return wrap(n.n.NamedChild(uint32(i))) }
func (n *Node) ChildByFieldName(name string) *Node { return wrap(n.n.ChildByFieldName(name)) }
func (n *Node) FieldNameForChild(i uint32) string  { return n.n.FieldNameForChild(i) }
func (n *Node) Parent() *Node                      { return wrap(n.n.Parent()) }
func (n *Node) Equals(o Node) bool                 { return n.n.Equals(o.n) }
func (n *Node) Utf8Text(src []byte) string         { return n.n.Utf8Text(src) }

// Children returns Child(0..ChildCount()-1) in one pass; a Child(i) loop
// rescans from the first child every time.
func (n *Node) Children() []Node {
	var buf [16]gositter.Node
	cs := n.n.Children(buf[:0])
	out := make([]Node, len(cs))
	for i, c := range cs {
		out[i].n = c
	}
	return out
}

// Preorder calls fn for n and, where fn returns true, for everything below
// it in document order, with no slice per node.
func (n *Node) Preorder(fn func(Node) bool) {
	n.n.Preorder(func(c gositter.Node) bool { return fn(Node{c}) })
}

// PreorderMask is Preorder with fn called only for the node kinds in mask
// (gositter Language.KindMask); subtrees without one are skipped.
func (n *Node) PreorderMask(mask uint32, fn func(Node) bool) {
	n.n.PreorderMask(mask, func(c gositter.Node) bool { return fn(Node{c}) })
}

type Cursor struct{ c *gositter.TreeCursor }

func (n *Node) Walk() *Cursor { return &Cursor{n.n.Walk()} }
func (c *Cursor) Close()      { c.c.Close() }

func (n *Node) ChildrenByFieldName(name string, c *Cursor) []Node {
	cs := n.n.ChildrenByFieldName(name, c.c)
	out := make([]Node, len(cs))
	for i, x := range cs {
		out[i].n = x
	}
	return out
}

// Parsers keep their arenas between files.
var parsers = sync.Pool{New: func() any { return gositter.NewParser() }}

// Parse returns the root node and a release func. The tree lives in the
// parser's arenas: no node may be used after release.
func Parse(src []byte, lang *gositter.Language) (*Node, func()) {
	p := parsers.Get().(*gositter.Parser)
	_ = p.SetLanguage(lang)
	return &Node{p.Parse(src).RootNode()}, func() {
		p.Reset() // an idle parser holds no chunks another could use
		parsers.Put(p)
	}
}
