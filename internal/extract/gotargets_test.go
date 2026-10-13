package extract

import (
	"fmt"
	"math/rand"
	"path"
	"reflect"
	"testing"
)

// The shared Go target lists must give what filtering the package per import
// gave: a file importing its own package, two checkouts of one module, and
// repeated files included.
func TestGoImportTargetsMatchPerImportFilter(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for round := 0; round < 300; round++ {
		goMods := map[string]string{}
		for _, dir := range []string{".", "a", "b", "a/vendor/x"}[:1+rng.Intn(4)] {
			goMods[dir] = []string{"m", "m", "n"}[rng.Intn(3)]
		}
		var files []string
		goPkgs := map[string][]string{}
		for i := 0; i < 1+rng.Intn(30); i++ {
			f := path.Join([]string{".", "a", "b", "a/vendor/x", "c"}[rng.Intn(5)], []string{"", "p", "q"}[rng.Intn(3)], fmt.Sprintf("f%d.go", rng.Intn(12)))
			files = append(files, f)
			ip := []string{"m/p", "m/q", "n/p", "m"}[rng.Intn(4)]
			goPkgs[ip] = append(goPkgs[ip], f)
			if rng.Intn(6) == 0 {
				goPkgs[ip] = append(goPkgs[ip], f, "")
			}
		}
		shared := map[[2]string][]string{}
		for i := 0; i < 60; i++ {
			im := Imp{File: files[rng.Intn(len(files))], Spec: []string{"m/p", "m/q", "n/p", "m", "fmt"}[rng.Intn(5)]}
			var want []string
			pkg := goPkgs[im.Spec]
			own := goModDir(goMods, im.File)
			if mine := keep(pkg, func(f string) bool { return goModDir(goMods, f) == own }); len(mine) > 0 {
				pkg = mine
			}
			for _, f := range pkg {
				if f != "" && f != im.File && !contains(want, f) {
					want = append(want, f)
				}
			}
			got := importTargets(im, nil, goPkgs, goMods, shared)
			if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
				t.Fatalf("round %d: %s imports %s: got %v, want %v", round, im.File, im.Spec, got, want)
			}
		}
	}
}
