// Package ts is the subset of the go-tree-sitter API that internal/extract
// uses, on top of gositter (pure Go, no cgo).
package ts

import (
	"errors"
	"math"
	"os"
	"strconv"
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

// ErrBudget is the panic value of a Parse stopped by Budget.
var ErrBudget = errors.New("ts: parse budget exceeded")

// Budget is the most parser steps in a row without progress for a source of
// up to 4 KB; 0 is no limit.
var Budget = budget()

func budget() uint32 {
	if n, err := strconv.ParseUint(os.Getenv("GRAPHIFY_PARSE_BUDGET"), 10, 32); err == nil {
		return uint32(n)
	}
	return 65536
}

// limit scales Budget by the source size: 4 times at 16 KB, back to 1 from
// 64 KB. A broken region can cost steps without progress for each byte and
// still end: 8.5 for kotlin "<<<<". A stalled step can cost time in
// proportion to the size, so a large file gets no more steps.
func limit(n int) uint32 {
	n = max(n, 4096)
	f := min(n, 16384, max(1<<28/n, 4096))
	return uint32(min(uint64(Budget)*uint64(f)/4096, math.MaxUint32))
}

// Parse returns the root node and a release func. The tree lives in the
// parser's arenas: no node may be used after release. A parse stopped by
// Budget panics with ErrBudget.
func Parse(src []byte, lang *gositter.Language) (*Node, func()) {
	p := parsers.Get().(*gositter.Parser)
	_ = p.SetLanguage(lang)
	tree := p.ParseBudget(src, limit(len(src)))
	if tree == nil {
		parsers.Put(p)
		panic(ErrBudget)
	}
	return &Node{tree.RootNode()}, func() {
		p.Reset() // an idle parser holds no chunks another could use
		parsers.Put(p)
	}
}
