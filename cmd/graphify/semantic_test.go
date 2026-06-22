package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBuildArgsSemantic(t *testing.T) {
	cases := []struct {
		args         []string
		wantRoot     string
		wantSemantic bool
		wantBackend  string
		wantErr      bool
	}{
		{nil, ".", false, "", false},
		{[]string{"--semantic", "--backend", "bedrock"}, ".", true, "bedrock", false},
		{[]string{"path", "--semantic", "--backend=bedrock"}, "path", true, "bedrock", false},
		{[]string{"--semantic"}, ".", true, "", true},                    // --semantic with no --backend errors
		{[]string{"--backend", "bedrock"}, ".", false, "bedrock", false}, // backend without semantic is inert (semantic off)
	}
	for _, c := range cases {
		opts, err := parseBuildOpts(c.args)
		if (err != nil) != c.wantErr {
			t.Errorf("parseBuildOpts(%v) err=%v, wantErr=%v", c.args, err, c.wantErr)
			continue
		}
		if c.wantErr {
			continue
		}
		if opts.root != c.wantRoot || opts.semantic != c.wantSemantic || opts.backend != c.wantBackend {
			t.Errorf("parseBuildOpts(%v) = %+v, want root=%q semantic=%v backend=%q",
				c.args, opts, c.wantRoot, c.wantSemantic, c.wantBackend)
		}
	}
}

// TestBuildOffIsDeterministic proves that a build WITHOUT --semantic produces
// byte-identical graph.json across runs (the deterministic-core invariant the
// semantic stage must never disturb). The semantic stage is wired into the same
// writeOutputs path, so this guards that the wiring adds nothing when off.
func TestBuildOffIsDeterministic(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.md", "# A\n\nSee [B](b.md).\n")
	write("b.md", "# B\n\nIsolated note about deployment bake times.\n")

	graphPath := filepath.Join(root, "graphify-out", "graph.json")

	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build 1: %v", err)
	}
	first := readGraph(t, graphPath)

	if err := cmdBuild([]string{root}); err != nil {
		t.Fatalf("build 2: %v", err)
	}
	second := readGraph(t, graphPath)

	if first != second {
		t.Errorf("build without --semantic is not deterministic across runs")
	}
}
