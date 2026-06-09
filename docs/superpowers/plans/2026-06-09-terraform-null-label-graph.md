# Terraform cloudposse null-label Graph Awareness — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make graphify's Terraform graph detect cloudposse null-label modules, tag them, model `context =` inheritance, and reconstruct the computed deployed name (`namespace-environment-stage-name-attributes`) so it is searchable.

**Architecture:** Build on existing Terraform extraction (`internal/extract/terraform.go` + `resolve.go`), which already captures module `source` (`ModRef`) and links module invocations to their target dir/concept nodes. Stage A adds a `[null-label]` marker + an `inherits_context` edge relation. Stage B adds a `ComputedName` node field and a single-block literal name reconstructor (`internal/extract/nulllabel.go`) faithful to cloudposse's `id` algorithm. Stage C adds a whole-corpus pass that fills unknown segments by following `inherits_context` and module-source edges.

**Tech Stack:** Go 1.x, tree-sitter-grammars/tree-sitter-hcl v1.2.0, tree-sitter/go-tree-sitter v0.25.0.

**Spec:** `docs/superpowers/specs/2026-06-09-terraform-null-label-graph-design.md`

**Worktree/branch:** `worktree-tf-null-label-graph-4e7a` (already set up; baseline `go test ./...` green = 63 tests).

---

## Reference: cloudposse `id` algorithm (what composeID reimplements)

```
id = join(delimiter, [ format(label) for label in label_order if length > 0 ])
```
- Valid labels: `namespace, tenant, environment, stage, name, attributes`.
- Default `label_order` = `["namespace","environment","stage","name","attributes"]` (tenant excluded by default).
- Default `delimiter` = `-`. `attributes` (a list) collapse into ONE slot via `join(delimiter, attributes)`.
- Each segment: `regex_replace(value, /[^-a-zA-Z0-9]/, "")` then case (default `lower`; also `upper`, `title`, `none`).
- Empty segments dropped.
- We do NOT replicate `id_length_limit` truncation/hash.
- Confirmed verbatim against `main.tf`: `attributes = join(local.delimiter, local.attributes)`, `labels = [for l in local.label_order : local.id_context[l] if length(local.id_context[l]) > 0]`, `id_full = join(local.delimiter, local.labels)`.

Worked examples (used as test goldens):
- `name="asdf" attributes=["1","2"] delimiter="!"` (default order) → `asdf!1!2`
- same inputs, `label_order=["attributes","name"]` → `1!2!asdf`
- `label_order=["tenant","namespace","name"] tenant="acme" namespace="eg" name="app"` → `acme-eg-app`

## Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically)

- `attribute` → `NamedChild(0)` = `identifier` (key), `NamedChild(1)` = `expression` (value).
- literal string: `expression > literal_value > string_lit > template_literal` (value = concat of `template_literal` children; `""` has none).
- literal list: `expression > collection_value > tuple > (expression > literal_value > string_lit)*`.
- variable ref: `expression > variable_expr (+ get_attr)` — no `literal_value`.
- interpolation: `expression > template_expr` (contains `template_interpolation`).
- `null`/number/bool: `expression > literal_value > null_lit | numeric_lit | bool_lit`.

---

# Stage A — Foundation: null-label marker + inherits_context edge

## Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes

**Files:**
- Modify: `internal/extract/terraform.go` (add helper; module arm `:99-106`)
- Test: `internal/extract/terraform_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/extract/terraform_test.go`:

```go
func TestExtractTerraformNullLabelMarker(t *testing.T) {
	src := []byte(`
module "this" {
  source  = "cloudposse/label/null"
  version = "0.25.0"
}
module "label" {
  source = "git::https://github.com/cloudposse/terraform-null-label.git?ref=tags/0.25.0"
}
module "plain" {
  source = "../vpc"
}
`)
	res := FileFromBytes("main.tf", src)
	labels := map[string]bool{}
	for _, n := range res.Nodes {
		labels[n.Label] = true
	}
	if !labels["module.this [null-label]"] {
		t.Error("expected module.this tagged [null-label] (registry source)")
	}
	if !labels["module.label [null-label]"] {
		t.Error("expected module.label tagged [null-label] (git source)")
	}
	if !labels["module.plain"] {
		t.Error("expected module.plain to be untagged")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/extract/ -run TestExtractTerraformNullLabelMarker -v`
Expected: FAIL — labels show `module.this`, `module.label` (no `[null-label]` suffix).

- [ ] **Step 3: Add the helper**

In `internal/extract/terraform.go`, add after `isLocalSource`'s neighbours (anywhere at file scope, e.g. just below `tfAttrString`):

```go
// isNullLabel reports whether a module source is the cloudposse null-label
// module — the registry form "cloudposse/label/null" or any git/github form of
// "terraform-null-label" (with or without a ?ref= version).
func isNullLabel(source string) bool {
	return strings.Contains(source, "cloudposse/label/null") ||
		strings.Contains(source, "terraform-null-label")
}
```

(`strings` is already imported.)

- [ ] **Step 4: Use it in the module arm**

Replace the `case "module":` block (`terraform.go:99-106`) with:

```go
		case "module":
			if len(labels) >= 1 {
				addr := "module." + labels[0]
				s := tfAttrString(bbody, "source", src)
				label := addr
				if isNullLabel(s) {
					label = addr + " [null-label]"
				}
				id := def(addr, label, loc)
				refsFrom(id, bbody)
				if s != "" {
					res.ModRefs = append(res.ModRefs, ModRef{FromID: id, Source: s, File: rel, Loc: loc})
				}
			}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/extract/ -run TestExtractTerraformNullLabelMarker -v`
Expected: PASS.

- [ ] **Step 6: Run the full extract suite (no regressions)**

Run: `go test ./internal/extract/`
Expected: PASS (existing `TestExtractTerraformModuleSource` etc. still green — IDs unchanged).

- [ ] **Step 7: Commit**

```bash
git add internal/extract/terraform.go internal/extract/terraform_test.go
git commit -m "feat(extract): tag cloudposse null-label modules in the TF graph"
```

## Task A2: `inherits_context` edge relation for `context =`

**Files:**
- Modify: `internal/extract/terraform.go` (`refsFrom`, `:55-58`)
- Test: `internal/extract/terraform_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/extract/terraform_test.go`:

```go
func TestExtractTerraformInheritsContext(t *testing.T) {
	src := []byte(`
module "this" {
  source = "cloudposse/label/null"
}
module "label" {
  source  = "cloudposse/label/null"
  context = module.this.context
}
`)
	res := FileFromBytes("main.tf", src)
	id2label := map[string]string{}
	for _, n := range res.Nodes {
		id2label[n.ID] = n.Label
	}
	found := false
	sawDependsRel := false
	for _, e := range res.Edges {
		if e.Relation == "inherits_context" &&
			id2label[e.Source] == "module.label [null-label]" &&
			id2label[e.Target] == "module.this [null-label]" {
			found = true
		}
		if e.Relation == "depends_on" {
			sawDependsRel = true
		}
	}
	if !found {
		t.Error("expected module.label --inherits_context--> module.this")
	}
	_ = sawDependsRel // (depends_on path must remain intact; covered by TestExtractTerraform)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/extract/ -run TestExtractTerraformInheritsContext -v`
Expected: FAIL — edge relation is `references`, not `inherits_context`.

- [ ] **Step 3: Make the relation selection 3-way**

In `internal/extract/terraform.go`, replace the relation selection inside `refsFrom` (`:55-58`):

```go
			rel2 := "references"
			if tfChild(c, "identifier", src) == "depends_on" {
				rel2 = "depends_on"
			}
```

with:

```go
			rel2 := "references"
			switch tfChild(c, "identifier", src) {
			case "depends_on":
				rel2 = "depends_on"
			case "context":
				rel2 = "inherits_context"
			}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/extract/ -run TestExtractTerraformInheritsContext -v`
Expected: PASS.

- [ ] **Step 5: Run the full extract suite**

Run: `go test ./internal/extract/`
Expected: PASS (existing `depends_on`/`references` assertions in `TestExtractTerraform` still hold).

- [ ] **Step 6: Commit**

```bash
git add internal/extract/terraform.go internal/extract/terraform_test.go
git commit -m "feat(extract): model TF context= inheritance as inherits_context edges"
```

---

# Stage B — Single-block literal name reconstruction

## Task B1: `ComputedName` node field threaded through export + query

**Files:**
- Modify: `internal/model/model.go` (`Node`, `:9-15`)
- Modify: `internal/export/export.go` (`jsonNode` `:21-29`; `ToJSON` node loop `:66-70`)
- Modify: `internal/query/query.go` (`Node` `:28-36`)
- Test: `internal/export/export_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/export/export_test.go`:

```go
func TestToJSONComputedName(t *testing.T) {
	g := model.New()
	g.AddNode(model.Node{ID: "m1", Label: "module.this [null-label]", FileType: "code", SourceFile: "main.tf", SourceLocation: "L1", ComputedName: "eg-prod-app"})
	communities := map[int][]string{0: {"m1"}}

	path := filepath.Join(t.TempDir(), "graph.json")
	if err := ToJSON(g, communities, path, "deadbeefcafe"); err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var out jsonGraph
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(out.Nodes))
	}
	n := out.Nodes[0]
	if n.ComputedName != "eg-prod-app" {
		t.Errorf("computed_name = %q, want eg-prod-app", n.ComputedName)
	}
	// The computed name must be folded into norm_label so search finds it.
	if !strings.Contains(n.NormLabel, "eg-prod-app") {
		t.Errorf("norm_label = %q, want it to contain eg-prod-app", n.NormLabel)
	}
}
```

Add `"strings"` to the `export_test.go` import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/export/ -run TestToJSONComputedName -v`
Expected: FAIL to compile — `model.Node` has no `ComputedName`, `jsonNode` has no `ComputedName`.

- [ ] **Step 3: Add the field to model.Node**

In `internal/model/model.go`, the `Node` struct becomes:

```go
type Node struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	FileType       string `json:"file_type"`
	SourceFile     string `json:"source_file"`
	SourceLocation string `json:"source_location,omitempty"`
	ComputedName   string `json:"computed_name,omitempty"`
}
```

- [ ] **Step 4: Add it to export's jsonNode and fold into norm_label**

In `internal/export/export.go`, `jsonNode` becomes:

```go
type jsonNode struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	FileType       string `json:"file_type"`
	SourceFile     string `json:"source_file"`
	SourceLocation string `json:"source_location,omitempty"`
	Community      *int   `json:"community"`
	NormLabel      string `json:"norm_label"`
	ComputedName   string `json:"computed_name,omitempty"`
}
```

And the node-build loop (`:66-70`) becomes:

```go
		nl := normLabel(n.Label)
		if n.ComputedName != "" {
			nl = nl + " " + normLabel(n.ComputedName)
		}
		out.Nodes = append(out.Nodes, jsonNode{
			ID: n.ID, Label: n.Label, FileType: n.FileType,
			SourceFile: n.SourceFile, SourceLocation: n.SourceLocation,
			Community: comm, NormLabel: nl, ComputedName: n.ComputedName,
		})
```

- [ ] **Step 5: Add the field to query.Node (display round-trip)**

In `internal/query/query.go`, the `Node` struct gains a final field:

```go
type Node struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	FileType       string `json:"file_type"`
	SourceFile     string `json:"source_file"`
	SourceLocation string `json:"source_location"`
	Community      *int   `json:"community"`
	NormLabel      string `json:"norm_label"`
	ComputedName   string `json:"computed_name"`
}
```

(No `Query` change — search already matches `NormLabel`, which now carries the computed name.)

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/export/ ./internal/query/ -v`
Expected: PASS (incl. existing `TestToJSON`, `query_test.go`).

- [ ] **Step 7: Commit**

```bash
git add internal/model/model.go internal/export/export.go internal/query/query.go internal/export/export_test.go
git commit -m "feat(model): add searchable ComputedName node field for TF labels"
```

## Task B2: `nulllabel.go` — capture + composeID (the core reconstructor)

**Files:**
- Create: `internal/extract/nulllabel.go`
- Test: `internal/extract/nulllabel_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/extract/nulllabel_test.go`:

```go
package extract

import "testing"

// composeFromHCL parses a single module body and returns its reconstructed id.
func composeFromHCL(t *testing.T, body string) string {
	t.Helper()
	res := FileFromBytes("main.tf", []byte("module \"this\" {\n  source = \"cloudposse/label/null\"\n"+body+"\n}\n"))
	for _, n := range res.Nodes {
		if n.Label == "module.this [null-label]" {
			return n.ComputedName
		}
	}
	t.Fatalf("no null-label module node found")
	return ""
}

func TestComposeID(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"defaultOrder", `name = "asdf"` + "\n" + `attributes = ["1","2"]` + "\n" + `delimiter = "!"`, "asdf!1!2"},
		{"reorder", `name = "asdf"` + "\n" + `attributes = ["1","2"]` + "\n" + `delimiter = "!"` + "\n" + `label_order = ["attributes","name"]`, "1!2!asdf"},
		{"tenantViaOrder", `namespace = "eg"` + "\n" + `name = "app"` + "\n" + `tenant = "acme"` + "\n" + `label_order = ["tenant","namespace","name"]`, "acme-eg-app"},
		{"defaultDelim", `namespace = "eg"` + "\n" + `stage = "prod"` + "\n" + `name = "app"`, "eg-prod-app"},
		{"emptyDropped", `namespace = "eg"` + "\n" + `stage = ""` + "\n" + `name = "app"`, "eg-app"},
		{"lowercased", `namespace = "EG"` + "\n" + `name = "App"`, "eg-app"},
		{"regexStripped", `namespace = "eg_x"` + "\n" + `name = "a.b"`, "egx-ab"},
		{"caseNone", `namespace = "EG"` + "\n" + `name = "App"` + "\n" + `label_value_case = "none"`, "EG-App"},
		{"attrsDedup", `name = "app"` + "\n" + `attributes = ["a","a","b"]`, "app-a-b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := composeFromHCL(t, c.body); got != c.want {
				t.Errorf("composeID(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestComposeIDPartial(t *testing.T) {
	// A var-sourced segment with a context attr present => UNKNOWN segment, partial id.
	got := composeFromHCL(t, `namespace = var.ns`+"\n"+`name = "app"`+"\n"+`context = var.context`)
	if got != "{namespace}-app (partial)" {
		t.Errorf("partial id = %q, want {namespace}-app (partial)", got)
	}
}

func TestComposeIDUnresolved(t *testing.T) {
	// Non-literal label_order => shape unknowable => no id emitted.
	got := composeFromHCL(t, `name = "app"`+"\n"+`label_order = var.order`)
	if got != "" {
		t.Errorf("unresolved id = %q, want empty", got)
	}
}

func TestComposeIDNoLiterals(t *testing.T) {
	// Nothing known (only var inputs, no context) => no useless all-sentinel id.
	got := composeFromHCL(t, `namespace = var.ns`+"\n"+`name = var.name`)
	if got != "" {
		t.Errorf("no-literals id = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/extract/ -run TestComposeID -v`
Expected: FAIL to compile — `composeID`/`nullLabelInputs` undefined, and `module.this` has empty `ComputedName` (B2 wiring not done yet).

- [ ] **Step 3: Create `internal/extract/nulllabel.go`**

```go
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
		}
	}

	// attributes (list): absent -> empty (hasContext marks the id partial)
	if e := exprs["attributes"]; e == nil {
		in.attrState = segEmpty
	} else {
		in.attrs, in.attrState = classifyList(e, src)
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

// tfNamedChild returns n's first named child of the given kind, or nil.
func tfNamedChild(n *ts.Node, kind string) *ts.Node {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c != nil && c.Kind() == kind {
			return c
		}
	}
	return nil
}
```

- [ ] **Step 4: Wire it into the module arm**

In `internal/extract/terraform.go`, generalise the `def` closure (`:35-48`) by adding `defNode` and keeping `def` as a wrapper. Replace the `def := func(...) {...}` block with:

```go
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
```

Then update the `case "module":` arm (from Task A1) to compute and pass the name:

```go
		case "module":
			if len(labels) >= 1 {
				addr := "module." + labels[0]
				s := tfAttrString(bbody, "source", src)
				label := addr
				computed := ""
				if isNullLabel(s) {
					label = addr + " [null-label]"
					computed = composeID(nullLabelInputs(bbody, src))
				}
				id := defNode(addr, label, loc, computed)
				refsFrom(id, bbody)
				if s != "" {
					res.ModRefs = append(res.ModRefs, ModRef{FromID: id, Source: s, File: rel, Loc: loc})
				}
			}
```

- [ ] **Step 5: Run the reconstructor tests**

Run: `go test ./internal/extract/ -run 'TestComposeID' -v`
Expected: PASS for `TestComposeID` (all sub-cases), `TestComposeIDPartial`, `TestComposeIDUnresolved`, `TestComposeIDNoLiterals`.

- [ ] **Step 6: Run the full extract suite**

Run: `go test ./internal/extract/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/extract/nulllabel.go internal/extract/nulllabel_test.go internal/extract/terraform.go
git commit -m "feat(extract): reconstruct cloudposse null-label computed names (single block)"
```

## Task B3: end-to-end searchability test

**Files:**
- Test: `internal/extract/nulllabel_test.go`

- [ ] **Step 1: Write the test**

Append to `internal/extract/nulllabel_test.go` (no new imports needed — the file already imports `testing`):

```go
func TestNullLabelComputedNameOnNode(t *testing.T) {
	src := []byte(`
module "this" {
  source     = "cloudposse/label/null"
  namespace  = "eg"
  stage      = "prod"
  name       = "app"
  attributes = ["public"]
}
resource "aws_s3_bucket" "b" {
  bucket = module.this.id
}
`)
	res := FileFromBytes("main.tf", src)
	var got string
	for _, n := range res.Nodes {
		if n.Label == "module.this [null-label]" {
			got = n.ComputedName
		}
	}
	if got != "eg-prod-app-public" {
		t.Fatalf("ComputedName = %q, want eg-prod-app-public", got)
	}
	// And the resource still links to the label module (existing behaviour).
	id2label := map[string]string{}
	for _, n := range res.Nodes {
		id2label[n.ID] = n.Label
	}
	linked := false
	for _, e := range res.Edges {
		if e.Relation == "references" && id2label[e.Source] == "aws_s3_bucket.b" && id2label[e.Target] == "module.this [null-label]" {
			linked = true
		}
	}
	if !linked {
		t.Error("expected aws_s3_bucket.b --references--> module.this")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/extract/ -run TestNullLabelComputedNameOnNode -v`
Expected: PASS — `ComputedName == "eg-prod-app-public"` and the reference edge holds.

- [ ] **Step 3: Full suite**

Run: `go test ./...`
Expected: PASS (still 63 + new tests).

- [ ] **Step 4: Commit**

```bash
git add internal/extract/nulllabel_test.go
git commit -m "test(extract): null-label computed name end-to-end on the module node"
```

## Task B4: real-world fixture + manual graph smoke test

**Files:**
- Create: `internal/extract/testdata/tf/label/main.tf`
- Test: `internal/extract/nulllabel_test.go`

- [ ] **Step 1: Create the fixture**

`internal/extract/testdata/tf/label/main.tf`:

```hcl
module "this" {
  source      = "cloudposse/label/null"
  version     = "0.25.0"
  namespace   = "eg"
  environment = "ue1"
  stage       = "prod"
  name        = "app"
  attributes  = ["public"]
  delimiter   = "-"
}

resource "aws_s3_bucket" "default" {
  bucket = module.this.id
  tags   = module.this.tags
}
```

- [ ] **Step 2: Write the test**

Append to `internal/extract/nulllabel_test.go`:

```go
func TestNullLabelFixture(t *testing.T) {
	r, err := File("testdata/tf/label", "main.tf")
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	ext := Resolve([]Result{r}, []string{"label/main.tf"})
	var got string
	for _, n := range ext.Nodes {
		if n.Label == "module.this [null-label]" {
			got = n.ComputedName
		}
	}
	if got != "eg-ue1-prod-app-public" {
		t.Fatalf("fixture ComputedName = %q, want eg-ue1-prod-app-public", got)
	}
}
```

- [ ] **Step 3: Run it**

Run: `go test ./internal/extract/ -run TestNullLabelFixture -v`
Expected: PASS.

- [ ] **Step 4: Manual smoke test against the real CLI** (optional but recommended)

Run:
```bash
go run ./cmd/graphify build internal/extract/testdata
go run ./cmd/graphify query 'eg-ue1-prod-app'
```
Expected: `query` lists the `module.this [null-label]` node (matched via `norm_label`).
Then clean up any generated `graphify-out` under testdata if it is not gitignored there: `git status` and discard unintended artifacts.

- [ ] **Step 5: Commit**

```bash
git add internal/extract/testdata/tf/label/main.tf internal/extract/nulllabel_test.go
git commit -m "test(extract): null-label fixture + computed-name search smoke"
```

---

# Stage C — Whole-corpus context-chain reconstruction

> **Highest-uncertainty phase.** It depends on B's realized `labelInputs`/`composeID`. Before starting, re-read `nulllabel.go` as implemented and adjust the structs below if B drifted. Each task is still TDD; validate the carry structs against real fixtures.

Stage C fills the UNKNOWN segments B leaves, by following context inheritance:
1. **Same-corpus `module.X.context`** — a null-label/wrapper `module.this` declared in the same dir (or any dir in the corpus) whose own inputs are literal.
2. **Caller-arg threading** — a `var.X` (or `var.context`) input whose value is supplied by a caller's module invocation, reached via the existing module→dir `references` edge.

## Task C1: capture var-references and invocation args

**Files:**
- Modify: `internal/extract/nulllabel.go` (extend `labelInputs` + capture)
- Modify: `internal/extract/extract.go` (add carry structs to `Result`)
- Modify: `internal/extract/terraform.go` (populate carry on every module + null-label module)
- Test: `internal/extract/nulllabel_test.go`

Add to `labelInputs` a parallel map of var-references (segment → variable name) and a context reference:

```go
// in labelInputs:
varRefs    map[string]string // scalar/attr key -> referenced var name (when value is var.X)
attrVarRef string            // var name when attributes = var.X
contextRef string            // address from `context = <ref>`: "module.<name>", "var.<name>", or ""
```

Capture rules (extend `nullLabelInputs`): when `classifyScalar` returns `segUnknown` AND the value expression is exactly `var.<name>` (a `variable_expr` "var" + a single `get_attr`), record `varRefs[key] = name` (reuse `tfRefAddress` to read `var.<name>` then strip the `var.` prefix). Likewise for `attributes = var.x`. For `context`, set `contextRef` from `tfRefAddress` of its value (`module.<name>` or `var.<name>`).

Carry structs in `extract.go`:

```go
// NullLabelRef is a captured null-label module invocation awaiting cross-module
// context resolution in Resolve.
type NullLabelRef struct {
	NodeID string
	Scope  string // dirScope of the declaring file
	File   string
	Inputs labelInputs
}

// ModInvoke is any module invocation's literal/var args, used to thread a caller's
// values into a wrapper module's variables.
type ModInvoke struct {
	NodeID string
	Scope  string
	Dir    string            // the file's directory (path.Dir of File), for source matching
	Args   map[string]segVal // arg name -> literal value/state
}
```

Add `NullLabels []NullLabelRef` and `ModInvokes []ModInvoke` to `Result`. Populate both in the `case "module":` arm: always append a `ModInvoke` (capture every arg via `classifyScalar`); when `isNullLabel`, also append a `NullLabelRef` with the captured `labelInputs`.

- [ ] **Step 1–5 (TDD):** Write a unit test asserting `nullLabelInputs` records `varRefs["namespace"]=="namespace"` for `namespace = var.namespace` and `contextRef=="module.this"` for `context = module.this.context`; run (fails); implement capture; run (passes); commit.

```bash
git commit -m "feat(extract): capture null-label var-refs and module invocation args"
```

## Task C2: resolver pass in Resolve

**Files:**
- Create: `internal/extract/nulllabel_resolve.go`
- Modify: `internal/extract/resolve.go` (call the resolver near the end, before `return out`)
- Test: `internal/extract/nulllabel_test.go`

Algorithm (`resolveNullLabels(results []Result, files []string, out *model.Extraction)`):

1. Index: `byContextAddr[scope+"\x00module."+name] -> NullLabelRef`; `invokeByDir[dir][argMatchesModuleName]` — more practically, index `ModInvoke`s and the module→dir `references` edges already in `out.Edges` (relation `references`, target = a `tfmodule` dir node whose `Label` is the dir path). For each null-label node, find the caller invocation by locating the module-source `references` edge whose target dir equals the null-label node's dir.
2. For each `NullLabelRef` whose current `composeID(Inputs)` is non-exact (empty or contains `{`), accumulate context:
   - **Local `module.X.context`:** if `contextRef == "module.<x>"` and `byContextAddr[scope+module.x]` exists, merge that ref's resolved inputs as the parent (override-else-inherit for scalars + knobs; parent-first concat+dedup for attrs).
   - **Caller threading:** for each segment with a `varRefs[seg]` and an identifiable caller `ModInvoke`, substitute the caller's arg value (`Args[varName]`). For `var.context`, follow the caller's `context` arg recursively.
   - **Stop** at the first hop with no resolvable parent/caller, or a non-local module source (registry/git) — leave those segments UNKNOWN.
3. Recompute `composeID` on the merged inputs and, if improved, update the node's `ComputedName` in `out.Nodes` (find by `NodeID`).

Guard recursion depth (e.g. max 8 hops) to avoid cycles.

- [ ] **Step 1: Write the failing test** — a 2-dir fixture: caller in `root/main.tf` invokes `module "label" { source = "../modules/label"; namespace="eg"; stage="prod"; name="app"; attributes=["1"] }`; `modules/label/main.tf` has `module "this" { source="cloudposse/label/null"; namespace=var.namespace; stage=var.stage; name=var.name; attributes=var.attributes }` + the matching `variable` blocks. Assert the `module.this` node's `ComputedName == "eg-prod-app-1"` after `Resolve`.
- [ ] **Step 2: Run** (fails — `ComputedName` is empty/partial without C2).
- [ ] **Step 3: Implement** `nulllabel_resolve.go` + call it from `Resolve` before `return out`.
- [ ] **Step 4: Run** (passes).
- [ ] **Step 5: Add a partial-stop test** — chain hits a `var.namespace` with no caller arg → `ComputedName` stays `{namespace}-...-... (partial)`; and a chain hitting a registry source stops cleanly.
- [ ] **Step 6: Full suite** `go test ./...` green.
- [ ] **Step 7: Commit**

```bash
git commit -m "feat(extract): resolve null-label names across local module chains"
```

## Task C3: docs + changelog

**Files:**
- Modify: `CLAUDE.md` or `skills/graphify/SKILL.md` (note null-label search), `GOALS.md` (mark follow-up)

- [ ] Add a short note that null-label modules are tagged `[null-label]` and searchable by their reconstructed `computed_name`; mark the GOALS follow-up done. Commit:

```bash
git commit -m "docs: document null-label tagging and computed-name search"
```

---

## Final verification (run before opening a PR)

- [ ] `go build ./...` — clean.
- [ ] `go vet ./...` — clean.
- [ ] `go test ./...` — all green.
- [ ] `git log --oneline` — one commit per task, on `worktree-tf-null-label-graph-4e7a`.
- [ ] Regenerate the repo's own graph is NOT needed — CI does it on merge; do not hand-edit `graphify-out/graph.json`.

## Self-review notes (author)

- **Spec coverage:** marker (A1) ✓, inherits_context (A2) ✓, ComputedName storage + norm_label fold (B1) ✓, single-block reconstruction incl. label_order/tenant/delimiter/case/attributes-merge/partial/unresolved (B2 + tests) ✓, end-to-end search (B3/B4) ✓, chain reconstruction (C1/C2) ✓.
- **Type consistency:** `defNode(addr,label,loc,computed)` + `def` wrapper used consistently; `labelInputs`/`segVal`/`segState` defined in B2 and extended (not redefined) in C1; `composeID`/`nullLabelInputs` signatures stable across B2→C.
- **Known risk:** Stage C's caller-threading is the least certain; its tasks are coarser by design and must be validated against the real B implementation and fixtures before finalising.
