package extract

import (
	"slices"
	"testing"
	"unsafe"
)

// goCalls types receivers from the whole function in one walk: a local declared
// after the call still types it, and only calls inside the body count.
func TestGoCallsOneWalk(t *testing.T) {
	src := `package p

type Foo struct{}

func newFoo() *Foo { return &Foo{} }

func (s *Foo) run(a [len("xy")]int, w Writer) {
	early.Do()
	s.step()
	w.Write()
	plain()
	late.Close()
	var early Foo
	late := newFoo()
	for _, w := range a {
		w.Skip()
	}
}
`
	res := FileFromBytes("p/a.go", []byte(src))
	type call struct{ callee, recv, loc string }
	var got []call
	for _, n := range res.Calls {
		c := res.call(n)
		if c.CallerID == res.Defs[len(res.Defs)-1].ID {
			got = append(got, call{c.Callee, c.Recv, c.Loc.String()})
		}
	}
	// w is a parameter and a range variable, so it stays untyped; len in the
	// parameter list is not a call of the body.
	want := []call{
		{"Do", "Foo", "L8"}, {"step", "self", "L9"}, {"Write", "w", "L10"},
		{"plain", "", "L11"}, {"Close", "Foo", "L12"}, {"newFoo", "", "L14"}, {"Skip", "w", "L16"},
	}
	if len(got) != len(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("call %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// A text is held once per file, and a call that numbers a text the result
// does not have is found.
func TestCallText(t *testing.T) {
	res := FileFromBytes("p/a.go", []byte("package p\n\nfunc f() {\n\tx.g()\n\tx.g()\n\tg()\n}\n"))
	if len(res.Calls) != 3 || unsafe.Sizeof(res.Calls[0]) != 16 {
		t.Fatalf("calls = %+v, size %d", res.Calls, unsafe.Sizeof(res.Calls[0]))
	}
	want := []string{"", res.Defs[0].ID, "g", "x"}
	if !slices.Equal(res.CallText, want) {
		t.Errorf("CallText = %q, want %q", res.CallText, want)
	}
	if !res.CallsOK() {
		t.Error("CallsOK = false for an extracted result")
	}
	for _, bad := range []Result{
		{Calls: []Call{{Callee: 1}}},
		{Calls: []Call{{Recv: 2}}, CallText: []string{"", "a"}},
		{Calls: []Call{{CallerID: 1}}, CallText: []string{"x", "a"}},
	} {
		if bad.CallsOK() {
			t.Errorf("CallsOK = true for %+v", bad)
		}
	}
}
