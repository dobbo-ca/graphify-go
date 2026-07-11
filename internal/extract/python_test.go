package extract

import (
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
)

func TestExtractPython(t *testing.T) {
	root := "testdata/pyproj"
	files := []string{"util/math.py", "web/server.py"}

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
	for _, want := range []string{"math.py", "server.py", "add()", "Server", "Server.start()", "boot()"} {
		if !labels[want] {
			t.Errorf("missing node label %q", want)
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
	// Same-file call: Server.start -> boot.
	if !has("Server.start()", "calls", "boot()") {
		t.Error("expected Server.start --calls--> boot")
	}
	// Cross-file call: boot -> add (unique global definition).
	if !has("boot()", "calls", "add()") {
		t.Error("expected cross-file boot --calls--> add")
	}
	// External import edge for os.
	rels := map[string]int{}
	for _, e := range ext.Edges {
		rels[e.Relation]++
	}
	if rels["imports"] == 0 {
		t.Error("no external import edges (expected os)")
	}
}

// A `# NOTE:` comment becomes a rationale node with an edge to the file, and a
// function docstring becomes a rationale node with an edge to that function.
func TestExtractPythonRationale(t *testing.T) {
	src := []byte("# NOTE: keep boot fast\n" +
		"def boot():\n" +
		"    \"\"\"Boot the service and return its status code for callers.\"\"\"\n" +
		"    return 1\n")
	res := FileFromBytes("svc/app.py", src)

	fileID := idutil.MakeID("svc/app.py")
	funcID := idutil.MakeID("svc.app", "boot")

	label := map[string]string{}
	ftype := map[string]string{}
	for _, n := range res.Nodes {
		label[n.ID] = n.Label
		ftype[n.ID] = n.FileType
	}
	hasRationaleFor := func(labelPrefix, target string) bool {
		for _, e := range res.Edges {
			if e.Relation != "rationale_for" || e.Target != target {
				continue
			}
			if ftype[e.Source] == "rationale" && strings.HasPrefix(label[e.Source], labelPrefix) {
				return true
			}
		}
		return false
	}
	// NOTE comment -> rationale node, edge to the file.
	if !hasRationaleFor("# NOTE: keep boot fast", fileID) {
		t.Errorf("expected rationale_for edge from NOTE comment to file %s", fileID)
	}
	// Function docstring -> rationale node, edge to the function.
	if !hasRationaleFor("Boot the service", funcID) {
		t.Errorf("expected rationale_for edge from docstring to function %s", funcID)
	}
}

// A leading comment as a body child (which tree-sitter includes but upstream's
// AST omits) must not hide the docstring that follows it. A module-level comment
// is a direct sibling of the docstring statement, so module.Child(0) is the
// comment and module.Child(1) is the docstring: this exercises the comment-skip
// in pyDocstring, unlike a function body where the comment attaches to the
// function_definition node rather than the block.
func TestExtractPythonDocstringAfterComment(t *testing.T) {
	src := []byte("# a leading comment\n" +
		"\"\"\"Module docstring longer than twenty characters explaining architecture.\"\"\"\n")
	res := FileFromBytes("svc/app.py", src)

	fileID := idutil.MakeID("svc/app.py")

	label := map[string]string{}
	ftype := map[string]string{}
	for _, n := range res.Nodes {
		label[n.ID] = n.Label
		ftype[n.ID] = n.FileType
	}
	found := false
	for _, e := range res.Edges {
		if e.Relation != "rationale_for" || e.Target != fileID {
			continue
		}
		if ftype[e.Source] == "rationale" && strings.HasPrefix(label[e.Source], "Module docstring") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected docstring after leading comment to yield rationale_for edge to %s", fileID)
	}
}
