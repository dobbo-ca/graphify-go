package extract

import (
	"strings"
	"unicode"

	ts "github.com/tree-sitter/go-tree-sitter"
)

// segState classifies a captured label segment.
type segState int

const (
	segUnknown segState = iota // value cannot be read from local literals
	segKnown                   // a definite literal value (possibly empty string)
	segEmpty                   // explicitly null / unset -> dropped from id
)

type segVal struct {
	val   string
	state segState
}

// labelInputs holds the cloudposse label fields captured from one module body.
type labelInputs struct {
	scalars    map[string]segVal // namespace, tenant, environment, stage, name
	attrs      []string          // literal attribute list (when attrState == segKnown)
	attrState  segState
	delimiter  string
	labelOrder []string
	valueCase  string
	hasContext bool // a `context =` attr is present -> some segments may be inherited
	unresolved bool // a knob (delimiter/label_order/label_value_case) is non-literal

	// Stage C carry: cross-module resolution inputs. varRefs maps a label field
	// (a labelScalar, or "attributes") to the bare variable name when its value
	// expression is exactly `var.<name>`. contextRef is the address a `context =`
	// attr points at (`module.<name>` or `var.<name>`), or "".
	varRefs    map[string]string
	contextRef string
}

var labelScalars = []string{"namespace", "tenant", "environment", "stage", "name"}
var defaultLabelOrder = []string{"namespace", "environment", "stage", "name", "attributes"}

// nullLabelInputs captures the null-label fields from a module body. Only literal
// values are read; anything dynamic (var/expr/interpolation) becomes UNKNOWN, and
// a non-literal knob marks the whole shape unresolved.
func nullLabelInputs(body *ts.Node, src []byte) labelInputs {
	exprs := map[string]*ts.Node{}
	if body != nil {
		for i := uint(0); i < body.NamedChildCount(); i++ {
			a := body.NamedChild(i)
			if a == nil || a.Kind() != "attribute" {
				continue
			}
			key := tfChild(a, "identifier", src)
			if key != "" && a.NamedChildCount() >= 2 {
				exprs[key] = a.NamedChild(1) // the value expression
			}
		}
	}

	in := labelInputs{
		scalars: map[string]segVal{}, delimiter: "-", valueCase: "lower",
		labelOrder: defaultLabelOrder, hasContext: exprs["context"] != nil,
		varRefs: map[string]string{},
	}

	for _, k := range labelScalars {
		e := exprs[k]
		if e == nil { // absent: dropped here; if a context attr exists, hasContext
			in.scalars[k] = segVal{state: segEmpty} // marks the id partial so Stage C fills it
			continue
		}
		v, st := classifyScalar(e, src)
		switch {
		case st == segKnown && v != "":
			in.scalars[k] = segVal{val: v, state: segKnown}
		case st == segKnown && v == "": // literal empty string -> dropped
			in.scalars[k] = segVal{state: segEmpty}
		case st == segEmpty: // explicit null -> dropped
			in.scalars[k] = segVal{state: segEmpty}
		default: // explicit var.X / expression -> referenced but unresolved
			in.scalars[k] = segVal{state: segUnknown}
			if vn := exprVarName(e, src); vn != "" { // Stage C: record `var.<name>`
				in.varRefs[k] = vn
			}
		}
	}

	// attributes (list): absent -> empty (hasContext marks the id partial)
	if e := exprs["attributes"]; e == nil {
		in.attrState = segEmpty
	} else {
		in.attrs, in.attrState = classifyList(e, src)
		if in.attrState == segUnknown {
			if vn := exprVarName(e, src); vn != "" { // Stage C: `attributes = var.<name>`
				in.varRefs["attributes"] = vn
			}
		}
	}

	// context: record the address it points at so Stage C can inherit segments.
	if e := exprs["context"]; e != nil {
		in.contextRef = exprRefAddress(e, src)
	}

	// knobs
	if e := exprs["delimiter"]; e != nil {
		v, st := classifyScalar(e, src)
		switch st {
		case segKnown:
			in.delimiter = v // includes explicit "" (no delimiter)
		case segEmpty:
			in.delimiter = "-" // null -> default
		default:
			in.unresolved = true
		}
	}
	if e := exprs["label_value_case"]; e != nil {
		v, st := classifyScalar(e, src)
		switch {
		case st == segKnown && v != "":
			in.valueCase = strings.ToLower(v)
		case st == segKnown || st == segEmpty:
			in.valueCase = "lower"
		default:
			in.unresolved = true
		}
	}
	if e := exprs["label_order"]; e != nil {
		vals, st := classifyList(e, src)
		switch st {
		case segKnown:
			if len(vals) > 0 {
				in.labelOrder = vals
			}
		case segEmpty:
			// null/[] -> keep default order
		default:
			in.unresolved = true
		}
	}
	return in
}

// composeID reconstructs the cloudposse id from captured inputs. It returns "" when
// the shape is unresolved or nothing is known; a partial id (with "{seg}" sentinels
// and a " (partial)" suffix) when some segments are unknown; or the exact id.
func composeID(in labelInputs) string {
	if in.unresolved {
		return ""
	}
	var parts []string
	known := 0
	partial := in.hasContext // context may add inherited segments not visible here
	for _, key := range in.labelOrder {
		if key == "attributes" {
			switch in.attrState {
			case segUnknown:
				parts = append(parts, "{attributes}")
				partial = true
			case segKnown:
				var norm []string
				seen := map[string]bool{}
				for _, a := range in.attrs {
					v := normalizeSeg(a, in.valueCase)
					if v == "" || seen[v] {
						continue
					}
					seen[v] = true
					norm = append(norm, v)
				}
				if len(norm) > 0 {
					parts = append(parts, strings.Join(norm, in.delimiter))
					known++
				}
			}
			continue
		}
		sv, ok := in.scalars[key]
		if !ok {
			continue // an unrecognised label_order entry
		}
		switch sv.state {
		case segKnown:
			v := normalizeSeg(sv.val, in.valueCase)
			if v == "" {
				continue
			}
			parts = append(parts, v)
			known++
		case segUnknown:
			parts = append(parts, "{"+key+"}")
			partial = true
		}
	}
	if known == 0 {
		return ""
	}
	id := strings.Join(parts, in.delimiter)
	if partial {
		id += " (partial)"
	}
	return id
}

// normalizeSeg strips chars outside [-a-zA-Z0-9] then applies the case mode,
// mirroring cloudposse's regex_replace_chars + label_value_case.
func normalizeSeg(v, caseMode string) string {
	var b strings.Builder
	for _, r := range v {
		if r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	s := b.String()
	switch caseMode {
	case "none":
		return s
	case "upper":
		return strings.ToUpper(s)
	case "title":
		return titleCase(s)
	default: // "lower" and any unrecognised value
		return strings.ToLower(s)
	}
}

// titleCase mirrors Terraform title(lower(v)). After regex stripping there are no
// spaces, so the value is a single word: uppercase the first rune, lowercase rest.
func titleCase(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(strings.ToLower(s))
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// classifyScalar reads a value expression: a pure literal string -> (text, segKnown)
// where text may be ""; a null literal -> ("", segEmpty); anything dynamic or a
// non-string literal -> ("", segUnknown).
func classifyScalar(expr *ts.Node, src []byte) (string, segState) {
	lv := tfNamedChild(expr, "literal_value")
	if lv == nil {
		return "", segUnknown // variable_expr, template_expr, function_call, ...
	}
	if sl := tfNamedChild(lv, "string_lit"); sl != nil {
		return stringLitText(sl, src), segKnown
	}
	if tfNamedChild(lv, "null_lit") != nil {
		return "", segEmpty
	}
	return "", segUnknown // numeric_lit / bool_lit are not string labels
}

// classifyList reads a value expression as a list of literal strings. A literal
// tuple of strings -> (vals, segKnown) (vals may be empty for []); a null -> empty;
// anything else (or a tuple with a non-literal element) -> UNKNOWN.
func classifyList(expr *ts.Node, src []byte) ([]string, segState) {
	cv := tfNamedChild(expr, "collection_value")
	if cv == nil {
		if lv := tfNamedChild(expr, "literal_value"); lv != nil && tfNamedChild(lv, "null_lit") != nil {
			return nil, segEmpty
		}
		return nil, segUnknown
	}
	tup := tfNamedChild(cv, "tuple")
	if tup == nil {
		return nil, segUnknown
	}
	var out []string
	for i := uint(0); i < tup.NamedChildCount(); i++ {
		el := tup.NamedChild(i)
		if el == nil || el.Kind() != "expression" {
			continue
		}
		v, st := classifyScalar(el, src)
		if st == segUnknown {
			return nil, segUnknown
		}
		if st == segKnown && v != "" {
			out = append(out, v)
		}
	}
	return out, segKnown
}

// stringLitText returns the literal text of a string_lit (concatenated
// template_literal children); "" for an empty string "".
func stringLitText(sl *ts.Node, src []byte) string {
	var b strings.Builder
	for i := uint(0); i < sl.NamedChildCount(); i++ {
		if c := sl.NamedChild(i); c != nil && c.Kind() == "template_literal" {
			b.WriteString(c.Utf8Text(src))
		}
	}
	return b.String()
}

// exprRefAddress returns the canonical reference address of a value expression
// that is exactly a single reference (e.g. `var.context` -> "var.context",
// `module.x.context` -> "module.x"), or "" if the expression is not a bare
// reference. It locates the leading variable_expr and reuses tfRefAddress.
func exprRefAddress(expr *ts.Node, src []byte) string {
	ve := tfNamedChild(expr, "variable_expr")
	if ve == nil {
		return ""
	}
	return tfRefAddress(ve, src)
}

// exprVarName returns the bare variable name when expr is exactly `var.<name>`,
// else "". It rejects deeper references like `var.x.y` or `module.x` so only a
// direct pass-through of a single input variable is treated as a var-ref.
func exprVarName(expr *ts.Node, src []byte) string {
	addr := exprRefAddress(expr, src)
	if name, ok := strings.CutPrefix(addr, "var."); ok && name != "" {
		return name
	}
	return ""
}

// tfNamedChild returns n's first named child of the given kind, or nil.
func tfNamedChild(n *ts.Node, kind string) *ts.Node {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c != nil && c.Kind() == kind {
			return c
		}
	}
	return nil
}
