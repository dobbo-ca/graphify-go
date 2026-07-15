package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// askGraph wires a rare identifier (authValidate) one hop from a common helper,
// plus an unrelated noise node, so TF-IDF scoring and seed selection are exercised.
const askGraph = `{
  "directed": true, "multigraph": false, "graph": {},
  "nodes": [
    {"id":"auth_validate","label":"authValidate()","file_type":"code","source_file":"auth.go","source_location":"L10","community":0,"norm_label":"authvalidate()"},
    {"id":"auth_check","label":"checkToken()","file_type":"code","source_file":"auth.go","source_location":"L20","community":0,"norm_label":"checktoken()"},
    {"id":"util_log","label":"log()","file_type":"code","source_file":"util.go","source_location":"L1","community":1,"norm_label":"log()"},
    {"id":"noise","label":"unrelated()","file_type":"code","source_file":"other.go","source_location":"L1","community":2,"norm_label":"unrelated()"}
  ],
  "links": [
    {"source":"auth_validate","target":"auth_check","relation":"calls","confidence":"INFERRED"},
    {"source":"auth_check","target":"util_log","relation":"calls","confidence":"INFERRED"}
  ]
}`

func loadAsk(t *testing.T) *Graph {
	t.Helper()
	p := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(p, []byte(askGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestAskSeedsRareIdentifier(t *testing.T) {
	g := loadAsk(t)
	out := Ask(g, "where is authValidate", false, 2, 2000)
	if !strings.Contains(out, "Traversal: BFS depth=2") {
		t.Errorf("missing BFS header: %q", out)
	}
	// authValidate is the dominant exact-ish match and must lead the output.
	if !strings.HasPrefix(firstNodeLine(out), "NODE authValidate()") {
		t.Errorf("seed not rendered first, got:\n%s", out)
	}
	// The unrelated noise node shares no query token and must not be pulled in.
	if strings.Contains(out, "unrelated()") {
		t.Errorf("noise node leaked into result:\n%s", out)
	}
}

func TestAskTraversesToNeighbors(t *testing.T) {
	g := loadAsk(t)
	out := Ask(g, "authValidate", false, 2, 2000)
	// depth 2 from authValidate reaches checkToken (1 hop) and log (2 hops).
	for _, want := range []string{"checkToken()", "log()"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in 2-hop traversal:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "EDGE authValidate() --calls [INFERRED]--> checkToken()") {
		t.Errorf("expected rendered edge, got:\n%s", out)
	}
}

func TestAskNoMatch(t *testing.T) {
	g := loadAsk(t)
	if out := Ask(g, "nonexistentxyz", false, 2, 2000); out != "No matching nodes found." {
		t.Errorf("got %q, want no-match message", out)
	}
}

func TestAskDFSMode(t *testing.T) {
	g := loadAsk(t)
	out := Ask(g, "authValidate", true, 2, 2000)
	if !strings.Contains(out, "Traversal: DFS depth=2") {
		t.Errorf("missing DFS header: %q", out)
	}
}

func TestAskBudgetTruncates(t *testing.T) {
	g := loadAsk(t)
	// A tiny budget forces the truncation branch.
	out := Ask(g, "authValidate", false, 2, 5)
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation marker with tiny budget:\n%s", out)
	}
}

func TestQueryTermsDropsShortAndPunctuation(t *testing.T) {
	got := queryTerms("Is an auth-token ok?")
	// "is", "an", "ok" are <=2 chars and dropped; "auth"/"token" survive,
	// the hyphen splits the compound, and the "?" is stripped.
	want := map[string]bool{"auth": true, "token": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want keys of %v", got, want)
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected term %q in %v", g, got)
		}
	}
}

func TestPickSeedsGapCutoff(t *testing.T) {
	scored := []scoredNode{{1000, "a", 1}, {1.0, "b", 1}, {0.9, "c", 1}}
	seeds := pickSeeds(&Graph{}, scored, 3, 0.2)
	if len(seeds) != 1 || seeds[0] != "a" {
		t.Errorf("gap cutoff failed: %v, want [a]", seeds)
	}
}

// A swarm of homonymous nodes (three `GET` handlers) must not fill every seed
// slot: dedup by normalized label seeds one representative and leaves room for
// the distinct relevant node (#1766).
func TestPickSeedsDedupsByLabel(t *testing.T) {
	g := &Graph{byID: map[string]*Node{
		"get1":  {NormLabel: "get()"},
		"get2":  {NormLabel: "get()"},
		"get3":  {NormLabel: "get()"},
		"users": {NormLabel: "usersmodel()"},
	}}
	scored := []scoredNode{
		{1000, "get1", 5}, {1000, "get2", 5}, {1000, "get3", 5}, {900, "users", 12},
	}
	seeds := pickSeeds(g, scored, 3, 0.2)
	if len(seeds) != 2 {
		t.Fatalf("expected 2 seeds (one GET + users), got %v", seeds)
	}
	set := map[string]bool{}
	for _, s := range seeds {
		set[s] = true
	}
	if !set["users"] {
		t.Errorf("distinct users node must be seeded, got %v", seeds)
	}
	gets := 0
	for _, id := range []string{"get1", "get2", "get3"} {
		if set[id] {
			gets++
		}
	}
	if gets != 1 {
		t.Errorf("exactly one GET homonym should be seeded, got %d in %v", gets, seeds)
	}
}

// Case-variant homonyms (GET/Get/get) must collapse to one seed — exercises
// normLabel's case-folding, not just exact-string equality.
func TestPickSeedsDedupNormalizesCase(t *testing.T) {
	g := &Graph{byID: map[string]*Node{
		"h1": {NormLabel: "GET()"},
		"h2": {NormLabel: "Get()"},
		"h3": {NormLabel: "get()"},
		"u":  {NormLabel: "usersmodel()"},
	}}
	scored := []scoredNode{{1000, "h1", 5}, {1000, "h2", 5}, {1000, "h3", 5}, {900, "u", 12}}
	seeds := pickSeeds(g, scored, 3, 0.2)
	if len(seeds) != 2 {
		t.Fatalf("case-variant homonyms must collapse to one seed, got %v", seeds)
	}
}

// Label-less nodes must NOT collapse into a single seed — they stay distinct via
// the id fallback (mirrors upstream `... or nid`).
func TestPickSeedsKeepsLabellessDistinct(t *testing.T) {
	g := &Graph{byID: map[string]*Node{
		"n1": {}, "n2": {}, "n3": {},
	}}
	scored := []scoredNode{{1000, "n1", 0}, {1000, "n2", 0}, {1000, "n3", 0}}
	seeds := pickSeeds(g, scored, 3, 0.2)
	if len(seeds) != 3 {
		t.Errorf("distinct label-less nodes must not be deduped to one; got %v", seeds)
	}
}

// Question/filler stopwords (English and German/Romance) are dropped so the
// content noun drives seeding (#1900).
func TestQueryTermsDropsStopwords(t *testing.T) {
	if got := queryTerms("Wie funktioniert die Authentifizierung?"); len(got) != 1 || got[0] != "authentifizierung" {
		t.Errorf("German stopwords not dropped: got %v, want [authentifizierung]", got)
	}
	if got := queryTerms("how does the cache work"); len(got) != 1 || got[0] != "cache" {
		t.Errorf("English stopwords not dropped: got %v, want [cache]", got)
	}
}

// An all-stopword query falls back to the unfiltered searchable tokens so it
// still seeds on something.
func TestQueryTermsAllStopwordsFallback(t *testing.T) {
	if got := queryTerms("wie funktioniert das"); len(got) != 3 {
		t.Errorf("all-stopword query should fall back to unfiltered terms, got %v", got)
	}
}

// Accented tokens must be kept whole (Unicode-aware tokenizer): an accented
// content noun survives while accented/ASCII stopwords around it drop. With an
// ASCII-only \w this splits "Größe" into unsearchable fragments and returns the
// four fillers via fallback instead.
func TestQueryTermsHandlesAccentedTokens(t *testing.T) {
	if got := queryTerms("warum funktioniert die Größe nicht"); len(got) != 1 || got[0] != "größe" {
		t.Errorf("accented content noun should survive whole, got %v, want [größe]", got)
	}
}

// firstNodeLine returns the first NODE line of rendered output.
func firstNodeLine(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "NODE ") {
			return l
		}
	}
	return ""
}
