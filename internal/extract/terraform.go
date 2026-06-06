package extract

import (
	"path/filepath"
	"strings"

	tshcl "github.com/tree-sitter-grammars/tree-sitter-hcl/bindings/go"
	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractTerraform pulls Terraform/HCL blocks and the references between them.
// Nodes: resources, data sources, modules, variables, outputs, providers, and
// locals. Edges: contains (file -> block), references (block -> the addresses it
// interpolates, e.g. aws_instance.web -> var.region), and depends_on.
//
// IDs are scoped by the parent DIRECTORY, not the file stem: Terraform addresses
// are module(directory)-scoped, so a resource defined in main.tf is referenced
// from other .tf files in the same directory. Directory scoping lets those
// cross-file references resolve once per-file extractions are merged.
func extractTerraform(rel string, src []byte) Result {
	root, done := parseRoot(src, tshcl.Language())
	defer done()

	scope := dirScope(rel)
	fileID := idutil.MakeID(rel)
	res := Result{Nodes: []model.Node{{
		ID: fileID, Label: filepath.Base(rel), FileType: "code", SourceFile: rel, SourceLocation: "L1",
	}}}
	seen := map[string]bool{fileID: true}

	// def adds a block node (once) and a contains edge from the file.
	def := func(addr, label, loc string) string {
		id := idutil.MakeID(scope, addr)
		if !seen[id] {
			seen[id] = true
			res.Nodes = append(res.Nodes, model.Node{
				ID: id, Label: label, FileType: "code", SourceFile: rel, SourceLocation: loc,
			})
			res.Edges = append(res.Edges, model.Edge{
				Source: fileID, Target: id, Relation: "contains",
				Confidence: "EXTRACTED", SourceFile: rel, SourceLocation: loc,
			})
		}
		return id
	}
	// refsFrom emits an edge from srcID to every address interpolated within node.
	refsFrom := func(srcID string, node *ts.Node) {
		walk(node, func(c *ts.Node) bool {
			if c.Kind() != "attribute" {
				return true
			}
			rel2 := "references"
			if tfChild(c, "identifier", src) == "depends_on" {
				rel2 = "depends_on"
			}
			walk(c, func(v *ts.Node) bool {
				if v.Kind() != "variable_expr" {
					return true
				}
				if addr := tfRefAddress(v, src); addr != "" {
					tgt := idutil.MakeID(scope, addr)
					if tgt != srcID {
						res.Edges = append(res.Edges, model.Edge{
							Source: srcID, Target: tgt, Relation: rel2,
							Confidence: "EXTRACTED", SourceFile: rel, SourceLocation: line(c),
						})
					}
				}
				return true
			})
			return true
		})
	}

	body := tfChildNode(root, "body")
	if body == nil {
		return res
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		b := body.Child(i)
		if b.Kind() != "block" {
			continue
		}
		btype, labels, bbody := tfBlock(b, src)
		loc := line(b)
		switch btype {
		case "resource":
			if len(labels) >= 2 {
				refsFrom(def(labels[0]+"."+labels[1], labels[0]+"."+labels[1], loc), bbody)
			}
		case "data":
			if len(labels) >= 2 {
				a := "data." + labels[0] + "." + labels[1]
				refsFrom(def(a, a, loc), bbody)
			}
		case "module":
			if len(labels) >= 1 {
				refsFrom(def("module."+labels[0], "module."+labels[0], loc), bbody)
			}
		case "variable":
			if len(labels) >= 1 {
				def("var."+labels[0], "var."+labels[0], loc)
			}
		case "output":
			if len(labels) >= 1 {
				refsFrom(def("output."+labels[0], "output."+labels[0], loc), bbody)
			}
		case "provider":
			if len(labels) >= 1 {
				def("provider."+labels[0], "provider."+labels[0], loc)
			}
		case "locals":
			if bbody == nil {
				continue
			}
			for j := uint(0); j < bbody.ChildCount(); j++ {
				a := bbody.Child(j)
				if a.Kind() != "attribute" {
					continue
				}
				key := tfChild(a, "identifier", src)
				if key == "" {
					continue
				}
				refsFrom(def("local."+key, "local."+key, line(a)), a)
			}
		}
	}
	return res
}

// tfBlock returns a block's type identifier, its quoted labels, and its body.
func tfBlock(n *ts.Node, src []byte) (btype string, labels []string, body *ts.Node) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Kind() {
		case "identifier":
			if btype == "" {
				btype = c.Utf8Text(src)
			}
		case "string_lit":
			labels = append(labels, strings.Trim(c.Utf8Text(src), `"`))
		case "body":
			body = c
		}
	}
	return
}

// tfRefAddress turns a variable_expr (plus its trailing get_attr chain) into a
// canonical block address, or "" for meta references (var/each/count/etc.).
func tfRefAddress(varExpr *ts.Node, src []byte) string {
	parts := []string{firstIdent(varExpr, src)}
	parent := varExpr.Parent()
	if parent != nil {
		after := false
		for i := uint(0); i < parent.ChildCount(); i++ {
			c := parent.Child(i)
			if c.Equals(*varExpr) {
				after = true
				continue
			}
			if !after {
				continue
			}
			if c.Kind() == "get_attr" {
				parts = append(parts, firstIdent(c, src))
			} else {
				break
			}
		}
	}
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	switch parts[0] {
	case "var", "local", "module":
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1]
		}
	case "data":
		if len(parts) >= 3 {
			return "data." + parts[1] + "." + parts[2]
		}
	case "each", "count", "self", "path", "terraform":
		return ""
	default: // resource reference: <type>.<name>
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1]
		}
	}
	return ""
}

func firstIdent(n *ts.Node, src []byte) string {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == "identifier" {
			return c.Utf8Text(src)
		}
	}
	return ""
}

// tfChild returns the text of n's first child of the given kind.
func tfChild(n *ts.Node, kind string, src []byte) string {
	if c := tfChildNode(n, kind); c != nil {
		return c.Utf8Text(src)
	}
	return ""
}

func tfChildNode(n *ts.Node, kind string) *ts.Node {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c.Kind() == kind {
			return c
		}
	}
	return nil
}

// dirScope is the directory-name scope for Terraform addresses.
func dirScope(rel string) string {
	if d := filepath.Base(filepath.Dir(rel)); d != "." && d != "/" && d != "" {
		return d
	}
	return "tf"
}
