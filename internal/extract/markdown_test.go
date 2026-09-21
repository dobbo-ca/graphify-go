package extract

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// TestExtractMarkdown resolves a small OKF-style markdown bundle and checks that
// each file becomes one concept node carrying its frontmatter `type`, that
// markdown links become `references` edges (absolute /tables/... and relative),
// that http(s) links are ignored, and that a directory's index.md `contains` its
// siblings.
func TestExtractMarkdown(t *testing.T) {
	root := "testdata/mdproj"
	files := []string{"index.md", "glossary.md", "tables/index.md", "tables/orders.md"}

	var results []Result
	for _, f := range files {
		r, err := File(root, f)
		if err != nil {
			t.Fatalf("File(%s): %v", f, err)
		}
		results = append(results, r)
	}
	ext := Resolve(results, files)

	// One concept node per file, with the frontmatter `type` as file_type.
	type want struct{ id, fileType, label string }
	wantNodes := []want{
		{"index", "bundle", "Analytics Knowledge Bundle"},
		{"glossary", "glossary", "Business Glossary"},
		{"tables_index", "index", "Tables"},
		{"tables_orders", "table", "orders"},
	}
	byID := map[string]struct {
		fileType, label string
	}{}
	for _, n := range ext.Nodes {
		byID[n.ID] = struct{ fileType, label string }{n.FileType, n.Label}
	}
	concepts := 0
	for _, n := range ext.Nodes {
		if n.FileType != "heading" {
			concepts++
		}
	}
	if concepts != len(wantNodes) {
		t.Errorf("concept node count = %d, want %d (one concept per file)", concepts, len(wantNodes))
	}
	for _, w := range wantNodes {
		got, ok := byID[w.id]
		if !ok {
			t.Errorf("missing concept node %q", w.id)
			continue
		}
		if got.fileType != w.fileType {
			t.Errorf("node %q file_type = %q, want %q (frontmatter type)", w.id, got.fileType, w.fileType)
		}
		if got.label != w.label {
			t.Errorf("node %q label = %q, want %q (frontmatter title)", w.id, got.label, w.label)
		}
	}

	has := func(src, rel, tgt string) bool {
		for _, e := range ext.Edges {
			if e.Relation == rel && e.Source == src && e.Target == tgt {
				return true
			}
		}
		return false
	}

	tests := []struct {
		name          string
		src, rel, tgt string
		wantPresent   bool
	}{
		{"absolute link becomes references edge", "index", "references", "tables_orders", true},
		{"relative link becomes references edge", "index", "references", "glossary", true},
		{"cross-dir absolute link resolves", "tables_orders", "references", "glossary", true},
		{"index.md contains sibling", "tables_index", "contains", "tables_orders", true},
		{"external http link is ignored", "index", "references", "tree_sitter_github_io", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := has(tc.src, tc.rel, tc.tgt); got != tc.wantPresent {
				t.Errorf("edge %s --%s--> %s present=%v, want %v", tc.src, tc.rel, tc.tgt, got, tc.wantPresent)
			}
		})
	}

	// description and tags ride along on ComputedName as searchable metadata.
	var orders string
	for _, n := range ext.Nodes {
		if n.ID == "tables_orders" {
			orders = n.ComputedName
		}
	}
	if orders == "" {
		t.Error("tables_orders ComputedName empty; expected description/tags metadata")
	}
}

// TestSplitFrontmatter covers frontmatter parsing edge cases: no frontmatter, a
// well-formed block, quoted scalars, and an unterminated block (whole source is
// body, no keys).
func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantType string
		wantBody string
	}{
		{"no frontmatter", "# Title\n\nbody", "", "# Title\n\nbody"},
		{"typed block", "---\ntype: table\n---\n# T\nbody", "table", "# T\nbody"},
		{"quoted scalar", "---\ntype: \"table\"\n---\nbody", "table", "body"},
		{"unterminated", "---\ntype: table\nbody without close", "", "---\ntype: table\nbody without close"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm, body := splitFrontmatter(tc.src)
			if fm["type"] != tc.wantType {
				t.Errorf("type = %q, want %q", fm["type"], tc.wantType)
			}
			if body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

// TestResolveMDTarget covers link-target resolution: absolute vs relative,
// anchors stripped, external schemes ignored, extensionless targets, and
// off-corpus targets.
func TestResolveMDTarget(t *testing.T) {
	corpus := map[string]bool{
		"index.md":         true,
		"tables/orders.md": true,
	}
	tests := []struct {
		name, from, target, want string
	}{
		{"absolute with ext", "index.md", "/tables/orders.md", "tables/orders.md"},
		{"relative from subdir", "tables/orders.md", "../index.md", "index.md"},
		{"anchor stripped", "index.md", "/tables/orders.md#cols", "tables/orders.md"},
		{"extensionless absolute", "index.md", "/tables/orders", "tables/orders.md"},
		{"http ignored", "index.md", "https://example.com", ""},
		{"mailto ignored", "index.md", "mailto:a@b.com", ""},
		{"in-page anchor only", "index.md", "#section", ""},
		{"off corpus", "index.md", "/missing.md", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveMDTarget(tc.from, tc.target, corpus); got != tc.want {
				t.Errorf("resolveMDTarget(%q, %q) = %q, want %q", tc.from, tc.target, got, tc.want)
			}
		})
	}
}

// TestExtractMarkdownHeadingsRefs covers the additive depth: ATX headings become
// nodes with `contains` edges (nested by level), and `[[wikilinks]]`,
// reference-style `[a]: url` definitions and inline `code` spans become MDRefs
// alongside inline links. It then runs Resolve to confirm an in-corpus wikilink
// stitches into a `references` edge while an external reference definition and an
// unresolved backtick symbol do not.
func TestExtractMarkdownHeadingsRefs(t *testing.T) {
	doc := "# Top\n\n" +
		"Intro links to [[other]] and mentions `Symbol` inline.\n\n" +
		"[a]: http://x\n\n" +
		"## Nested\n\n" +
		"More detail about `Symbol`.\n"

	r := extractMarkdown("notes.md", []byte(doc))

	// Heading nodes, typed "heading", one per ATX heading.
	nodes := map[string]model.Node{}
	for _, n := range r.Nodes {
		nodes[n.ID] = n
	}
	for _, w := range []struct{ id, label string }{{"notes_top", "Top"}, {"notes_nested", "Nested"}} {
		n, ok := nodes[w.id]
		if !ok {
			t.Fatalf("missing heading node %q", w.id)
		}
		if n.FileType != "heading" {
			t.Errorf("heading %q file_type = %q, want heading", w.id, n.FileType)
		}
		if n.Label != w.label {
			t.Errorf("heading %q label = %q, want %q", w.id, n.Label, w.label)
		}
	}

	hasContain := func(src, tgt string) bool {
		for _, e := range r.Edges {
			if e.Relation == "contains" && e.Source == src && e.Target == tgt {
				return true
			}
		}
		return false
	}
	if !hasContain("notes", "notes_top") {
		t.Error("missing contains edge notes --contains--> notes_top (file -> heading)")
	}
	if !hasContain("notes_top", "notes_nested") {
		t.Error("missing contains edge notes_top --contains--> notes_nested (heading nesting)")
	}

	// Wikilink, reference definition and backtick each captured as an MDRef.
	refTargets := map[string]bool{}
	for _, m := range r.MDRefs {
		refTargets[m.Target] = true
	}
	for _, tgt := range []string{"other", "http://x", "Symbol"} {
		if !refTargets[tgt] {
			t.Errorf("missing MDRef target %q", tgt)
		}
	}

	// Resolve: the wikilink resolves to the sibling doc; the external ref def and
	// the unresolved backtick symbol produce no `references` edge.
	other := extractMarkdown("other.md", []byte("# Other\n"))
	ext := Resolve([]Result{r, other}, []string{"notes.md", "other.md"})
	refs := 0
	toOther := false
	for _, e := range ext.Edges {
		if e.Relation == "references" && e.Source == "notes" {
			refs++
			if e.Target == "other" {
				toOther = true
			}
		}
	}
	if !toOther {
		t.Error("wikilink [[other]] did not resolve to references edge notes --references--> other")
	}
	if refs != 1 {
		t.Errorf("references edges from notes = %d, want 1 (external ref def and backtick symbol must not resolve)", refs)
	}
}

// TestResolveBacktickCodeSymbol covers backtick code-span resolution: a unique
// code definition named by an inline `code` span becomes a `references` edge from
// the doc concept to that code node; an ambiguous name (defined twice) and a
// noise token with no matching definition both drop.
func TestResolveBacktickCodeSymbol(t *testing.T) {
	doc := "# Doc\n\nUses `UniqueSymbolXyz`, `Dupe` and `true` inline.\n"
	r := extractMarkdown("notes.md", []byte(doc))

	code := Result{Defs: []Def{
		{ID: "UNIQUE", Name: "UniqueSymbolXyz", File: "a.go"},
		{ID: "DUPE1", Name: "Dupe", File: "a.go"},
		{ID: "DUPE2", Name: "Dupe", File: "b.go"},
	}}

	ext := Resolve([]Result{r, code}, []string{"notes.md", "a.go", "b.go"})

	hasRef := func(src, tgt string) bool {
		for _, e := range ext.Edges {
			if e.Relation == "references" && e.Source == src && e.Target == tgt {
				return true
			}
		}
		return false
	}

	// (a) unique backtick symbol -> references edge to the code def.
	if !hasRef("notes", "UNIQUE") {
		t.Error("expected notes --references--> UNIQUE (unique backtick code symbol)")
	}
	// (b) ambiguous symbol (defined twice) -> no edge.
	if hasRef("notes", "DUPE1") || hasRef("notes", "DUPE2") {
		t.Error("did not expect a references edge for ambiguous backtick symbol `Dupe`")
	}
	// (c) exactly one references edge: `true` (noise, no def) also drops.
	refs := 0
	for _, e := range ext.Edges {
		if e.Relation == "references" && e.Source == "notes" {
			refs++
		}
	}
	if refs != 1 {
		t.Errorf("references edges from notes = %d, want 1 (only the unique code symbol resolves)", refs)
	}
}
