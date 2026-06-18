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
	seeds := pickSeeds(scored, 3, 0.2)
	if len(seeds) != 1 || seeds[0] != "a" {
		t.Errorf("gap cutoff failed: %v, want [a]", seeds)
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
