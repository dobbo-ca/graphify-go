package extract

import (
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

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
	got := composeFromHCL(t, `namespace = var.ns`+"\n"+`name = "app"`+"\n"+`context = var.context`)
	if got != "{namespace}-app (partial)" {
		t.Errorf("partial id = %q, want {namespace}-app (partial)", got)
	}
}

func TestComposeIDUnresolved(t *testing.T) {
	got := composeFromHCL(t, `name = "app"`+"\n"+`label_order = var.order`)
	if got != "" {
		t.Errorf("unresolved id = %q, want empty", got)
	}
}

func TestComposeIDNoLiterals(t *testing.T) {
	got := composeFromHCL(t, `namespace = var.ns`+"\n"+`name = var.name`)
	if got != "" {
		t.Errorf("no-literals id = %q, want empty", got)
	}
}

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

// computedNameOf returns the ComputedName of the null-label module node living
// in dir, or "" if not found. The node ID encodes its dir scope, so two
// null-label modules in different dirs are distinguishable.
func computedNameOf(ext model.Extraction, dir string) string {
	want := idutil.MakeID(dir, "module.this")
	for _, n := range ext.Nodes {
		if n.ID == want {
			return n.ComputedName
		}
	}
	return ""
}

func TestNullLabelChainResolved(t *testing.T) {
	root := []byte(`
module "label" {
  source     = "../modules/label"
  namespace  = "eg"
  stage      = "prod"
  name       = "app"
  attributes = ["1"]
}
`)
	wrapper := []byte(`
variable "namespace" {}
variable "stage" {}
variable "name" {}
variable "attributes" { default = [] }

module "this" {
  source     = "cloudposse/label/null"
  namespace  = var.namespace
  stage      = var.stage
  name       = var.name
  attributes = var.attributes
}
`)
	rRoot := FileFromBytes("root/main.tf", root)
	rWrap := FileFromBytes("modules/label/main.tf", wrapper)
	ext := Resolve([]Result{rRoot, rWrap}, []string{"root/main.tf", "modules/label/main.tf"})
	if got := computedNameOf(ext, "modules/label"); got != "eg-prod-app-1" {
		t.Fatalf("chain ComputedName = %q, want eg-prod-app-1", got)
	}
}

func TestNullLabelChainPartialStop(t *testing.T) {
	root := []byte(`
module "label" {
  source     = "../modules/label"
  namespace  = var.something
  stage      = "prod"
  name       = "app"
  attributes = ["1"]
}
`)
	wrapper := []byte(`
variable "namespace" {}
variable "stage" {}
variable "name" {}
variable "attributes" { default = [] }

module "this" {
  source     = "cloudposse/label/null"
  namespace  = var.namespace
  stage      = var.stage
  name       = var.name
  attributes = var.attributes
}
`)
	rRoot := FileFromBytes("root/main.tf", root)
	rWrap := FileFromBytes("modules/label/main.tf", wrapper)
	ext := Resolve([]Result{rRoot, rWrap}, []string{"root/main.tf", "modules/label/main.tf"})
	got := computedNameOf(ext, "modules/label")
	if !strings.Contains(got, "{namespace}") {
		t.Fatalf("partial-stop ComputedName = %q, want it to contain {namespace}", got)
	}
	if !strings.HasSuffix(got, "(partial)") {
		t.Fatalf("partial-stop ComputedName = %q, want it to end with (partial)", got)
	}
}

func TestNullLabelInheritPartialParent(t *testing.T) {
	src := []byte(`
variable "x" {}
module "base" {
  source      = "cloudposse/label/null"
  namespace   = var.x
  environment = "ue1"
}
module "this" {
  source  = "cloudposse/label/null"
  stage   = "prod"
  name    = "app"
  context = module.base.context
}
`)
	ext := Resolve([]Result{FileFromBytes("main.tf", src)}, []string{"main.tf"})
	got := computedNameOf(ext, "tf")
	if !strings.Contains(got, "{namespace}") {
		t.Fatalf("inherit-partial-parent ComputedName = %q, want it to contain {namespace}", got)
	}
	if !strings.HasSuffix(got, "(partial)") {
		t.Fatalf("inherit-partial-parent ComputedName = %q, want it to end with (partial)", got)
	}
}

func TestNullLabelInheritExactParent(t *testing.T) {
	src := []byte(`
module "base" {
  source      = "cloudposse/label/null"
  namespace   = "eg"
  environment = "ue1"
}
module "this" {
  source  = "cloudposse/label/null"
  stage   = "prod"
  name    = "app"
  context = module.base.context
}
`)
	ext := Resolve([]Result{FileFromBytes("main.tf", src)}, []string{"main.tf"})
	if got := computedNameOf(ext, "tf"); got != "eg-ue1-prod-app" {
		t.Fatalf("inherit-exact-parent ComputedName = %q, want eg-ue1-prod-app", got)
	}
}

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
