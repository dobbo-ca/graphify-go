package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
)

// startProf profiles one build or update: CPU to $GFY_CPUPROFILE, allocations
// to $GFY_MEMPROFILE. Measurement only; both unset is a no-op.
func startProf() func() {
	cpu, mem := os.Getenv("GFY_CPUPROFILE"), os.Getenv("GFY_MEMPROFILE")
	if cpu == "" && mem == "" {
		return func() {}
	}
	f, m := profFile(cpu), profFile(mem)
	if f != nil {
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "[graphify] warning: no CPU profile: %v\n", err)
			f.Close()
			f = nil
		}
	}
	if m != nil {
		runtime.MemProfileRate = 64 << 10
	}
	return func() {
		if f != nil {
			pprof.StopCPUProfile()
			f.Close()
		}
		if m != nil {
			_ = pprof.Lookup("allocs").WriteTo(m, 0)
			m.Close()
		}
	}
}

// profFile warns and returns nil on a bad path.
func profFile(path string) *os.File {
	if path == "" {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[graphify] warning: no profile: %v\n", err)
		return nil
	}
	return f
}
