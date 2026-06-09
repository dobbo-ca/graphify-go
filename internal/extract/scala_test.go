package extract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractScala(t *testing.T) {
	root := "testdata/scalaproj"
	files := []string{"util/math.scala", "web/server.scala"}

	var results []Result
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", f, err)
		}
		results = append(results, extractScala(filepath.ToSlash(f), src))
	}
	ext := Resolve(results, files)

	labels := map[string]bool{}
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	for _, want := range []string{
		"math.scala", "server.scala",
		"add()", "boot()",
		"MathUtils", "MathUtils.double()",
		"Server", "Server.start()",
		"scala.collection.mutable.Map", // external import dependency node
	} {
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

	// Type method scoped under its type, with a contains edge.
	if !has("Server", "contains", "Server.start()") {
		t.Error("expected Server --contains--> Server.start()")
	}
	if !has("MathUtils", "contains", "MathUtils.double()") {
		t.Error("expected MathUtils --contains--> MathUtils.double()")
	}
	// Same-file call: Server.start -> boot.
	if !has("Server.start()", "calls", "boot()") {
		t.Error("expected Server.start --calls--> boot")
	}
	// Cross-file call: boot -> add (unique global definition).
	if !has("boot()", "calls", "add()") {
		t.Error("expected cross-file boot --calls--> add")
	}
	// Import recorded as an external dependency edge from the file node.
	if !has("server.scala", "imports", "scala.collection.mutable.Map") {
		t.Error("expected server.scala --imports--> scala.collection.mutable.Map")
	}
}
