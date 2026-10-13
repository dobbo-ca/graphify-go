package model

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

func TestEdgeCodes(t *testing.T) {
	if n := unsafe.Sizeof(Edge{}); n != 72 {
		t.Fatalf("Edge is %d bytes, want 72", n)
	}
	if RelImplements.String() != "implements" || ConfAmbiguous.String() != "AMBIGUOUS" || Rel("calls") != RelCalls || Conf("") != 0 {
		t.Fatal("constants and tables disagree")
	}
	// The JSON of an edge is the JSON of the same edge with string fields.
	type plain struct {
		Source     string `json:"source"`
		Target     string `json:"target"`
		Relation   string `json:"relation"`
		Confidence string `json:"confidence"`
	}
	const rel = "rel<é>\x00\"x"
	got, _ := json.Marshal(Edge{Source: "a", Target: "b", Relation: Rel(rel), Confidence: Conf("OTHER")})
	want, _ := json.Marshal(plain{"a", "b", rel, "OTHER"})
	if string(got) != string(want) {
		t.Fatalf("got %s want %s", got, want)
	}
	var back Edge
	if err := json.Unmarshal(got, &back); err != nil || back.Relation.String() != rel || back.Confidence.String() != "OTHER" {
		t.Fatalf("round trip: %v %+v", err, back)
	}
	// New strings from many goroutines get one code each.
	var wg sync.WaitGroup
	codes := make([][]Relation, 8)
	for g := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				s := "t" + string(rune('a'+i%26)) + string(rune('a'+i/26))
				r := Rel(s)
				if r.String() != s {
					t.Errorf("code %d is %q, want %q", r, r.String(), s)
				}
				codes[g] = append(codes[g], r)
			}
		}()
	}
	wg.Wait()
	for g := range codes {
		for i, r := range codes[g] {
			if r != codes[0][i] {
				t.Fatalf("goroutine %d got another code for string %d", g, i)
			}
		}
	}
}

func TestLocAndFile(t *testing.T) {
	// Every text comes back as it went in, and equal texts have equal codes.
	texts := []string{"", "L1", "L0", "L42", "L2147483646", "L2147483647", "L99999999999", "L01", "L-1", "L+1", "L", "l5", "L5 ", "12", "L1é", "a\x00\"<b>"}
	seen := map[Loc]string{}
	for _, s := range texts {
		l := LocOf(s)
		if l.String() != s || LocOf(s) != l {
			t.Errorf("LocOf(%q) = %d, String %q", s, l, l.String())
		}
		if prev, ok := seen[l]; ok {
			t.Errorf("%q and %q share code %d", prev, s, l)
		}
		seen[l] = s
		want, _ := json.Marshal(s)
		if got := l.AppendJSON(nil); string(got) != string(want) {
			t.Errorf("AppendJSON(%q) = %s, want %s", s, got, want)
		}
	}
	if LocOf("") != 0 || FileOf("") != 0 || Line(7) != LocOf("L7") || Line(-3).String() != "L-3" || Line(1<<31).String() != "L2147483648" {
		t.Fatal("zero values or Line disagree with LocOf")
	}
	if LocOf("L7") >= locText || LocOf("L2147483646") >= locText || LocOf("L01") < locText {
		t.Fatal("a line is not held as a number, or a text is")
	}
	// The JSON of an edge is the JSON of the same edge with string fields.
	type plain struct {
		Source         string `json:"source"`
		Target         string `json:"target"`
		Relation       string `json:"relation"`
		Confidence     string `json:"confidence"`
		SourceFile     string `json:"source_file,omitempty"`
		SourceLocation string `json:"source_location,omitempty"`
	}
	for _, p := range []plain{{"a", "b", "calls", "EXTRACTED", "", ""}, {"a", "b", "calls", "EXTRACTED", "d/<é>.go", "L12"}, {"a", "b", "calls", "EXTRACTED", "x.md", "sec & 2"}} {
		want, _ := json.Marshal(p)
		var e Edge
		if err := json.Unmarshal(want, &e); err != nil {
			t.Fatal(err)
		}
		got, _ := json.Marshal(e)
		if string(got) != string(want) || e.SourceFile.String() != p.SourceFile || e.SourceLocation.String() != p.SourceLocation {
			t.Fatalf("got %s want %s", got, want)
		}
	}
}

// v0.10.1 held confidence as a string, so any number of distinct values loads.
func TestManyConfidences(t *testing.T) {
	const n = 70000
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"source":"a","target":"b","relation":"calls","confidence":"many` + strconv.Itoa(i) + `"}`)
	}
	sb.WriteByte(']')
	var es []Edge
	if err := json.Unmarshal([]byte(sb.String()), &es); err != nil || len(es) != n {
		t.Fatalf("unmarshal: %v, %d edges", err, len(es))
	}
	for _, i := range []int{0, 65534, 65535, n - 1} {
		want := "many" + strconv.Itoa(i)
		if got := es[i].Confidence.String(); got != want || Conf(want) != es[i].Confidence {
			t.Errorf("edge %d: confidence %q, want %q", i, got, want)
		}
	}
	got, _ := json.Marshal(es)
	if string(got) != sb.String() {
		t.Error("the edges do not marshal back to their input")
	}
}

func TestTypeOnlyJSON(t *testing.T) {
	const in = `{"source":"a","target":"b","relation":"imports_from","confidence":"EXTRACTED","type_only":true,"source_file":"a.ts","source_location":"L3"}`
	var e Edge
	if err := json.Unmarshal([]byte(in), &e); err != nil || !e.TypeOnly || e.Confidence != ConfExtracted || e.Relation != RelImportsFrom {
		t.Fatalf("unmarshal: %v %+v", err, e)
	}
	if got, _ := json.Marshal(e); string(got) != in {
		t.Fatalf("got %s want %s", got, in)
	}
	e.TypeOnly = false
	if got, _ := json.Marshal(e); strings.Contains(string(got), "type_only") {
		t.Fatalf("type_only written when false: %s", got)
	}
}
