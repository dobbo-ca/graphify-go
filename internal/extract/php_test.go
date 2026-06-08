package extract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractPHP(t *testing.T) {
	root := "testdata/phpproj"
	files := []string{"util/math.php", "web/server.php"}

	// Call extractPHP directly: the central File() dispatch is wired elsewhere.
	var results []Result
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", f, err)
		}
		results = append(results, extractPHP(filepath.ToSlash(f), src))
	}
	ext := Resolve(results, files)

	labels := map[string]bool{}
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	for _, want := range []string{"math.php", "server.php", "add()", "Server", "Server.start()", "boot()"} {
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
	// Method scoped under its type, with a contains edge.
	if !has("Server", "contains", "Server.start()") {
		t.Error("expected Server --contains--> Server.start")
	}
	// Same-file call: Server.start -> boot.
	if !has("Server.start()", "calls", "boot()") {
		t.Error("expected Server.start --calls--> boot")
	}
	// Cross-file call: boot -> add (unique global definition).
	if !has("boot()", "calls", "add()") {
		t.Error("expected cross-file boot --calls--> add")
	}
	// External import edge for the `use` statement (Psr\Log\LoggerInterface).
	if rels := func() int {
		n := 0
		for _, e := range ext.Edges {
			if e.Relation == "imports" {
				n++
			}
		}
		return n
	}(); rels == 0 {
		t.Error("no external import edges (expected Psr\\Log\\LoggerInterface)")
	}
}
