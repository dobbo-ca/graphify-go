package extract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractLua(t *testing.T) {
	root := "testdata/luaproj"
	files := []string{"util/math.lua", "web/server.lua"}

	var results []Result
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		results = append(results, extractLua(filepath.ToSlash(f), src))
	}
	ext := Resolve(results, files)

	labels := map[string]bool{}
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	for _, want := range []string{
		"math.lua", "server.lua",
		"add()", "boot()",
		"M", "M.double()",
		"Server", "Server.start()",
		"socket", // external dependency from require
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

	// Method scoped under its table, with a contains edge.
	if !has("M", "contains", "M.double()") {
		t.Error("expected M --contains--> M.double()")
	}
	if !has("Server", "contains", "Server.start()") {
		t.Error("expected Server --contains--> Server.start()")
	}
	// Same-file call: M.double -> add.
	if !has("M.double()", "calls", "add()") {
		t.Error("expected M.double --calls--> add")
	}
	// Same-file call: Server.start -> boot.
	if !has("Server.start()", "calls", "boot()") {
		t.Error("expected Server.start --calls--> boot")
	}
	// Cross-file call: boot (server.lua) -> add (unique global in math.lua).
	if !has("boot()", "calls", "add()") {
		t.Error("expected cross-file boot --calls--> add")
	}
	// require("socket") recorded as an external import.
	hasImport := false
	for _, e := range ext.Edges {
		if e.Relation == "imports" && id2label[e.Target] == "socket" {
			hasImport = true
		}
	}
	if !hasImport {
		t.Error("expected an imports edge to external dependency socket")
	}
}
