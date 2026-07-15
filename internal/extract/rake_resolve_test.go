package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/graph"
)

// A call made in a .rake file must cross-resolve to a definition in a .rb file:
// .rake is plain Ruby, so both share the ruby language family and the call edge
// must survive resolution rather than being dropped as a cross-family phantom
// (#1784). Mirrors the ruby cross-file boot()->add() shape in ruby_test.go.
func TestRakeCrossResolvesToRb(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lib/helpers.rb"), []byte("def add(a, b)\n  a + b\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks/build.rake"), []byte("def run\n  add(1, 2)\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ext := resolveFiles(t, root, "lib/helpers.rb", "tasks/build.rake")
	g := graph.Build(ext)

	var runID, addID string
	for _, n := range ext.Nodes {
		switch n.Label {
		case "run()":
			runID = n.ID
		case "add()":
			addID = n.ID
		}
	}
	if runID == "" || addID == "" {
		t.Fatalf("missing node ids: run()=%q add()=%q", runID, addID)
	}
	if !g.HasEdge(runID, addID) {
		t.Errorf(".rake call run() did not cross-resolve to .rb def add(): ruby-family grouping dropped the edge")
	}
}
