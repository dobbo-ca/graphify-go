package extract

import (
	"regexp"
	"testing"
)

// recvTailRE is the expression recvTail replaced; the scan must agree with it
// on every input, including invalid UTF-8.
var recvTailRE = regexp.MustCompile(`[\p{L}\p{N}_]+$`)

var recvTailSeeds = []string{
	"", "a", "a.b", "a.b.c_1", "f()", "a[0]", "self", "$this", "@self", "super()",
	"a.", ".", "_", "__x__", "a b", "x\n", "ns::Type", "ptr->field",
	"naïve", "obj.日本語", "x.٣٤", "a.Ⅻ", "é", "a.́", "a​b", "x.½",
	"\xff", "a\xff", "\xffa", "a.\xc3", "\xc3a", "\xe4\xb8", "\xe4\xb8é", "é\xe4\xb8",
	"\xf0\x9f\x98", "a\xf0\x9f\x98\x80b", "\xc0\x80a", "\xed\xa0\x80z", "a\xef\xbf\xbd", "\xef\xbf\xbdb",
}

func TestRecvTailMatchesRegexp(t *testing.T) {
	for _, s := range recvTailSeeds {
		if got, want := recvTail(s), recvTailRE.FindString(s); got != want {
			t.Errorf("recvTail(%q) = %q, regexp gives %q", s, got, want)
		}
	}
}

func FuzzRecvTail(f *testing.F) {
	for _, s := range recvTailSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got, want := recvTail(s), recvTailRE.FindString(s); got != want {
			t.Fatalf("recvTail(%q) = %q, regexp gives %q", s, got, want)
		}
	})
}
