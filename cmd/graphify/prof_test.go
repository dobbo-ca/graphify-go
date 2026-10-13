package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func keepMemProfileRate(t *testing.T) int {
	rate := runtime.MemProfileRate
	t.Cleanup(func() { runtime.MemProfileRate = rate })
	return rate
}

func TestProfWritesBothProfiles(t *testing.T) {
	keepMemProfileRate(t)
	dir := t.TempDir()
	cpu, mem := filepath.Join(dir, "cpu.pprof"), filepath.Join(dir, "mem.pprof")
	t.Setenv("GFY_CPUPROFILE", cpu)
	t.Setenv("GFY_MEMPROFILE", mem)

	stop := startProf()
	var keep [][]byte
	for i := 0; i < 2000; i++ {
		keep = append(keep, make([]byte, 4096))
	}
	_ = keep
	stop()

	for _, p := range []string{cpu, mem} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) < 2 || b[0] != 0x1f || b[1] != 0x8b {
			t.Errorf("%s: not gzip pprof (%d bytes)", filepath.Base(p), len(b))
		}
	}
}

func TestProfUnsetIsNoOp(t *testing.T) {
	rate := keepMemProfileRate(t)
	t.Setenv("GFY_CPUPROFILE", "")
	t.Setenv("GFY_MEMPROFILE", "")
	dir := t.TempDir()
	t.Chdir(dir)

	startProf()()

	if runtime.MemProfileRate != rate {
		t.Errorf("MemProfileRate = %d, want %d", runtime.MemProfileRate, rate)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("unset profile created %d files", len(ents))
	}
}

func TestProfBadPathDoesNotPanic(t *testing.T) {
	rate := keepMemProfileRate(t)
	dir := t.TempDir()
	t.Setenv("GFY_CPUPROFILE", filepath.Join(dir, "missing", "cpu.pprof"))
	t.Setenv("GFY_MEMPROFILE", filepath.Join(dir, "missing", "mem.pprof"))

	startProf()()

	if runtime.MemProfileRate != rate {
		t.Errorf("MemProfileRate = %d, want %d", runtime.MemProfileRate, rate)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("bad path created %d entries", len(ents))
	}
}
