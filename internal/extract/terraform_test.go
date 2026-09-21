package extract

import "testing"

func TestExtractTerraform(t *testing.T) {
	root := "testdata/tf"
	files := []string{"main.tf", "outputs.tf"}

	var results []Result
	for _, f := range files {
		r, err := File(root, f)
		if err != nil {
			t.Fatalf("File(%s): %v", f, err)
		}
		results = append(results, r)
	}
	ext := Resolve(results, files)

	labels := map[string]bool{}
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	for _, want := range []string{"aws_instance.web", "aws_vpc.main", "data.aws_ami.ubuntu", "var.region", "output.instance_id"} {
		if !labels[want] {
			t.Errorf("missing node %q", want)
		}
	}

	has := func(srcLabel, rel, tgtLabel string) bool {
		for _, e := range ext.Edges {
			if e.Relation == rel && id2label[e.Source] == srcLabel && id2label[e.Target] == tgtLabel {
				return true
			}
		}
		return false
	}
	if !has("aws_instance.web", "references", "var.region") {
		t.Error("expected aws_instance.web --references--> var.region")
	}
	if !has("aws_instance.web", "depends_on", "aws_vpc.main") {
		t.Error("expected aws_instance.web --depends_on--> aws_vpc.main")
	}
	// Cross-file resolution: outputs.tf references a resource defined in main.tf.
	if !has("output.instance_id", "references", "aws_instance.web") {
		t.Error("expected cross-file output.instance_id --references--> aws_instance.web")
	}
}

// Empty block bodies (e.g. `data "aws_region" "current" {}`) are pervasive in
// real Terraform. tree-sitter yields a nil body for them, which previously
// panicked when refsFrom walked it. The block node must still be extracted.
func TestExtractTerraformEmptyBody(t *testing.T) {
	src := []byte(`data "aws_region" "current" {}
data "aws_caller_identity" "current" {}
resource "aws_x" "y" {}
module "m" {}
output "o" {}
`)
	res := FileFromBytes("empty.tf", src)
	labels := map[string]bool{}
	for _, n := range res.Nodes {
		labels[n.Label] = true
	}
	for _, want := range []string{"data.aws_region.current", "data.aws_caller_identity.current", "aws_x.y", "module.m", "output.o"} {
		if !labels[want] {
			t.Errorf("missing node %q from empty-body blocks", want)
		}
	}
}

// Terraform addresses are directory-scoped. Two directories that merely share a
// base name (e.g. workspaces/scalr-agents and modules/scalr-agents) must NOT
// collapse their addresses into one node — that fabricates cross-directory
// references and drops real nodes. Scope is the full directory path, so the same
// resource address in different directories yields distinct nodes.
func TestExtractTerraformScopeByFullPath(t *testing.T) {
	src := []byte(`resource "aws_x" "r" {}`)
	files := []string{"a/dup/main.tf", "b/dup/main.tf"}
	results := []Result{
		FileFromBytes(files[0], src),
		FileFromBytes(files[1], src),
	}
	ext := Resolve(results, files)

	var ids []string
	for _, n := range ext.Nodes {
		if n.Label == "aws_x.r" {
			ids = append(ids, n.ID)
		}
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 distinct aws_x.r nodes (one per directory), got %d: %v", len(ids), ids)
	}
	if ids[0] == ids[1] {
		t.Errorf("same-basename directories collided into one node id %q", ids[0])
	}
}

// A module block's source must link the module call to what it instantiates: a
// local/relative source resolves to the target directory node (and that node
// gains contains edges to the directory's files); a registry/private-registry
// source becomes an external concept node. Without this every module node is an
// island with no edge to its implementation.
func TestExtractTerraformModuleSource(t *testing.T) {
	files := []string{"workspaces/ws/main.tf", "modules/proscia/main.tf"}
	results := []Result{
		FileFromBytes(files[0], []byte(`
module "p" {
  source = "../../modules/proscia"
}
module "vpc" {
  source  = "cloudposse/vpc/aws"
  version = "2.2.0"
}
`)),
		FileFromBytes(files[1], []byte(`resource "aws_x" "y" {}`)),
	}
	ext := Resolve(results, files)

	id2label := map[string]string{}
	type2id := map[string]string{}
	for _, n := range ext.Nodes {
		id2label[n.ID] = n.Label
		type2id[n.Label] = n.ID
	}
	has := func(srcLabel, rel, tgtLabel string) bool {
		for _, e := range ext.Edges {
			if e.Relation == rel && id2label[e.Source] == srcLabel && id2label[e.Target] == tgtLabel {
				return true
			}
		}
		return false
	}

	// Local source resolves to the target directory node.
	if _, ok := type2id["modules/proscia"]; !ok {
		t.Error("missing directory node for resolved local module source modules/proscia")
	}
	if !has("module.p", "references", "modules/proscia") {
		t.Error("expected module.p --references--> modules/proscia (dir node)")
	}
	// The dir node is navigable into the module's files.
	if !has("modules/proscia", "contains", "main.tf") {
		t.Error("expected modules/proscia --contains--> main.tf")
	}
	// Registry source becomes an external concept node.
	if !has("module.vpc", "references", "cloudposse/vpc/aws") {
		t.Error("expected module.vpc --references--> cloudposse/vpc/aws (external node)")
	}
}

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
	for _, e := range res.Edges {
		if e.Relation == "inherits_context" &&
			id2label[e.Source] == "module.label [null-label]" &&
			id2label[e.Target] == "module.this [null-label]" {
			found = true
		}
	}
	if !found {
		t.Error("expected module.label --inherits_context--> module.this")
	}
}

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

// Block attributes are persisted on the node so "which resources run t3.large"
// is a graph query instead of a file read. Sensitive keys keep the key and
// redact the value: graph.json is committed in CI, so a hardcoded secret must
// never reach it.
func TestExtractTerraformAttributes(t *testing.T) {
	src := []byte(`resource "aws_instance" "web" {
  instance_type = "t3.large"
  count         = 2
  monitoring    = true
  ami           = var.ami_id
  vpc_security_group_ids = ["sg-1", "sg-2"]
  tags = {
    Name = "web"
  }
  db_password = "hunter2"
}

locals {
  x = "y"
}
`)
	res := FileFromBytes("main.tf", src)
	var got map[string]string
	for _, n := range res.Nodes {
		if n.Label == "aws_instance.web" {
			got = n.Attributes
		}
		if n.Label == "local.x" && n.Attributes != nil {
			t.Errorf("locals entries must not carry attributes, got %v", n.Attributes)
		}
	}
	if got == nil {
		t.Fatal("aws_instance.web has no attributes")
	}
	want := map[string]string{
		"instance_type":          "t3.large",
		"count":                  "2",
		"monitoring":             "true",
		"ami":                    "var.ami_id",
		"vpc_security_group_ids": "sg-1, sg-2",
		"db_password":            redactedValue,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("attribute %q = %q, want %q", k, got[k], v)
		}
	}
	if v, ok := got["tags"]; ok {
		t.Errorf("nested object attribute should be dropped, got tags=%q", v)
	}
}

// Redaction is keyed on the attribute NAME, so every common secret spelling is
// caught regardless of where it appears.
func TestExtractTerraformAttributeRedaction(t *testing.T) {
	src := []byte(`resource "x" "y" {
  db_password           = "p"
  aws_secret_access_key = "s"
  client_secret         = "c"
  api_key               = "k"
  connection_string     = "conn"
  ami                   = "ami-123"
}
`)
	res := FileFromBytes("main.tf", src)
	var got map[string]string
	for _, n := range res.Nodes {
		if n.Label == "x.y" {
			got = n.Attributes
		}
	}
	for _, k := range []string{"db_password", "aws_secret_access_key", "client_secret", "api_key", "connection_string"} {
		if got[k] != redactedValue {
			t.Errorf("attribute %q = %q, want %q", k, got[k], redactedValue)
		}
	}
	if got["ami"] != "ami-123" {
		t.Errorf("non-sensitive attribute was lost: ami=%q", got["ami"])
	}

	// The secret's name can live in the block LABEL, and Terraform's own
	// `sensitive = true` is the strongest author-declared signal; both must
	// redact the value-carrying attribute (`default` / `value`).
	src = []byte(`variable "db_password" {
  type      = string
  sensitive = true
  default   = "hunter2"
}

output "conn" {
  sensitive = true
  value     = "postgres://admin:hunter2@db"
}

output "api_token" {
  value = "tok-abc"
}

resource "aws_instance" "web" {
  instance_type = "t3.large"
  user_data     = <<-EOT
    #!/bin/bash
    export AWS_SECRET=AKIAhunter2SECRET
  EOT
}
`)
	res = FileFromBytes("main.tf", src)
	byLabel := map[string]map[string]string{}
	for _, n := range res.Nodes {
		byLabel[n.Label] = n.Attributes
	}
	for _, c := range []struct{ label, key string }{
		{"var.db_password", "default"},
		{"output.conn", "value"},
		{"output.api_token", "value"},
	} {
		if v := byLabel[c.label][c.key]; v != redactedValue {
			t.Errorf("%s.%s = %q, want %q", c.label, c.key, v, redactedValue)
		}
	}
	// A sensitive block redacts every value, not just the obvious one.
	if v := byLabel["var.db_password"]["type"]; v != redactedValue {
		t.Errorf("var.db_password type = %q, want %q", v, redactedValue)
	}
	if v, ok := byLabel["aws_instance.web"]["user_data"]; ok {
		t.Errorf("bootstrap script must be dropped, got user_data=%q", v)
	}
	if v := byLabel["aws_instance.web"]["instance_type"]; v != "t3.large" {
		t.Errorf("instance_type = %q, want t3.large", v)
	}
}

// A comment between `=` and the value is a named child of the attribute, so the
// value must be found by skipping comments — otherwise the comment is stored as
// the value and `sensitive = true` stops redacting.
func TestExtractTerraformAttributeCommentBeforeValue(t *testing.T) {
	src := []byte(`variable "thing" {
  sensitive = // yes
    true
  default = "hunter2"
}

resource "aws_instance" "web" {
  instance_type = /* c */ "t3.large"
}
`)
	res := FileFromBytes("main.tf", src)
	byLabel := map[string]map[string]string{}
	for _, n := range res.Nodes {
		byLabel[n.Label] = n.Attributes
	}
	if v := byLabel["var.thing"]["default"]; v != redactedValue {
		t.Errorf("var.thing default = %q, want %q", v, redactedValue)
	}
	if v := byLabel["aws_instance.web"]["instance_type"]; v != "t3.large" {
		t.Errorf("instance_type = %q, want t3.large", v)
	}
}
