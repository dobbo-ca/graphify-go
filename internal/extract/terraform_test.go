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
