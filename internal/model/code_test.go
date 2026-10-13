package model

import (
	"encoding/json"
	"sync"
	"testing"
	"unsafe"
)

func TestEdgeCodes(t *testing.T) {
	if n := unsafe.Sizeof(Edge{}); n != 64 {
		t.Fatalf("Edge is %d bytes, want 64", n)
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
