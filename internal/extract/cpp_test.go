package extract

import (
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestExtractCpp(t *testing.T) {
	root := "testdata/cppproj"
	files := []string{"util/math.cpp", "web/server.cpp"}

	// C++ dispatch is wired centrally; call extractCpp directly here so the test
	// is self-contained.
	var results []Result
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", f, err)
		}
		results = append(results, extractCpp(filepath.ToSlash(f), src))
	}
	ext := Resolve(results, files)

	labels := map[string]bool{}
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		labels[n.Label] = true
		id2label[n.ID] = n.Label
	}
	for _, want := range []string{"math.cpp", "server.cpp", "add()", "Server", "Server.start()", "boot()"} {
		if !labels[want] {
			t.Errorf("missing node label %q", want)
		}
	}

	has := func(srcLabel, rel, tgtLabel string) bool {
		for _, e := range ext.Edges {
			if e.Relation.String() == rel && id2label[e.Source] == srcLabel && id2label[e.Target] == tgtLabel {
				return true
			}
		}
		return false
	}
	// Inline method scoped under its class, with a contains edge.
	if !has("Server", "contains", "Server.start()") {
		t.Error("expected Server --contains--> Server.start")
	}
	// Same-file call: Server.start -> boot.
	if !has("Server.start()", "calls", "boot()") {
		t.Error("expected Server.start --calls--> boot")
	}
	// Cross-file call: boot -> add (unique global definition).
	if !has("boot()", "calls", "add()") {
		t.Error("expected cross-file boot --calls--> add")
	}

	// External import edge for <string>.
	rels := map[string]int{}
	for _, e := range ext.Edges {
		rels[e.Relation.String()]++
	}
	if rels["imports"] == 0 {
		t.Error("no external import edges (expected string)")
	}
}

// A `.h` declaring a C++ class routes to the C++ extractor; the C grammar has
// no class_specifier and would emit a function-shaped `Widget()`.
func TestCppHeaderRoutesToCppExtractor(t *testing.T) {
	labels := func(src string) map[string]bool {
		out := map[string]bool{}
		for _, n := range FileFromBytes("w.h", []byte(src)).Nodes {
			out[n.Label] = true
		}
		return out
	}

	got := labels("class Widget { public: void show(); virtual int area() const = 0; int count; };\n")
	if !got["Widget"] || got["Widget()"] {
		t.Errorf("C++ header: want class node Widget, got %v", got)
	}

	// A plain C header keeps the C extractor's output.
	plain := "#include <stdio.h>\nstruct point { int x; };\nint add(int a, int b);\n"
	want := map[string]bool{}
	for _, n := range extractC("w.h", []byte(plain)).Nodes {
		want[n.Label] = true
	}
	if got := labels(plain); len(got) != len(want) || isCppHeader([]byte(plain)) {
		t.Errorf("plain C header: got %v, want %v", got, want)
	}
}

// An export macro between `class`/`struct` and the name must not hide the type.
func TestCppExportMacroClass(t *testing.T) {
	src := "class Base {};\n" +
		"class Q_CORE_EXPORT\nWidget : public Base { public: void show() {} };\n" +
		"struct API Thing { int x; };\n" +
		"class MY_WIDGET final : public Base {};\n" +
		"class MACRO OTHER final : public Base {};\n" +
		"void F() {\n  for (class API v : items) {}\n  class API w{1};\n}\n"
	if out := blankCppExportMacros([]byte(src)); len(out) != len(src) ||
		!strings.Contains(string(out), "class              \nWidget") ||
		!strings.Contains(string(out), "(class API v :") || !strings.Contains(string(out), "class API w{1}") {
		t.Errorf("blanking changed offsets or touched a variable:\n%s", out)
	}

	res := FileFromBytes("w.h", []byte(src))
	labels := map[string]string{}
	for _, n := range res.Nodes {
		labels[n.Label] = n.SourceLocation
	}
	for _, want := range []string{"Widget", "Widget.show()", "Thing", "MY_WIDGET", "OTHER"} {
		if _, ok := labels[want]; !ok {
			t.Errorf("missing node %q in %v", want, labels)
		}
	}
	if labels["Thing"] != "L4" {
		t.Errorf("Thing at %s, want L4", labels["Thing"])
	}
	inherits := false
	for _, r := range res.TypeRefs {
		if r.FromID == "w_widget" && r.Name == "Base" && r.Relation == "inherits" {
			inherits = true
		}
	}
	if !inherits {
		t.Errorf("no Widget inherits Base ref in %v", res.TypeRefs)
	}
}

// The keyword scan must blank exactly what one unanchored regexp pass did.
func TestCppExportMacroScanMatchesRegexp(t *testing.T) {
	re := regexp.MustCompile(`\b(?:class|struct)\s+((?:[A-Z][A-Z0-9_]*\s+)+)([A-Za-z_]\w*(?:\s+final)?\s*)([:{])`)
	ref := func(src []byte) []byte {
		out := append([]byte(nil), src...)
		for _, m := range re.FindAllSubmatchIndex(src, -1) {
			if strings.HasSuffix(strings.TrimSpace(string(src[:m[0]])), "(") {
				continue
			}
			if rest := strings.TrimSpace(string(src[m[7]:])); src[m[6]] == '{' && rest != "" && strings.ContainsRune("0123456789\"'-", rune(rest[0])) {
				continue
			}
			end := m[3]
			if strings.TrimSpace(string(src[m[4]:m[5]])) == "final" {
				end = m[2] + strings.LastIndexAny(strings.TrimSpace(string(src[m[2]:m[3]])), " \t\r\n") + 1
			}
			for i := m[2]; i < end; i++ {
				if out[i] != '\r' && out[i] != '\n' {
					out[i] = ' '
				}
			}
		}
		return out
	}
	toks := []string{"class ", "struct\n", "class", " ", "\n", "\t", "API ", "A_B2 ", "API", "Foo", "Foo ", "final ", ":", "{", " {", "(", "1", "x", "_", "é", "\xff", ";"}
	rng, blanked := rand.New(rand.NewSource(1)), 0
	for i := 0; i < 200000; i++ {
		var sb strings.Builder
		for n := rng.Intn(14); n >= 0; n-- {
			sb.WriteString(toks[rng.Intn(len(toks))])
		}
		src := []byte(sb.String())
		got, want := blankCppExportMacros(src), ref(src)
		if string(got) != string(want) {
			t.Fatalf("%q: got %q, want %q", src, got, want)
		}
		if string(got) != string(src) {
			blanked++
		}
	}
	if blanked < 100 {
		t.Fatalf("only %d inputs blanked: the generator no longer reaches the match", blanked)
	}
	t.Log("blanked", blanked)
}
