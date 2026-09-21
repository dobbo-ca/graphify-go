package extract

import (
	"path/filepath"
	"regexp"
	"strings"

	tshcl "github.com/tree-sitter-grammars/tree-sitter-hcl/bindings/go"
	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
	"github.com/dobbo-ca/graphify-go/internal/security"
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

	// defNode adds a block node (once) with an optional computed name, plus a
	// contains edge from the file. def is the common case (no computed name).
	defNode := func(addr, label, loc, computed string) string {
		id := idutil.MakeID(scope, addr)
		if !seen[id] {
			seen[id] = true
			res.Nodes = append(res.Nodes, model.Node{
				ID: id, Label: label, FileType: "code", SourceFile: rel, SourceLocation: loc, ComputedName: computed,
			})
			res.Edges = append(res.Edges, model.Edge{
				Source: fileID, Target: id, Relation: "contains",
				Confidence: "EXTRACTED", SourceFile: rel, SourceLocation: loc,
			})
		}
		return id
	}
	def := func(addr, label, loc string) string { return defNode(addr, label, loc, "") }
	// refsFrom emits an edge from srcID to every address interpolated within node.
	refsFrom := func(srcID string, node *ts.Node) {
		walk(node, func(c *ts.Node) bool {
			if c.Kind() != "attribute" {
				return true
			}
			rel2 := "references"
			switch tfChild(c, "identifier", src) {
			case "depends_on":
				rel2 = "depends_on"
			case "context":
				rel2 = "inherits_context"
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
		owner := ""
		switch btype {
		case "resource":
			if len(labels) >= 2 {
				owner = def(labels[0]+"."+labels[1], labels[0]+"."+labels[1], loc)
				refsFrom(owner, bbody)
			}
		case "data":
			if len(labels) >= 2 {
				a := "data." + labels[0] + "." + labels[1]
				owner = def(a, a, loc)
				refsFrom(owner, bbody)
			}
		case "module":
			if len(labels) >= 1 {
				addr := "module." + labels[0]
				s := tfAttrString(bbody, "source", src)
				label := addr
				computed := ""
				var in labelInputs
				if isNullLabel(s) {
					label = addr + " [null-label]"
					in = nullLabelInputs(bbody, src)
					computed = composeID(in)
				}
				id := defNode(addr, label, loc, computed)
				owner = id
				refsFrom(id, bbody)
				if s != "" {
					res.ModRefs = append(res.ModRefs, ModRef{FromID: id, Source: s, File: rel, Loc: loc})
				}
				// Stage C carry: capture every invocation's args so a local
				// wrapper chain can be followed, and the null-label inputs so
				// its partial id can be completed from the caller.
				args, varRefs := moduleArgs(bbody, src)
				res.ModInvokes = append(res.ModInvokes, ModInvoke{
					NodeID: id, Dir: scope, Args: args, ArgVarRefs: varRefs,
				})
				if isNullLabel(s) {
					res.NullLabels = append(res.NullLabels, NullLabelRef{
						NodeID: id, Scope: scope, Inputs: in,
					})
				}
			}
		case "variable":
			if len(labels) >= 1 {
				owner = def("var."+labels[0], "var."+labels[0], loc)
			}
		case "output":
			if len(labels) >= 1 {
				owner = def("output."+labels[0], "output."+labels[0], loc)
				refsFrom(owner, bbody)
			}
		case "provider":
			if len(labels) >= 1 {
				owner = def("provider."+labels[0], "provider."+labels[0], loc)
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
		if attrs := blockAttributes(bbody, src, labels); owner != "" && attrs != nil {
			for i := range res.Nodes {
				if res.Nodes[i].ID == owner {
					res.Nodes[i].Attributes = attrs
					break
				}
			}
		}
	}
	return res
}

// maxAttributes caps how many attributes one block contributes, mirroring
// upstream's _METADATA_MAX_ATTRIBUTES / _METADATA_MAX_LIST_ITEMS. Value length
// is capped by security.SanitizeLabel.
const (
	maxAttributes = 100
	maxListItems  = 50
)

// sensitiveKeyRe matches attribute keys whose VALUE must never reach graph.json
// — this repo commits that artifact in CI, so a hardcoded `db_password` in a
// .tf would land in the tree and in an agent's context. The key survives
// redaction so `instance_type`/`ami` queries still work and a reader can still
// see THAT a secret is set. Mirrors upstream _SENSITIVE_KEY_RE.
var sensitiveKeyRe = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[-_]?key|access[-_]?key|private[-_]?key|credential|client[-_]?secret|connection[-_]?string|sas[-_]?token|auth|passphrase)`)

const redactedValue = "[redacted]"

// scriptKeys are bootstrap-script attributes: multi-line shell / cloud-init
// bodies with no query value that routinely carry a hardcoded credential.
// They are dropped outright rather than truncated.
var scriptKeys = map[string]bool{
	"user_data":                   true,
	"user_data_base64":            true,
	"user_data_replace_on_change": true,
	"custom_data":                 true,
	"metadata_startup_script":     true,
}

// blockAttributes reads a block body's direct attributes into a flat
// key -> value map for search. Nested objects are dropped (the flat map cannot
// hold them); sensitive keys keep the key and redact the value. A block is
// sensitive as a whole when one of its labels reads as a secret name
// (variable "db_password" carries its value in `default`) or when it declares
// Terraform's own `sensitive = true`.
func blockAttributes(body *ts.Node, src []byte, labels []string) map[string]string {
	if body == nil {
		return nil
	}
	blockSensitive := false
	for _, l := range labels {
		if sensitiveKeyRe.MatchString(l) {
			blockSensitive = true
		}
	}
	for i := uint(0); i < body.ChildCount() && !blockSensitive; i++ {
		a := body.Child(i)
		if a == nil || a.Kind() != "attribute" || a.NamedChildCount() < 2 {
			continue
		}
		if tfChild(a, "identifier", src) == "sensitive" && attrValue(attrValueNode(a), src) == "true" {
			blockSensitive = true
		}
	}
	attrs := map[string]string{}
	for i := uint(0); i < body.ChildCount(); i++ {
		a := body.Child(i)
		if a == nil || a.Kind() != "attribute" {
			continue
		}
		key := tfChild(a, "identifier", src)
		if key == "" || a.NamedChildCount() < 2 {
			continue
		}
		switch {
		case scriptKeys[key]:
			continue
		case (blockSensitive && key != "sensitive") || sensitiveKeyRe.MatchString(key):
			attrs[key] = redactedValue
		default:
			v := attrValue(attrValueNode(a), src)
			if v == "" {
				continue
			}
			attrs[key] = v
		}
		if len(attrs) >= maxAttributes {
			break
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

// attrValueNode returns an attribute's value expression: its first named child
// after the identifier that is not a comment (a comment between `=` and the
// value is itself a named child, so index 1 is not reliably the expression).
func attrValueNode(a *ts.Node) *ts.Node {
	for i := uint(1); i < a.NamedChildCount(); i++ {
		c := a.NamedChild(i)
		if c != nil && c.Kind() != "comment" {
			return c
		}
	}
	return nil
}

// attrValue renders an attribute value expression as one string: a literal
// string as its text, a literal tuple as its elements joined by ", ", and
// anything else (numbers, bools, var refs, interpolations, function calls) as
// its raw source text. Objects yield "" and are dropped by the caller.
func attrValue(e *ts.Node, src []byte) string {
	cur := e
	for cur != nil && cur.NamedChildCount() == 1 {
		switch cur.Kind() {
		case "expression", "literal_value", "collection_value":
			cur = cur.NamedChild(0)
			continue
		}
		break
	}
	if cur == nil {
		return ""
	}
	switch cur.Kind() {
	case "string_lit":
		return security.SanitizeLabel(stringLitText(cur, src))
	case "object":
		return ""
	case "tuple":
		var items []string
		for i := uint(0); i < cur.NamedChildCount() && len(items) < maxListItems; i++ {
			el := cur.NamedChild(i)
			if el == nil || el.Kind() != "expression" { // skips tuple_start/tuple_end
				continue
			}
			if v := attrValue(el, src); v != "" {
				items = append(items, v)
			}
		}
		return strings.Join(items, ", ")
	}
	return security.SanitizeLabel(strings.TrimSpace(cur.Utf8Text(src)))
}

// listSep joins/splits a list value carried inside a segVal (Args holds scalars
// and lists in one map; a list is encoded as its elements joined by listSep).
const listSep = "\x00"

// moduleArgs reads a module block body into Stage C carry data: literal scalar
// and list arguments (Args) and bare `arg = var.<name>` pass-throughs (varRefs).
// Non-literal, non-var-ref arguments are simply omitted (they are unresolvable
// from here).
func moduleArgs(body *ts.Node, src []byte) (args map[string]segVal, varRefs map[string]string) {
	args = map[string]segVal{}
	varRefs = map[string]string{}
	if body == nil {
		return
	}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		a := body.NamedChild(i)
		if a == nil || a.Kind() != "attribute" {
			continue
		}
		key := tfChild(a, "identifier", src)
		if key == "" || a.NamedChildCount() < 2 {
			continue
		}
		e := a.NamedChild(1) // value expression
		if key == "source" || key == "version" {
			continue
		}
		if vn := exprVarName(e, src); vn != "" { // arg = var.<name>
			varRefs[key] = vn
			continue
		}
		// literal scalar?
		if v, st := classifyScalar(e, src); st != segUnknown {
			if st == segKnown && v != "" {
				args[key] = segVal{val: v, state: segKnown}
			} else {
				args[key] = segVal{state: segEmpty}
			}
			continue
		}
		// literal list (e.g. attributes = ["1","2"])?
		if vals, st := classifyList(e, src); st != segUnknown {
			args[key] = segVal{val: strings.Join(vals, listSep), state: segKnown}
		}
	}
	return
}

// isNullLabel reports whether a module source is the cloudposse null-label
// module — the registry form "cloudposse/label/null" or any git/github form of
// "terraform-null-label" (with or without a ?ref= version).
func isNullLabel(source string) bool {
	return strings.Contains(source, "cloudposse/label/null") ||
		strings.Contains(source, "terraform-null-label")
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

// tfAttrString returns the literal string value of the attribute named key in
// body, or "" if absent. Used for a module's `source`, which Terraform requires
// to be a literal (no interpolation), so taking the first string_lit is safe.
func tfAttrString(body *ts.Node, key string, src []byte) string {
	if body == nil {
		return ""
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		a := body.Child(i)
		if a == nil || a.Kind() != "attribute" || tfChild(a, "identifier", src) != key {
			continue
		}
		var out string
		walk(a, func(n *ts.Node) bool {
			if out != "" {
				return false
			}
			if n.Kind() == "string_lit" {
				out = strings.Trim(n.Utf8Text(src), `"`)
				return false
			}
			return true
		})
		return out
	}
	return ""
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

// dirScope is the directory scope for Terraform addresses. Terraform addresses
// are module(directory)-scoped, so same-directory files share a scope and their
// cross-file references resolve once merged. The scope is the file's full
// directory path (not just its base name) so that two directories sharing a base
// name — e.g. workspaces/scalr-agents and modules/scalr-agents — do not collide.
func dirScope(rel string) string {
	if d := filepath.ToSlash(filepath.Dir(rel)); d != "." && d != "/" && d != "" {
		return d
	}
	return "tf"
}
