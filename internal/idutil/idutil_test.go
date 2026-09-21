package idutil

import "testing"

func TestMakeID(t *testing.T) {
	cases := []struct {
		parts []string
		want  string
	}{
		{[]string{"main.go"}, "main_go"},
		{[]string{"server", "HandleRequest"}, "server_handlerequest"},
		{[]string{"  foo  ", "Bar()"}, "foo_bar"},
		{[]string{"a__b"}, "a_b"},
		{[]string{"_._leading"}, "leading"},
		{[]string{"café"}, "café"}, // NFKC keeps the composed letter, fold lowercases
		{[]string{"", "x"}, "x"},
	}
	for _, c := range cases {
		if got := MakeID(c.parts...); got != c.want {
			t.Errorf("MakeID(%q) = %q, want %q", c.parts, got, c.want)
		}
	}
}

func TestNormalizeIDMatchesMakeID(t *testing.T) {
	// An ID produced by MakeID must be a fixed point of NormalizeID, so edge
	// endpoints reconcile to the same key the node was stored under.
	id := MakeID("pkg", "SomeType")
	if got := NormalizeID(id); got != id {
		t.Errorf("NormalizeID(%q) = %q, want stable", id, got)
	}
	if got := NormalizeID("Some-Type"); got != "some_type" {
		t.Errorf("NormalizeID(Some-Type) = %q, want some_type", got)
	}
}

func TestCleanIsIdempotentAndCaselessStable(t *testing.T) {
	// Casefolding expands some characters into a base letter plus a combining
	// mark; folding after the non-word filter left that mark unfiltered, so the
	// result was not a fixed point of NormalizeID (upstream's ghost-node class).
	for _, in := range []string{
		"İslemYap",     // Turkish dotted capital I -> "i" + U+0307
		"ᾴ",            // Greek alpha with oxia and ypogegrammeni
		"́ͅ",           // bare ypogegrammeni + combining acute
		"pkg.SomeType", // ASCII control
		"café",
	} {
		id := MakeID(in)
		if got := NormalizeID(id); got != id {
			t.Errorf("NormalizeID(MakeID(%q)) = %q, want %q", in, got, id)
		}
		if got := MakeID(fold.String(in)); got != id {
			t.Errorf("MakeID(fold(%q)) = %q, want %q", in, got, id)
		}
	}
}
