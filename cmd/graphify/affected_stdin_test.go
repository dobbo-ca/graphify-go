package main

import (
	"reflect"
	"strings"
	"testing"
)

// `gh pr diff <n> --name-only | graphify affected -` feeds a path per line;
// blank and whitespace-padded lines must not become bogus seeds.
func TestReadPathList(t *testing.T) {
	got := readPathList(strings.NewReader("a/b.go\n\n  c.go  \n\n"))
	want := []string{"a/b.go", "c.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readPathList = %q, want %q", got, want)
	}
	if got := readPathList(strings.NewReader("")); len(got) != 0 {
		t.Fatalf("empty stdin = %q, want none", got)
	}
}
