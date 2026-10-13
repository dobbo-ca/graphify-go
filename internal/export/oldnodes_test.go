package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// wholeOldNodes is the decode scanOldNodes stands in for.
func wholeOldNodes(doc []byte) ([]oldNode, error) {
	var existing struct {
		Nodes []oldNode `json:"nodes"`
	}
	err := json.Unmarshal(doc, &existing)
	return existing.Nodes, err
}

// checkScan fails when scanOldNodes accepts doc but the whole decode
// disagrees. It reports whether the scan accepted.
func checkScan(t *testing.T, doc []byte) bool {
	t.Helper()
	got, ok := scanOldNodes(bytes.NewReader(doc))
	one, okOne := scanOldNodes(iotest.OneByteReader(bytes.NewReader(doc)))
	if ok != okOne || !slices.Equal(got, one) {
		t.Fatalf("scan depends on how the input is cut into reads: %q", doc)
	}
	if !ok {
		return false
	}
	want, err := wholeOldNodes(doc)
	if err != nil {
		t.Fatalf("scan accepted what the decoder refuses (%v): %q", err, doc)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("scan got %v, decoder %v: %q", got, want, doc)
	}
	return true
}

var scanSeeds = []string{
	`{"nodes":[{"id":"a","source_file":"a.go"},{"id":"b"}],"links":[]}`,
	"{\n  \"directed\": false,\n  \"graph\": {\"n\": 1, \"m\": {\".go\": 2}},\n  \"nodes\": [\n    {\n      \"id\": \"a\",\n      \"label\": \"A\",\n      \"source_file\": \"p/a.go\",\n      \"community\": 3\n    }\n  ],\n  \"links\": [{\"weight\": 1.5e-3, \"x\": [true, false, null, -0, 12.5E+2]}],\n  \"hyperedges\": []\n}\n",
	`{"nodes":[]}`, `{}`, `{"links":[1,2]}`, ` { "nodes" : [ null , {} , {"id":"é\n\"x","source_file":"\\"} ] } `,
	`{"nodes":[{"id":"a"}],"nodes":[{"id":"b"}]}`, `{"nodes":[{"id":"a","source_file":"s"},{"id":"c"}],"nodes":[{"id":"b"}]}`, `{"Nodes":[{"id":"a"}]}`, `{"nodes":[{"id":"a"}],"NODES":[]}`,
	`{"nodes":null}`, `{"nodes":{}}`, `{"nodes":[{"id":1}]}`, `{"nodes":[1]}`, `{"nodes":[{"ID":"a","Source_File":"b"}]}`,
	`{"nodes":[{"id":"a","id":"b"}]}`, `{"nodes":[{"id":"a"}]}`, `{"nodes":[{"id":"a"},]}`, `{"nodes":[{"id":"a"}],}`,
	`{"nodes":[{"id":"a"}]} x`, `{"nodes":[{"id":"a"}]`, `{"nodes":[{"id":"a"}]}{}`, `[]`, `null`, ``, `  `, `{"a":01}`, `{"a":1.}`, `{"a":-}`,
	`{"a":1e}`, `{"a":"\x"}`, `{"a":"\u12g4"}`, "{\"a\":\"\x01\"}", "{\"a\":\"\xff\"}", "{\"nodes\":[{\"id\":\"\xff\"}]}", `{"a":tru}`, `{"a":nul}`,
	`{"a" 1}`, `{"a":1 "b":2}`, `{a:1}`, `{"a":[1 2]}`, `{"a":{"b":}}`, `{"a":[[[[{"b":[{}]}]]]]}`, "{\"a\":1}\x00", "{\"a\":1\x00}",
	`{"nodes":[{"id":"a","source_file":"s"}], "a":1e5, "b":-0.0, "c":0, "d":"😀\ud800"}`,
	`{"a":` + strings.Repeat("[", 1001) + strings.Repeat("]", 1001) + `}`,
}

func FuzzScanOldNodes(f *testing.F) {
	for _, s := range scanSeeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, doc []byte) { checkScan(t, doc) })
}

// Byte edits of valid documents: the scan must refuse or agree, never differ.
func TestScanOldNodesMutations(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const alphabet = "{}[]\",:\\ \n0123456789-+.eEtrufalsn\x00\x1f\xffu"
	accepted := 0
	for _, seed := range scanSeeds[:6] {
		for range 20000 {
			doc := []byte(seed)
			for range 1 + rng.Intn(2) {
				if len(doc) == 0 {
					break
				}
				i := rng.Intn(len(doc))
				switch rng.Intn(4) {
				case 0:
					doc[i] = alphabet[rng.Intn(len(alphabet))]
				case 1:
					doc = slices.Delete(doc, i, i+1)
				case 2:
					doc = slices.Insert(doc, i, alphabet[rng.Intn(len(alphabet))])
				case 3:
					doc = doc[:i]
				}
			}
			if checkScan(t, doc) {
				accepted++
			}
		}
	}
	if accepted < 1000 {
		t.Fatalf("only %d mutants accepted: the test checks nothing", accepted)
	}
}

// Nodes past one decode batch, the later ones without a source: a batch
// must not inherit fields from the one before.
func TestScanOldNodesBatches(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"nodes":[`)
	const n = 20000
	for i := range n {
		if i > 0 {
			b.WriteString(", ")
		}
		if i < n/2 {
			fmt.Fprintf(&b, `{"id":"n%d","source_file":"dir/file%d.go","pad":[%d,{"x":"y"}]}`, i, i, i)
		} else {
			fmt.Fprintf(&b, `{"id":"n%d"}`, i)
		}
	}
	b.WriteString(`],"links":[]}`)
	if b.Len() < 3*256<<10/2 {
		t.Fatalf("document of %d bytes does not span the batches", b.Len())
	}
	doc := []byte(b.String())
	got, ok := scanOldNodes(bytes.NewReader(doc))
	want, _ := wholeOldNodes(doc)
	if !ok || !slices.Equal(got, want) || len(got) != n {
		t.Fatalf("ok=%v, %d nodes, equal=%v", ok, len(got), slices.Equal(got, want))
	}
}

// The scan must take what ToJSON writes, or the whole decode it replaces
// would quietly come back.
func TestScanOldNodesTakesToJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.json")
	g := model.New()
	g.AddNode(model.Node{ID: "a", Label: "a", FileType: "code", SourceFile: "pkg/a.go"})
	g.AddNode(model.Node{ID: "b \"q\" \u00e9<", Label: "b", FileType: "code", SourceFile: "pkg/b.go"})
	if err := ToJSON(g, map[int][]string{0: {"a"}}, path, "abc", true); err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !checkScan(t, doc) {
		t.Fatalf("scan refused ToJSON output: %s", doc)
	}
	if got, _ := scanOldNodes(bytes.NewReader(doc)); len(got) != 2 {
		t.Fatalf("got %d nodes, want 2", len(got))
	}
}
