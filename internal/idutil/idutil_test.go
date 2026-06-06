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
