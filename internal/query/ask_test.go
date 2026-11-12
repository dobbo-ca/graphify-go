package query

import (
	"fmt"
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
	out := Ask(g, "where is authValidate", false, 2, 2000, nil)
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
	out := Ask(g, "authValidate", false, 2, 2000, nil)
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
	if out := Ask(g, "nonexistentxyz", false, 2, 2000, nil); out != "No matching nodes found." {
		t.Errorf("got %q, want no-match message", out)
	}
}

func TestAskDFSMode(t *testing.T) {
	g := loadAsk(t)
	out := Ask(g, "authValidate", true, 2, 2000, nil)
	if !strings.Contains(out, "Traversal: DFS depth=2") {
		t.Errorf("missing DFS header: %q", out)
	}
}

func TestAskBudgetTruncates(t *testing.T) {
	g := loadAsk(t)
	// A tiny budget forces the truncation branch.
	out := Ask(g, "authValidate", false, 2, 5, nil)
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

func TestAskEmitsEdgeBetweenSeeds(t *testing.T) {
	g := loadAsk(t)
	// Both endpoints are seeds, so the traversal itself records no edge between
	// them; the induced-edge pass must still emit it, exactly once.
	for _, dfs := range []bool{false, true} {
		out := Ask(g, "authValidate checkToken", dfs, 1, 2000, nil)
		n := 0
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "EDGE ") &&
				strings.Contains(line, "authValidate()") && strings.Contains(line, "checkToken()") {
				n++
			}
		}
		if n != 1 {
			t.Errorf("dfs=%v: want 1 authValidate<->checkToken edge, got %d:\n%s", dfs, n, out)
		}
	}
}

// contextGraph gives authValidate one `calls` neighbour and one `imports`
// neighbour so relation filtering can be observed.
const contextGraph = `{
  "directed": true, "multigraph": false, "graph": {},
  "nodes": [
    {"id":"auth_validate","label":"authValidate()","file_type":"code","source_file":"auth.go","source_location":"L10","community":0,"norm_label":"authvalidate()"},
    {"id":"auth_check","label":"checkToken()","file_type":"code","source_file":"auth.go","source_location":"L20","community":0,"norm_label":"checktoken()"},
    {"id":"crypto_mod","label":"crypto","file_type":"code","source_file":"crypto.go","source_location":"L1","community":1,"norm_label":"crypto"}
  ],
  "links": [
    {"source":"auth_validate","target":"auth_check","relation":"calls","confidence":"INFERRED"},
    {"source":"auth_validate","target":"crypto_mod","relation":"imports","confidence":"EXPLICIT"}
  ]
}`

func TestAskContextFilter(t *testing.T) {
	p := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(p, []byte(contextGraph), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	unfiltered := Ask(g, "authValidate", false, 2, 2000, nil)
	if !strings.Contains(unfiltered, "checkToken()") || !strings.Contains(unfiltered, "crypto") {
		t.Fatalf("expected both neighbours without a filter:\n%s", unfiltered)
	}
	out := Ask(g, "authValidate", false, 2, 2000, []string{"calls"})
	if !strings.Contains(out, "checkToken()") {
		t.Errorf("calls neighbour missing under --context calls:\n%s", out)
	}
	if strings.Contains(out, "crypto") {
		t.Errorf("imports neighbour leaked under --context calls:\n%s", out)
	}
	if dfs := Ask(g, "authValidate", true, 2, 2000, []string{"calls"}); strings.Contains(dfs, "crypto") {
		t.Errorf("imports neighbour leaked under DFS --context calls:\n%s", dfs)
	}
}

// A node's Terraform attributes feed the search text, so an agent can ask
// "which resource runs t3.large" and land on the resource that sets it.
func TestAskMatchesAttributes(t *testing.T) {
	src := `{
  "directed": true, "multigraph": false, "graph": {},
  "nodes": [
    {"id":"web","label":"aws_instance.web","file_type":"code","source_file":"main.tf","source_location":"L1","community":0,"norm_label":"aws_instance.web","attributes":{"instance_type":"t3.large"}},
    {"id":"db","label":"aws_instance.db","file_type":"code","source_file":"main.tf","source_location":"L9","community":0,"norm_label":"aws_instance.db","attributes":{"instance_type":"t3.micro"}}
  ],
  "links": []
}`
	p := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	out := Ask(g, "t3.large", false, 1, 2000, nil)
	if !strings.HasPrefix(firstNodeLine(out), "NODE aws_instance.web") {
		t.Errorf("attribute match did not seed aws_instance.web, got:\n%s", out)
	}
}

func nodeJSON(id, label, file, loc string) string {
	return `{"id":"` + id + `","label":"` + label + `","file_type":"code","source_file":"` + file + `","source_location":"` + loc + `","community":0}`
}

func linkJSON(src, tgt, rel, conf, file, loc, extra string) string {
	return `{"source":"` + src + `","target":"` + tgt + `","relation":"` + rel + `","confidence":"` + conf + `","source_file":"` + file + `","source_location":"` + loc + `"` + extra + `}`
}

func graphJSON(nodes, links []string) string {
	return `{"directed":false,"multigraph":true,"graph":{},"nodes":[` + strings.Join(nodes, ",") + `],"links":[` + strings.Join(links, ",") + `]}`
}

func TestAskStoredDirectionAndSite(t *testing.T) {
	g := loadJSON(t, graphJSON([]string{
		nodeJSON("m_py", "m.py", "m.py", "L1"),
		nodeJSON("m_py_alpha", "alpha()", "m.py", "L1"),
		nodeJSON("m_py_beta", "beta()", "m.py", "L5"),
	}, []string{
		linkJSON("m_py", "m_py_alpha", "contains", "EXTRACTED", "m.py", "L1", ""),
		linkJSON("m_py", "m_py_beta", "contains", "EXTRACTED", "m.py", "L5", ""),
		linkJSON("m_py_beta", "m_py_alpha", "calls", "INFERRED", "m.py", "L6", ""),
	}))
	want := "Traversal: BFS depth=2 | Start: [beta()] | 3 nodes found\n\nNODE beta() [src=m.py loc=L5 community=0]\nNODE m.py [src=m.py loc=L1 community=0]\nNODE alpha() [src=m.py loc=L1 community=0]\nEDGE m.py --contains [EXTRACTED]--> beta() at=m.py:L5\nEDGE beta() --calls [INFERRED]--> alpha() at=m.py:L6\nEDGE m.py --contains [EXTRACTED]--> alpha() at=m.py:L1"
	if got := Ask(g, "beta", false, 2, 2000, nil); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAskParallelRelations(t *testing.T) {
	g := loadJSON(t, graphJSON([]string{
		nodeJSON("a_py", "a.py", "a.py", "L1"),
		nodeJSON("a_py_alpha", "Alpha", "a.py", "L1"),
		nodeJSON("a_py_make_alpha", "make_alpha()", "a.py", "L4"),
	}, []string{
		linkJSON("a_py", "a_py_alpha", "contains", "EXTRACTED", "a.py", "L1", ""),
		linkJSON("a_py", "a_py_make_alpha", "contains", "EXTRACTED", "a.py", "L4", ""),
		linkJSON("a_py_make_alpha", "a_py_alpha", "calls", "INFERRED", "a.py", "L5", ""),
		linkJSON("a_py_make_alpha", "a_py_alpha", "references", "EXTRACTED", "a.py", "L4", ""),
	}))
	want := "Traversal: BFS depth=2 | Start: [make_alpha()] | 3 nodes found\n\nNODE make_alpha() [src=a.py loc=L4 community=0]\nNODE Alpha [src=a.py loc=L1 community=0]\nNODE a.py [src=a.py loc=L1 community=0]\nEDGE a.py --contains [EXTRACTED]--> make_alpha() at=a.py:L4\nEDGE make_alpha() --calls [INFERRED]--> Alpha at=a.py:L5\nEDGE make_alpha() --references [EXTRACTED]--> Alpha at=a.py:L4\nEDGE a.py --contains [EXTRACTED]--> Alpha at=a.py:L1"
	if got := Ask(g, "make_alpha", false, 2, 2000, nil); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	got := Ask(g, "make_alpha", false, 2, 2000, []string{"calls"})
	if !strings.Contains(got, "2 nodes found") || !strings.HasSuffix(got, "EDGE make_alpha() --calls [INFERRED]--> Alpha at=a.py:L5") || strings.Contains(got, "references") {
		t.Errorf("relation filter: %s", got)
	}
}

func TestAskMutualContextSelfLoop(t *testing.T) {
	g := loadJSON(t, graphJSON([]string{
		nodeJSON("a", "alpha()", "m.py", "L1"),
		nodeJSON("b", "bravo()", "m.py", "L1"),
	}, []string{
		linkJSON("a", "b", "calls", "EXTRACTED", "m.py", "L5", `,"context":"call"`),
		linkJSON("b", "a", "calls", "EXTRACTED", "m.py", "L9", ""),
		linkJSON("a", "b", "references", "EXTRACTED", "m.py", "L4", ""),
		linkJSON("a", "a", "calls", "EXTRACTED", "m.py", "L2", ""),
	}))
	want := []string{
		"EDGE alpha() --calls [EXTRACTED]--> bravo() at=m.py:L5",
		"EDGE bravo() --calls [EXTRACTED]--> alpha() at=m.py:L9",
		"EDGE alpha() --references [EXTRACTED]--> bravo() at=m.py:L4",
	}
	var got []string
	for _, l := range strings.Split(Ask(g, "alpha", false, 2, 2000, nil), "\n") {
		if strings.HasPrefix(l, "EDGE ") {
			got = append(got, l)
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAskHopOrder(t *testing.T) {
	g := loadJSON(t, graphJSON([]string{
		nodeJSON("s", "zeta()", "m.py", "L1"),
		nodeJSON("x", "xray()", "m.py", "L2"),
		nodeJSON("h", "hub()", "m.py", "L3"),
		nodeJSON("l1", "leaf1()", "m.py", "L4"),
		nodeJSON("l2", "leaf2()", "m.py", "L5"),
		nodeJSON("l3", "leaf3()", "m.py", "L6"),
	}, []string{
		linkJSON("s", "x", "calls", "EXTRACTED", "m.py", "L1", ""),
		linkJSON("x", "h", "calls", "EXTRACTED", "m.py", "L2", ""),
		linkJSON("l1", "h", "calls", "EXTRACTED", "m.py", "L4", ""),
		linkJSON("l2", "h", "calls", "EXTRACTED", "m.py", "L5", ""),
		linkJSON("l3", "h", "calls", "EXTRACTED", "m.py", "L6", ""),
	}))
	want := "Traversal: BFS depth=2 | Start: [zeta()] | 3 nodes found\n\nNODE zeta() [src=m.py loc=L1 community=0]\nNODE xray() [src=m.py loc=L2 community=0]\nNODE hub() [src=m.py loc=L3 community=0]\nEDGE zeta() --calls [EXTRACTED]--> xray() at=m.py:L1\nEDGE xray() --calls [EXTRACTED]--> hub() at=m.py:L2"
	if got := Ask(g, "zeta", false, 2, 2000, nil); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestHubThresholdCountsIsolatedNodes(t *testing.T) {
	nodes := []string{nodeJSON("seedling", "seedling", "m.py", "L1"), nodeJSON("hubone", "hubone", "m.py", "L1"),
		nodeJSON("hubtwo", "hubtwo", "m.py", "L1"), nodeJSON("x1", "x1", "m.py", "L1"), nodeJSON("x2", "x2", "m.py", "L1")}
	links := []string{linkJSON("seedling", "hubtwo", "calls", "EXTRACTED", "", "", ""),
		linkJSON("hubone", "x1", "calls", "EXTRACTED", "", "", ""), linkJSON("hubone", "x2", "calls", "EXTRACTED", "", "", "")}
	for k := 0; k < 52; k++ {
		id := fmt.Sprintf("leaf%d", k)
		nodes = append(nodes, nodeJSON(id, id, "m.py", "L1"))
		links = append(links, linkJSON("hubone", id, "calls", "EXTRACTED", "", "", ""), linkJSON("hubtwo", id, "calls", "EXTRACTED", "", "", ""))
	}
	for k := 0; k < 50; k++ {
		id := fmt.Sprintf("iso%d", k)
		nodes = append(nodes, nodeJSON(id, id, "m.py", "L1"))
	}
	g := loadJSON(t, graphJSON(nodes, links))
	if got := Ask(g, "seedling", false, 2, 2000, nil); !strings.Contains(got, "| 2 nodes found") {
		t.Errorf("got header: %.120s", got)
	}
}

func TestHubThresholdContextKeepsDanglingNodes(t *testing.T) {
	var nodes, links []string
	add := func(id string) { nodes = append(nodes, nodeJSON(id, id, "m.py", "L1")) }
	rel := func(a, b, r string) { links = append(links, linkJSON(a, b, r, "EXTRACTED", "", "", "")) }
	add("seedling")
	add("x")
	add("y")
	rel("seedling", "x", "rela")
	rel("x", "seedling", "rela")
	for k := 0; k < 29; k++ {
		id := fmt.Sprintf("lx%d", k)
		add(id)
		rel("x", id, "rela")
		rel("x", id, "rela")
	}
	for k := 0; k < 40; k++ {
		id := fmt.Sprintf("ly%d", k)
		add(id)
		rel("y", id, "rela")
		rel("y", id, "rela")
	}
	for k := 0; k < 28; k++ {
		add(fmt.Sprintf("pad%d", k))
	}
	rel("pad0", "ghost", "relb")
	g := loadJSON(t, graphJSON(nodes, links))
	if got := Ask(g, "seedling", false, 2, 2000, []string{"rela"}); !strings.Contains(got, "| 2 nodes found") {
		t.Errorf("got header: %.120s", got)
	}
}
