package extract

import "testing"

func TestResolveModulePath(t *testing.T) {
	corpus := map[string]bool{
		"a/exact": true, "a/x.ts": true, "a/x.js": true, "a/y.d.ts": true,
		"a/dir/index.tsx": true, "a/both.js": true, "a/both/index.ts": true,
		"a/z.d.ts/index.js": true, "a/last/index.d.ts": true,
	}
	for base, want := range map[string]string{
		"a/exact": "a/exact",
		"a/x":     "a/x.ts", // extension order, not corpus order
		"a/y":     "a/y.d.ts",
		"a/dir":   "a/dir/index.tsx",
		"a/both":  "a/both.js", // a file beats a directory index
		"a/last":  "a/last/index.d.ts",
		"a/z":     "", // the index probe starts from the bare path again
		"a/none":  "",
		"":        "",
	} {
		if got := resolveModulePath(base, corpus); got != want {
			t.Errorf("resolveModulePath(%q) = %q, want %q", base, got, want)
		}
	}
}
