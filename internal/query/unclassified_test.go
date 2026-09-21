package query

import "testing"

// UnclassifiedSummary names the biggest offending extensions first and caps the
// list, so the line stays short on a corpus with a long tail of file types.
func TestUnclassifiedSummaryRanksAndCaps(t *testing.T) {
	g := &Graph{Attrs: GraphAttrs{
		UnclassifiedFiles: 61,
		UnclassifiedExts: map[string]int{
			".swift": 40, ".kt": 12, ".png": 5, ".ico": 3, ".md": 1,
		},
	}}
	want := "Unclassified: 61 file(s) no extractor handles (.swift 40, .kt 12, .png 5)"
	if got := g.UnclassifiedSummary(); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// A graph built before coverage was recorded (or over a fully-classified
// corpus) produces no line, so callers can skip it unconditionally.
func TestUnclassifiedSummaryEmptyWhenNoneSkipped(t *testing.T) {
	if got := (&Graph{}).UnclassifiedSummary(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
