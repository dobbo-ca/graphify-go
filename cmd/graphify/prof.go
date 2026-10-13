package main

import (
	"os"
	"runtime"
	"runtime/pprof"
)

// startProf profiles one build: CPU to $GFY_CPUPROFILE, allocations to
// $GFY_MEMPROFILE. Measurement only; both unset is a no-op.
func startProf() func() {
	cpu, mem := os.Getenv("GFY_CPUPROFILE"), os.Getenv("GFY_MEMPROFILE")
	if cpu == "" && mem == "" {
		return func() {}
	}
	var f *os.File
	if cpu != "" {
		var err error
		if f, err = os.Create(cpu); err != nil {
			panic(err)
		}
		_ = pprof.StartCPUProfile(f)
	}
	if mem != "" {
		runtime.MemProfileRate = 64 << 10
	}
	return func() {
		if f != nil {
			pprof.StopCPUProfile()
			f.Close()
		}
		if mem != "" {
			if m, err := os.Create(mem); err == nil {
				_ = pprof.Lookup("allocs").WriteTo(m, 0)
				m.Close()
			}
		}
	}
}
