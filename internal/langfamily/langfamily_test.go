package langfamily

import "testing"

func TestCross(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"main.py", "Svc.java", true},    // python vs jvm
		{"main.py", "Svc.kt", true},      // python vs jvm (kotlin)
		{"app.ts", "widget.kt", true},    // jsts vs jvm
		{"Svc.java", "Widget.kt", false}, // both jvm
		{"impl.c", "decl.h", false},      // both native
		{"a.ts", "b.tsx", false},         // both jsts
		{"a.py", "b.py", false},          // same family
		{"a.go", "b.go", false},          // same family
		{"App.vue", "util.py", true},     // sfc vs python
		{"App.svelte", "lib.ts", false},  // sfc is jsts
		{"Page.astro", "a.js", false},    // sfc is jsts
		{"App.vue", "Other.vue", false},  // both jsts
		{"README.md", "a.py", false},     // unknown family stays permissive
		{"a.py", "", false},              // empty file stays permissive
	}
	for _, c := range cases {
		if got := Cross(c.a, c.b); got != c.want {
			t.Errorf("Cross(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestComponent(t *testing.T) {
	for f, want := range map[string]bool{"App.vue": true, "P.SVELTE": true, "a.astro": true, "t.ejs": true, "a.ts": false, "": false} {
		if Component(f) != want {
			t.Errorf("Component(%q) = %v", f, !want)
		}
	}
}
