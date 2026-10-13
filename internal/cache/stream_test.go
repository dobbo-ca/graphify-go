package cache

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/dobbo-ca/graphify-go/internal/extract"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// writeCache must produce the bytes of the whole-document marshal it replaced.
func TestWriteCacheMatchesMarshal(t *testing.T) {
	entry := func(i int) Entry {
		return Entry{Hash: fmt.Sprint("h", i), Result: extract.Result{
			Nodes: []model.Node{{ID: fmt.Sprint("n", i), Label: "<é&> ", Attributes: map[string]string{"b": "2", "a": "1"}}},
			Edges: []model.Edge{{Source: "a", Target: "b\xff", Relation: model.RelCalls, Weight: 0.5}},
			Calls: []extract.Call{{Callee: 1, Recv: 2}}, CallText: []string{"", "f", "日本"},
		}}
	}
	// Keys whose byte order differs from UTF-16 order, plus ones the encoder escapes.
	keys := []string{"a.go", "A.go", "dir/b.go", "", "<x>&.js", "q\"uote\\.py", "é.md", "￿.ts", "\U0001F600.ts", ".ts", "bad\xff.c", "tab\t.h", "a", "a/a", "a.a"}
	cases := map[string]Cache{"nil": nil, "empty": {}, "one": {"only.go": entry(0)}, "zero-entry": {"z.go": {}}}
	many := Cache{}
	for i, k := range keys {
		many[k] = entry(i)
	}
	cases["many"] = many
	for name, c := range cases {
		for _, stamp := range []string{"v0.10.1-s12", "", "<dev>\"x"} {
			want, err := json.Marshal(cacheFile{V: stamp, Files: c})
			if err != nil {
				t.Fatal(err)
			}
			var got bytes.Buffer
			if err := writeCache(&got, stamp, c, false); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Errorf("%s, stamp %q: streamed cache differs\n got: %s\nwant: %s", name, stamp, got.Bytes(), want)
			}
			// Draining writes the same bytes and empties the map.
			d := maps.Clone(c)
			got.Reset()
			if err := writeCache(&got, stamp, d, true); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), want) || len(d) != 0 {
				t.Errorf("%s, stamp %q: drained cache differs or %d entries left", name, stamp, len(d))
			}
		}
	}
}

type failWriter struct{ n int }

func (f *failWriter) Write(b []byte) (int, error) {
	if f.n--; f.n < 0 {
		return 0, fmt.Errorf("disk full")
	}
	return len(b), nil
}

// A write error part way through is returned: head, 50 entries, tail.
func TestWriteCacheWriteError(t *testing.T) {
	c := Cache{}
	for i := range 50 {
		c[fmt.Sprint("f", i, ".go")] = Entry{Hash: "h"}
	}
	for _, n := range []int{0, 1, 7, 51} {
		if err := writeCache(&failWriter{n: n}, "s", maps.Clone(c), true); err == nil {
			t.Errorf("writer failing after %d writes: no error", n)
		}
	}
}

// Load in pieces must return what one decode of the whole file returns.
func TestLoadSplitMatchesWholeDecode(t *testing.T) {
	whole := func(data []byte) Cache {
		var f cacheFile
		if err := json.Unmarshal(data, &f); err != nil {
			t.Fatal(err)
		}
		return f.Files
	}
	c := Cache{}
	for i := range 20000 {
		// Text that looks like an entry boundary, inside a key and a value.
		k := fmt.Sprint("d", i%7, `/}},"k":{"hash":"`, i, ".go")
		c[k] = Entry{Hash: fmt.Sprint("h", i), Result: extract.Result{
			Nodes: []model.Node{{ID: k, Label: `x}},"y":{"hash":"z`, Attributes: map[string]string{"a": "}}"}}},
			Calls: []extract.Call{{Callee: 1, Recv: 2}}, CallText: []string{"", "f\\", "日本"},
		}}
	}
	// Entries longer than a piece, alone and side by side.
	for _, k := range []string{"big0.go", "d3/big1.go", "d3/big2.go"} {
		c[k] = Entry{Hash: "h", Result: extract.Result{Nodes: []model.Node{{ID: k, Label: strings.Repeat(`}},"y":{"hash":"z`, 60000)}}}}
	}
	var buf bytes.Buffer
	if err := writeCache(&buf, "s1", c, false); err != nil {
		t.Fatal(err)
	}
	got := loadSplit(bytes.NewReader(buf.Bytes()), "s1")
	if got == nil {
		t.Fatal("loadSplit gave up on a file writeCache wrote")
	}
	if !reflect.DeepEqual(got, whole(buf.Bytes())) || len(got) != len(c) {
		t.Error("split decode differs from the whole decode")
	}
	if loadSplit(bytes.NewReader(buf.Bytes()), "s2") != nil {
		t.Error("loadSplit accepted another stamp")
	}

	// A file cut short, with a byte too many or a wrong last byte is not decoded in pieces.
	for _, b := range [][]byte{buf.Bytes()[:buf.Len()-1], buf.Bytes()[:buf.Len()/2], append(buf.Bytes()[:buf.Len():buf.Len()], '}'), append(buf.Bytes()[:buf.Len()-1:buf.Len()-1], 'x')} {
		if loadSplit(bytes.NewReader(b), "s1") != nil {
			t.Errorf("loadSplit accepted a damaged file of %d bytes", len(b))
		}
	}
	for _, e := range []Cache{{}, {"a.go": {Hash: "h"}}} {
		buf.Reset()
		if err := writeCache(&buf, "s1", e, false); err != nil {
			t.Fatal(err)
		}
		if got := loadSplit(iotest.OneByteReader(bytes.NewReader(buf.Bytes())), "s1"); got == nil || !reflect.DeepEqual(got, whole(buf.Bytes())) {
			t.Errorf("split decode of %d entries differs from the whole decode", len(e))
		}
	}

	// A key written twice, pieces apart: the later entry wins, as in a whole decode.
	var dup strings.Builder
	dup.WriteString(`{"v":"s1","files":{"a":{"hash":"1","result":{}}`)
	for i := range 20000 {
		fmt.Fprintf(&dup, `,"f%d":{"hash":"h%d","result":{}}`, i, i)
	}
	dup.WriteString(`,"a":{"hash":"2","result":{}}}}`)
	if got := loadSplit(strings.NewReader(dup.String()), "s1"); got["a"].Hash != "2" || !reflect.DeepEqual(got, whole([]byte(dup.String()))) {
		t.Error("split decode of a repeated key differs from the whole decode")
	}

	// A hand-made file whose every likely cut is inside an entry: the pieces
	// fail, and Load falls back to the whole decode.
	var sb strings.Builder
	sb.WriteString(`{"v":"s1","files":{`)
	for i := range 20000 {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `"f%d":{"hash":"h%d","extra":{"a":{"b":{}},"c":{"hash":"x"}},"result":{"GoMods":{"m":"%d"}}}`, i, i, i)
	}
	sb.WriteString("}}")
	bad := []byte(sb.String())
	if loadSplit(bytes.NewReader(bad), "s1") != nil {
		t.Error("loadSplit decoded pieces cut inside an entry")
	}
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := Load(path, "s1"), whole(bad); len(got) != 20000 || !reflect.DeepEqual(got, want) {
		t.Error("Load after a failed split differs from the whole decode")
	}
}

// An entry whose calls number a text it does not hold is not loaded, by
// either decoder, so the file is parsed again.
func TestLoadDropsBadCalls(t *testing.T) {
	c := Cache{
		"ok.go":  {Hash: "h", Result: extract.Result{Calls: []extract.Call{{Callee: 1}}, CallText: []string{"", "f"}}},
		"bad.go": {Hash: "h", Result: extract.Result{Calls: []extract.Call{{Callee: 2}}, CallText: []string{"", "f"}}},
	}
	path := filepath.Join(t.TempDir(), FileName)
	if err := Save(path, "s", c); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, "s"); len(got) != 1 || got["ok.go"].Hash != "h" {
		t.Errorf("split load = %+v, want ok.go only", got)
	}
	// Not in writeCache's layout: the whole-file decoder.
	data, _ := json.MarshalIndent(cacheFile{V: "s", Files: c}, "", " ")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, "s"); len(got) != 1 || got["ok.go"].Hash != "h" {
		t.Errorf("whole load = %+v, want ok.go only", got)
	}
}
