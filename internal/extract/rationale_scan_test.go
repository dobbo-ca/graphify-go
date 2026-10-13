package extract

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	tspy "github.com/dobbo-ca/gositter/grammars/python"
)

// The line scans must record what the whole-file split they replaced did.
func TestRationaleScanMatchesSplit(t *testing.T) {
	jsRef := func(b *builder, src []byte) {
		seen := map[string]bool{}
		for i, lineText := range strings.Split(string(src), "\n") {
			stripped := strings.TrimSpace(lineText)
			for _, p := range jsRationalePrefixes {
				if strings.HasPrefix(stripped, p) {
					b.addRationale(strings.TrimLeft(stripped, "/* "), i+1, b.fileID)
					break
				}
			}
			if jsCommentLineRe.MatchString(lineText) {
				for _, m := range jsDocRefRe.FindAllStringSubmatch(stripped, -1) {
					b.addDocRef(m[1], i+1, seen)
				}
			}
		}
	}
	pyRef := func(b *builder, src []byte) {
		for i, lineText := range strings.Split(string(src), "\n") {
			stripped := strings.TrimSpace(lineText)
			for _, p := range pyRationalePrefixes {
				if strings.HasPrefix(stripped, p) {
					b.addRationale(stripped, i+1, b.fileID)
					break
				}
			}
		}
	}
	toks := []string{"// NOTE: a", "* WHY: b", "# TODO: c", "// ", "/* ", "* ", "#", "x", " ", "\t", "\v", "\u00a0", "\r", "\n", "\n",
		"ADR-12", "adr 7", "rfc793", "RFC-1", "ADRx", "RF C", "\u212a", "\xff", "see ", "= 1;"}
	// pyRationale reads docstrings off the tree first; an empty one has none.
	root, done := parseRoot(nil, tspy.Language())
	defer done()
	rng, found := rand.New(rand.NewSource(1)), 0
	for i := 0; i < 50000; i++ {
		var sb strings.Builder
		for n := rng.Intn(12); n > 0; n-- {
			sb.WriteString(toks[rng.Intn(len(toks))])
		}
		src := []byte(sb.String())
		got, want := newBuilder("a.ts"), newBuilder("a.ts")
		got.jsRationale(src)
		jsRef(want, src)
		if !reflect.DeepEqual(got.res, want.res) {
			t.Fatalf("js %q:\n got %+v\nwant %+v", src, got.res, want.res)
		}
		found += len(got.res.Nodes)
		got, want = newBuilder("a.py"), newBuilder("a.py")
		got.pyRationale(root, src)
		pyRef(want, src)
		if !reflect.DeepEqual(got.res, want.res) {
			t.Fatalf("py %q:\n got %+v\nwant %+v", src, got.res, want.res)
		}
		found += len(got.res.Nodes)
	}
	if found < 1000 {
		t.Fatalf("only %d nodes recorded: the generator no longer reaches the scans", found)
	}
}
