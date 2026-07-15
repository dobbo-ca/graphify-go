package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/graph"
	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

func resolveFiles(t *testing.T, root string, files ...string) model.Extraction {
	t.Helper()
	var results []Result
	for _, f := range files {
		r, err := File(root, f)
		if err != nil {
			t.Fatalf("File(%s): %v", f, err)
		}
		results = append(results, r)
	}
	return Resolve(results, files)
}

func hasScriptCall(edges []model.Edge, srcID, tgtID string) bool {
	for _, e := range edges {
		if e.Relation == "calls" && e.Source == srcID && e.Target == tgtID {
			return true
		}
	}
	return false
}

func writeScript(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// `bash x.sh` and a bare `./x.sh` both emit a script_invocation calls edge from
// the invoking script's file node to the invoked script's file node (#1756).
func TestBashScriptInvocation(t *testing.T) {
	for _, cmd := range []string{"./helpers.sh", "bash ./helpers.sh"} {
		t.Run(cmd, func(t *testing.T) {
			root := t.TempDir()
			writeScript(t, root, "helpers.sh", "#!/bin/bash\necho helper\n")
			writeScript(t, root, "deploy.sh", "#!/bin/bash\n"+cmd+"\n")

			ext := resolveFiles(t, root, "deploy.sh", "helpers.sh")
			from, to := idutil.MakeID("deploy.sh"), idutil.MakeID("helpers.sh")
			if !hasScriptCall(ext.Edges, from, to) {
				t.Fatalf("missing script_invocation edge deploy.sh->helpers.sh; edges=%v", ext.Edges)
			}
			// Survives graph build (shell->shell, not a cross-language phantom).
			if g := graph.Build(ext); !g.HasEdge(from, to) {
				t.Errorf("script_invocation edge did not survive graph build")
			}
		})
	}
}

// The edge is attributed to the enclosing function when the invocation is inside
// one, not to the file node.
func TestBashScriptInvocationFromFunction(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "helpers.sh", "#!/bin/bash\necho helper\n")
	writeScript(t, root, "deploy.sh", "#!/bin/bash\ndeploy() { bash ./helpers.sh; }\n")

	ext := resolveFiles(t, root, "deploy.sh", "helpers.sh")
	fn := idutil.MakeID("deploy", "deploy") // fileStem "deploy" + func "deploy"
	to := idutil.MakeID("helpers.sh")
	if !hasScriptCall(ext.Edges, fn, to) {
		t.Errorf("expected edge from deploy() function node; edges=%v", ext.Edges)
	}
	if hasScriptCall(ext.Edges, idutil.MakeID("deploy.sh"), to) {
		t.Errorf("edge should attribute to the function, not the file node")
	}
}

// A runner name that shadows a function defined in the file is that function, not
// a script invocation — no edge even though the target exists.
func TestBashScriptInvocationSkipsShadowed(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "helpers.sh", "#!/bin/bash\necho helper\n")
	writeScript(t, root, "deploy.sh", "#!/bin/bash\nbash() { echo custom; }\nbash ./helpers.sh\n")

	ext := resolveFiles(t, root, "deploy.sh", "helpers.sh")
	if hasScriptCall(ext.Edges, idutil.MakeID("deploy.sh"), idutil.MakeID("helpers.sh")) {
		t.Errorf("shadowed `bash` must not emit a script_invocation edge; edges=%v", ext.Edges)
	}
}

// A dynamic target (`bash "./$X.sh"`) is not a literal and emits no edge.
func TestBashScriptInvocationSkipsDynamic(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "deploy.sh", "#!/bin/bash\nbash \"./$SCRIPT.sh\"\n")

	r, err := File(root, "deploy.sh")
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	for _, e := range r.Edges {
		if e.Relation == "calls" {
			t.Errorf("dynamic invocation must emit no calls edge, got %+v", e)
		}
	}
}

// A target that is not in the corpus leaves a dangling edge that graph build
// prunes, so the final graph has no invocation edge to a missing script.
func TestBashScriptInvocationPrunesMissing(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "deploy.sh", "#!/bin/bash\n./missing.sh\n")

	ext := resolveFiles(t, root, "deploy.sh") // missing.sh not collected
	g := graph.Build(ext)
	if g.HasEdge(idutil.MakeID("deploy.sh"), idutil.MakeID("missing.sh")) {
		t.Errorf("edge to a missing script must be pruned from the built graph")
	}
}
