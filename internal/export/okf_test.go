package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// okfSampleJSON has two nodes in distinct source dirs, one edge between them, and
// a community + label with a YAML-significant character to exercise quoting.
const okfSampleJSON = `{"directed":true,"multigraph":false,"graph":{},
"nodes":[
 {"id":"src_a_go_add","label":"Add: x","file_type":"code","source_file":"src/a.go","source_location":"L3","community":0,"norm_label":"add: x"},
 {"id":"pkg_b_go_b","label":"B()","file_type":"code","source_file":"pkg/b.go","community":1,"norm_label":"b()"}
],
"links":[
 {"source":"src_a_go_add","target":"pkg_b_go_b","relation":"calls","confidence":"INFERRED","confidence_score":0.5}
],
"hyperedges":[]}`

func writeOKFSample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(jsonPath, []byte(okfSampleJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return jsonPath
}

// readBundle walks out and returns a map of bundle-relative slash paths to file
// contents, so tests can assert on the whole tree.
func readBundle(t *testing.T, out string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(out, p)
		data, _ := os.ReadFile(p)
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestOKFConceptPathAndFrontmatter(t *testing.T) {
	jsonPath := writeOKFSample(t)
	out := filepath.Join(t.TempDir(), "okf")
	if err := OKFFromJSON(jsonPath, out); err != nil {
		t.Fatal(err)
	}
	files := readBundle(t, out)

	// Concept ID = bundle path with .md stripped: <source-dir>/<id>.md. Each path
	// segment is sanitised via idutil for safety, so "a.go" becomes "a_go".
	const concept = "src/a_go/src_a_go_add.md"
	doc, ok := files[concept]
	if !ok {
		t.Fatalf("missing concept document %s; bundle: %v", concept, keys(files))
	}

	// Frontmatter is a leading --- block with the REQUIRED `type` field.
	if !strings.HasPrefix(doc, "---\n") {
		t.Fatalf("concept does not start with YAML frontmatter:\n%s", doc)
	}
	fm := doc[len("---\n"):]
	end := strings.Index(fm, "\n---\n")
	if end < 0 {
		t.Fatalf("frontmatter not terminated:\n%s", doc)
	}
	fm = fm[:end]
	for _, want := range []string{
		`type: "code"`,
		`title: "Add: x"`, // the colon must be quoted, not breaking YAML
		"description:",
		`resource: "src/a.go:L3"`,
		"community:0",
	} {
		if !strings.Contains(fm, want) {
			t.Errorf("frontmatter missing %q:\n%s", want, fm)
		}
	}
}

func TestOKFCrossLinkForm(t *testing.T) {
	jsonPath := writeOKFSample(t)
	out := filepath.Join(t.TempDir(), "okf")
	if err := OKFFromJSON(jsonPath, out); err != nil {
		t.Fatal(err)
	}
	files := readBundle(t, out)

	doc := files["src/a_go/src_a_go_add.md"]
	if !strings.Contains(doc, "# Relations") {
		t.Errorf("concept missing # Relations section:\n%s", doc)
	}
	// Outgoing `calls` edge -> absolute, bundle-relative link (leading /, .md).
	const link = "(/pkg/b_go/pkg_b_go_b.md)"
	if !strings.Contains(doc, link) {
		t.Errorf("relation link %q not found:\n%s", link, doc)
	}
	if !strings.Contains(doc, "## calls") {
		t.Errorf("relations not grouped by relation name:\n%s", doc)
	}
	// The reverse direction is recorded on the callee.
	callee := files["pkg/b_go/pkg_b_go_b.md"]
	if !strings.Contains(callee, "(/src/a_go/src_a_go_add.md)") {
		t.Errorf("callee missing incoming relation link:\n%s", callee)
	}
}

func TestOKFIndexGeneration(t *testing.T) {
	jsonPath := writeOKFSample(t)
	out := filepath.Join(t.TempDir(), "okf")
	if err := OKFFromJSON(jsonPath, out); err != nil {
		t.Fatal(err)
	}
	files := readBundle(t, out)

	for _, idx := range []string{"index.md", "src/index.md", "src/a_go/index.md", "pkg/index.md", "pkg/b_go/index.md"} {
		if _, ok := files[idx]; !ok {
			t.Errorf("missing %s; bundle: %v", idx, keys(files))
		}
	}
	// The root index lists its subdirectories.
	root := files["index.md"]
	if !strings.Contains(root, "(/src/index.md)") || !strings.Contains(root, "(/pkg/index.md)") {
		t.Errorf("root index missing subdirectory links:\n%s", root)
	}
	// A leaf index lists its concept with a description.
	leaf := files["src/a_go/index.md"]
	if !strings.Contains(leaf, "(/src/a_go/src_a_go_add.md)") {
		t.Errorf("leaf index missing concept link:\n%s", leaf)
	}
}

func TestOKFDeterministic(t *testing.T) {
	jsonPath := writeOKFSample(t)
	out1 := filepath.Join(t.TempDir(), "okf")
	out2 := filepath.Join(t.TempDir(), "okf")
	if err := OKFFromJSON(jsonPath, out1); err != nil {
		t.Fatal(err)
	}
	if err := OKFFromJSON(jsonPath, out2); err != nil {
		t.Fatal(err)
	}
	a, b := readBundle(t, out1), readBundle(t, out2)
	if len(a) != len(b) {
		t.Fatalf("bundle file counts differ: %d vs %d", len(a), len(b))
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || va != vb {
			t.Errorf("byte-stable mismatch for %s", k)
		}
	}
}

func TestOKFContainment(t *testing.T) {
	// A source path with .. components must not escape the bundle root; conceptDir
	// sanitises each segment to a safe idutil token, so traversal is impossible.
	if d := conceptDir("../../etc/passwd"); strings.Contains(d, "..") {
		t.Errorf("conceptDir leaked traversal segments: %q", d)
	}
	if _, err := safeJoin("/base", "../escape", "x.md"); err == nil {
		t.Error("safeJoin accepted an escaping path, want error")
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
