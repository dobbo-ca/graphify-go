package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/dobbo-ca/graphify-go/internal/cache"
	"github.com/dobbo-ca/graphify-go/internal/detect"
	"github.com/dobbo-ca/graphify-go/internal/extract"
)

// binID tells builds apart when the version string is the same.
var binID = func() string {
	exe, err := os.Executable()
	if err != nil {
		return "noexe"
	}
	fi, err := os.Stat(exe)
	if err != nil {
		return "noexe"
	}
	return fmt.Sprintf("%d/%d", fi.Size(), fi.ModTime().UnixNano())
}()

// outFlags names the run options and manifest pass result that shape the
// outputs. The manifest result is the last of results.
func outFlags(noManifests, cargo, noCluster bool, results []extract.Result) string {
	if noManifests {
		return fmt.Sprintf("nomanifests cargo=%t nocluster=%t", cargo, noCluster)
	}
	b, _ := json.Marshal(results[len(results)-1])
	return fmt.Sprintf("cargo=%t nocluster=%t %x", cargo, noCluster, sha256.Sum256(b))
}

// outFP hashes every input of graph.json and GRAPH_REPORT.md other than file
// contents, plus the size and mtime of what a build wrote.
func outFP(root string, walk detect.WalkReport, commit, flags string) string {
	h := sha256.New()
	abs, _ := filepath.Abs(root)
	out := outDirFor(root)
	absOut, _ := filepath.Abs(out)
	fmt.Fprintf(h, "%q %q %q %q %q %q %q %q\n", cacheStamp(), binID, root, abs, out, absOut, commit, flags)
	for _, f := range walk.Files {
		fmt.Fprintf(h, "%q\n", f)
	}
	exts := make([]string, 0, len(walk.SkippedExts))
	for e := range walk.SkippedExts {
		exts = append(exts, e)
	}
	sort.Strings(exts)
	for _, e := range exts {
		fmt.Fprintf(h, "%q=%d\n", e, walk.SkippedExts[e])
	}
	for _, n := range []string{"graph.json", "GRAPH_REPORT.md", cache.FileName} {
		if fi, err := os.Stat(filepath.Join(out, n)); err == nil {
			fmt.Fprintf(h, "%d %d\n", fi.Size(), fi.ModTime().UnixNano())
		} else {
			fmt.Fprintln(h, "missing")
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// sigOf identifies a file version, to catch a writer that raced ours.
func sigOf(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d/%d", fi.Size(), fi.ModTime().UnixNano())
}

// startStatCheck stats the listed files during the walk. A stale index is
// dropped early so cache.Load's GC goal stays as on main.
func startStatCheck(root string, on bool) func() (cache.StatIndex, *cache.Output) {
	var prevStat cache.StatIndex
	var out *cache.Output
	done := make(chan struct{})
	go func() {
		defer close(done)
		if !on {
			return
		}
		s, o := cache.LoadStatOut(filepath.Join(outDirFor(root), cache.StatFileName), cacheStamp())
		if o != nil && statsFresh(root, s) {
			prevStat, out = s, o
		}
	}()
	return func() (cache.StatIndex, *cache.Output) {
		<-done
		return prevStat, out
	}
}

// statsFresh reports whether no file prevStat lists changed since it was
// recorded. No file is read.
func statsFresh(root string, prevStat cache.StatIndex) bool {
	keys := make([]string, 0, len(prevStat))
	for k := range prevStat {
		keys = append(keys, k)
	}
	var next atomic.Int64
	var stale atomic.Bool
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stale.Load() {
				i := int(next.Add(1)) - 1
				if i >= len(keys) {
					return
				}
				if !cache.StatFresh(filepath.Join(root, filepath.FromSlash(keys[i])), prevStat[keys[i]]) {
					stale.Store(true)
				}
			}
		}()
	}
	wg.Wait()
	return !stale.Load()
}

// sameFiles reports whether the walk found exactly the files prevStat lists.
func sameFiles(files []string, prevStat cache.StatIndex) bool {
	if len(files) != len(prevStat) {
		return false
	}
	for _, f := range files {
		if _, ok := prevStat[filepath.ToSlash(f)]; !ok {
			return false
		}
	}
	return true
}

// outputsCurrent reports whether an update of a tree statsFresh passed would
// rewrite the same bytes, so it can skip loading the cache.
func outputsCurrent(root string, rep detect.WalkReport, out *cache.Output, flags string) bool {
	// update would rewrite a lost or edited marker.
	abs, _ := filepath.Abs(root)
	if b, err := os.ReadFile(filepath.Join(outDirFor(root), rootFileName)); err != nil || string(b) != abs+"\n" {
		return false
	}
	return outFP(root, rep, gitHead(root), flags) == out.FP
}
