package langfamily

import "testing"

func TestCross(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"main.py", "Svc.java", true},   // python vs jvm
		{"main.py", "Svc.kt", true},     // python vs jvm (kotlin)
		{"app.ts", "widget.kt", true},   // jsts vs jvm
		{"Svc.java", "Widget.kt", false}, // both jvm
		{"impl.c", "decl.h", false},      // both native
		{"a.ts", "b.tsx", false},         // both jsts
		{"a.py", "b.py", false},          // same family
		{"a.go", "b.go", false},          // same family
		{"README.md", "a.py", false},     // unknown family stays permissive
		{"a.py", "", false},              // empty file stays permissive
	}
	for _, c := range cases {
		if got := Cross(c.a, c.b); got != c.want {
			t.Errorf("Cross(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
