//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

// isBrokenPipe must recognise a raw EPIPE and an EPIPE wrapped in an os.PathError
// (errors.Is unwraps to the syscall errno), and must reject unrelated errors so
// only genuine broken-pipe writes trigger the exit-0 shortcut (#1807).
func TestIsBrokenPipe(t *testing.T) {
	if !isBrokenPipe(syscall.EPIPE) {
		t.Error("isBrokenPipe(syscall.EPIPE) = false, want true")
	}
	wrapped := &os.PathError{Op: "write", Path: "x", Err: syscall.EPIPE}
	if !isBrokenPipe(wrapped) {
		t.Error("isBrokenPipe(wrapped PathError EPIPE) = false, want true")
	}
	if isBrokenPipe(errors.New("boom")) {
		t.Error("isBrokenPipe(errors.New(\"boom\")) = true, want false")
	}
}

// brokenPipeGraph is a minimal NetworkX node-link graph whose ~8 nodes all match
// the `query Node` pattern, guaranteeing the CLI produces stdout output to write.
const brokenPipeGraph = `{"directed":true,"multigraph":false,"graph":{},"nodes":[` +
	`{"id":"n0","label":"Node0()","file_type":"code","source_file":"f0.go","source_location":"L1","norm_label":"node0()"},` +
	`{"id":"n1","label":"Node1()","file_type":"code","source_file":"f1.go","source_location":"L1","norm_label":"node1()"},` +
	`{"id":"n2","label":"Node2()","file_type":"code","source_file":"f2.go","source_location":"L1","norm_label":"node2()"},` +
	`{"id":"n3","label":"Node3()","file_type":"code","source_file":"f3.go","source_location":"L1","norm_label":"node3()"},` +
	`{"id":"n4","label":"Node4()","file_type":"code","source_file":"f4.go","source_location":"L1","norm_label":"node4()"},` +
	`{"id":"n5","label":"Node5()","file_type":"code","source_file":"f5.go","source_location":"L1","norm_label":"node5()"},` +
	`{"id":"n6","label":"Node6()","file_type":"code","source_file":"f6.go","source_location":"L1","norm_label":"node6()"},` +
	`{"id":"n7","label":"Node7()","file_type":"code","source_file":"f7.go","source_location":"L1","norm_label":"node7()"}` +
	`],"links":[]}`

// A downstream reader that closes the pipe early (e.g. `graphify query Node | head`)
// must make graphify exit 0, not die with SIGPIPE (exit 141), which CI wrappers
// misread as a command failure (#1807). This drives the real binary end-to-end.
func TestQueryExitsZeroOnBrokenPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGPIPE semantics are unix-only")
	}

	bin := filepath.Join(t.TempDir(), "gfy")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "graphify-out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "graphify-out", "graph.json"), []byte(brokenPipeGraph), 0o644); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "query", "Node")
	cmd.Dir = work
	cmd.Stdout = w
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Drop the parent's write-end copy so only the child holds it, then close the
	// read end BEFORE the child gets around to writing. With no reader left, the
	// child's stdout writes hit a broken pipe deterministically.
	w.Close()
	r.Close()
	_ = cmd.Wait()

	code := cmd.ProcessState.ExitCode()
	if code == 141 {
		t.Fatalf("query died with SIGPIPE (exit 141) on early pipe close; broken-pipe handler did not fire")
	}
	if code != 0 {
		t.Fatalf("query exit code = %d, want 0 on broken pipe", code)
	}
}
