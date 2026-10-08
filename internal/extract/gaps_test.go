package extract

import (
	"os"
	"path/filepath"
	"testing"
)

// gapEdges extracts one file and returns "src->tgt" call/contains labels plus node labels.
func gapEdges(t *testing.T, rel, code string) (map[string]bool, map[string]bool) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, rel), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := File(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	ext := Resolve([]Result{r}, []string{rel})
	id2label := map[string]string{}
	nodes := map[string]bool{}
	for _, n := range ext.Nodes {
		id2label[n.ID] = n.Label
		nodes[n.Label] = true
	}
	edges := map[string]bool{}
	for _, e := range ext.Edges {
		edges[id2label[e.Source]+" "+e.Relation+" "+id2label[e.Target]] = true
	}
	return edges, nodes
}

func TestGapCSharpNullConditionalCall(t *testing.T) {
	edges, _ := gapEdges(t, "a.cs", `class S { public void Save() {} }
class C { void Run(S st) { st?.Save(); } }
`)
	if !edges["C.Run() calls S.Save()"] {
		t.Errorf("missing calls edge: %v", edges)
	}
}

func TestGapRubyBareSelfSend(t *testing.T) {
	edges, _ := gapEdges(t, "a.rb", `class W
  def run(helper)
    helper
    local = 1
    local
    do_thing
  end

  def do_thing
  end

  def helper
  end
end
`)
	if !edges["W.run() calls W.do_thing()"] {
		t.Errorf("missing bare self-send edge: %v", edges)
	}
	if edges["W.run() calls W.helper()"] {
		t.Errorf("parameter treated as call: %v", edges)
	}
}

func TestGapJSMemberAssignedFunction(t *testing.T) {
	edges, nodes := gapEdges(t, "a.js", `function helper2() {}
res.format = function () { helper2() }
`)
	if !nodes["format()"] || !edges["format() calls helper2()"] {
		t.Errorf("nodes=%v edges=%v", nodes, edges)
	}
}

func TestGapTSAbstractMethod(t *testing.T) {
	edges, nodes := gapEdges(t, "a.ts", `abstract class Shape {
  abstract area(): number;
  describe() { return this.area(); }
}
`)
	if !nodes["Shape.area()"] || !edges["Shape.describe() calls Shape.area()"] {
		t.Errorf("nodes=%v edges=%v", nodes, edges)
	}
}

func TestGapScalaDeferredDef(t *testing.T) {
	edges, nodes := gapEdges(t, "a.scala", `trait Shape {
  def area: Double
  def describe(): String = this.area().toString
}
`)
	if !nodes["Shape.area()"] || !edges["Shape.describe() calls Shape.area()"] {
		t.Errorf("nodes=%v edges=%v", nodes, edges)
	}
}

func TestGapAbstractWithSameFileOverride(t *testing.T) {
	edges, _ := gapEdges(t, "a.ts", `abstract class Shape {
  abstract area(): number;
  describe() { return this.area(); }
}
class Circle extends Shape {
  area() { return 1; }
}
`)
	if !edges["Shape.describe() calls Shape.area()"] {
		t.Errorf("ts: %v", edges)
	}
	edges, _ = gapEdges(t, "a.scala", `trait Shape {
  def area: Double
  def describe(): String = this.area().toString
}
class Circle extends Shape {
  def area: Double = 1.0
}
`)
	if !edges["Shape.describe() calls Shape.area()"] {
		t.Errorf("scala: %v", edges)
	}
}

func TestGapRubyLocalsNotSelfSends(t *testing.T) {
	for name, body := range map[string]string{
		"opassign": "value ||= 3\n    value",
		"multi":    "helper, value = 1, 2\n    value",
		"block":    "[1].each do |value|\n      value\n    end",
		"lambda":   "f = ->(value) { value }\n    f",
		"rescue":   "begin\n      1\n    rescue => value\n      value\n    end",
		"for":      "for value in [1]\n      value\n    end",
	} {
		edges, _ := gapEdges(t, "a.rb", "class C\n  def value\n    1\n  end\n  def run\n    "+body+"\n  end\nend\n")
		if edges["C.run() calls C.value()"] || edges[".run() calls .value()"] {
			t.Errorf("%s: fabricated edge: %v", name, edges)
		}
	}
}

func TestGapAbstractDoesNotBlockSingleImpl(t *testing.T) {
	for name, tc := range map[string]struct{ file, src, want string }{
		"ts":    {"a.ts", "abstract class Sh { abstract size(): number }\nclass Ci extends Sh { size() { return 1 } }\nfunction useIt(c: Ci) { return c.size() }\n", "useIt() calls Ci.size()"},
		"scala": {"a.scala", "trait Sh { def size: Int }\nclass Ci extends Sh { def size: Int = 1 }\nobject O { def useIt(c: Ci) = c.size() }\n", "O.useIt() calls Ci.size()"},
	} {
		edges, _ := gapEdges(t, tc.file, tc.src)
		if !edges[tc.want] {
			t.Errorf("%s same-file: %v", name, edges)
		}
	}
}

func TestGapAbstractCrossFileSingleImpl(t *testing.T) {
	root := t.TempDir()
	srcs := map[string]string{
		"shape.ts": "export abstract class Sh { abstract size(): number }\nexport class Ci extends Sh { size() { return 1 } }\n",
		"use.ts":   "import { Ci } from './shape'\nexport function useIt(c: Ci) { return c.size() }\n",
	}
	var rs []Result
	var files []string
	for f, s := range srcs {
		if err := os.WriteFile(filepath.Join(root, f), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		r, err := File(root, f)
		if err != nil {
			t.Fatal(err)
		}
		rs, files = append(rs, r), append(files, f)
	}
	ext := Resolve(rs, files)
	lbl := map[string]string{}
	for _, n := range ext.Nodes {
		lbl[n.ID] = n.Label
	}
	for _, e := range ext.Edges {
		if e.Relation == "calls" && lbl[e.Source] == "useIt()" && lbl[e.Target] == "Ci.size()" {
			return
		}
	}
	t.Errorf("missing useIt() calls Ci.size(): %v", ext.Edges)
}
