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
	texts := []string{"", "L1", "L0", "L42", "L1073741821", "L1073741822", "L1073741823", "L2147483646", "L2147483647", "L99999999999", "L01", "L-1", "L+1", "L", "l5", "L5 ", "12", "L1é", "a\x00\"<b>"}
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
	if LocOf("L7") >= locTypeOnly || LocOf("L1073741822") >= locTypeOnly || LocOf("L1073741823") < locText || LocOf("L01") < locText {
		t.Fatal("a line is not held as a number, or a text is")
	}
	// Around the last line held as a number, with the type-only bit and without.
	for _, n := range []int{0, 1, 1<<30 - 3, 1<<30 - 2, 1<<30 - 1, 1<<31 - 2} {
		s := "L" + strconv.Itoa(n)
		l := Line(n)
		want, _ := json.Marshal(s)
		e := Edge{SourceLocation: l}
		e.SetTypeOnly(true)
		if l != LocOf(s) || l&locTypeOnly != 0 || (l >= locText) != (n > 1<<30-2) || e.Loc() != l || e.SourceLocation == l {
			t.Errorf("line %d: Loc %#x, with the bit %#x", n, l, e.SourceLocation)
		}
		for _, l := range []Loc{l, e.SourceLocation} {
			text, _ := l.MarshalText()
			if l.String() != s || string(text) != s || string(l.AppendJSON(nil)) != string(want) {
				t.Errorf("line %d: %#x is %q, %s", n, l, l.String(), l.AppendJSON(nil))
			}
		}
		e.SetTypeOnly(false)
		if e.SourceLocation != l || e.TypeOnly() {
			t.Errorf("line %d: %#x after the bit is cleared", n, e.SourceLocation)
		}
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
	if err := json.Unmarshal([]byte(in), &e); err != nil || !e.TypeOnly() || e.Confidence != ConfExtracted || e.Relation != RelImportsFrom || e.Loc() != Line(3) {
		t.Fatalf("unmarshal: %v %+v", err, e)
	}
	if got, _ := json.Marshal(e); string(got) != in {
		t.Fatalf("got %s want %s", got, in)
	}
	e.SetTypeOnly(false)
	if got, _ := json.Marshal(e); strings.Contains(string(got), "type_only") {
		t.Fatalf("type_only written when false: %s", got)
	}

	// The JSON of an edge is the JSON of the same edge with plain fields.
	type plain struct {
		Source          string  `json:"source"`
		Target          string  `json:"target"`
		Relation        string  `json:"relation"`
		Confidence      string  `json:"confidence"`
		TypeOnly        bool    `json:"type_only,omitempty"`
		SourceFile      string  `json:"source_file,omitempty"`
		SourceLocation  string  `json:"source_location,omitempty"`
		Weight          float64 `json:"weight,omitempty"`
		ConfidenceScore float64 `json:"confidence_score,omitempty"`
	}
	for _, typeOnly := range []bool{false, true} {
		for _, loc := range []string{"", "L0", "L12", "L1073741823", "sec <&> 2"} {
			for _, p := range []plain{
				{"a", "b", "imports_from", "EXTRACTED", typeOnly, "", loc, 0, 0},
				{"a<", "b\"", "rel é", "OTHER", typeOnly, "d/<é>.ts", loc, 1.5, -1e-7},
			} {
				want, _ := json.Marshal(p)
				var e Edge
				if err := json.Unmarshal(want, &e); err != nil {
					t.Fatal(err)
				}
				if e.TypeOnly() != typeOnly || e.Loc() != LocOf(loc) || e.SourceLocation.String() != loc || e.SourceFile.String() != p.SourceFile ||
					e.Source != p.Source || e.Target != p.Target || e.Relation.String() != p.Relation || e.Confidence.String() != p.Confidence ||
					e.Weight != p.Weight || e.ConfidenceScore != p.ConfidenceScore {
					t.Fatalf("%s decodes to %+v", want, e)
				}
				// Decoded on the heap too: behind a pointer and as a map value.
				var ptr *Edge
				var byKey map[string]Edge
				if json.Unmarshal(want, &ptr) != nil || *ptr != e || json.Unmarshal([]byte(`{"k":`+string(want)+`}`), &byKey) != nil || byKey["k"] != e {
					t.Fatalf("%s decodes to %+v and %+v", want, ptr, byKey)
				}
				// By value, by pointer, in a slice and indented.
				got, _ := json.Marshal(e)
				gotP, _ := json.Marshal(&e)
				gotS, _ := json.Marshal([]Edge{e})
				gotI, _ := json.MarshalIndent(e, "", "  ")
				wantI, _ := json.MarshalIndent(p, "", "  ")
				if string(got) != string(want) || string(gotP) != string(want) || string(gotS) != "["+string(want)+"]" || string(gotI) != string(wantI) {
					t.Fatalf("got %s %s %s %s want %s", got, gotP, gotS, gotI, want)
				}
				if loc == "" && strings.Contains(string(got), "source_location") {
					t.Fatalf("source_location written for no location: %s", got)
				}
				// Two edges that differ only in type_only are different values.
				other := e
				other.SetTypeOnly(!typeOnly)
				if other == e || other.Loc() != e.Loc() || other.SourceLocation.String() != e.SourceLocation.String() {
					t.Fatalf("%+v and %+v", e, other)
				}
			}
		}
	}

	// As a struct with a bool field decodes: any key case, last key wins, and
	// an absent key or a null leaves the value alone.
	e = Edge{}
	for _, c := range []struct {
		in   string
		want bool
	}{
		{`{"TYPE_ONLY":true}`, true},
		{`{"source_location":"L4"}`, true},
		{`null`, true},
		{`{"type_only":null}`, true},
		{`{"type_only":true,"source_location":"x","type_only":false}`, false},
		{`{"type_only":false,"type\u005fonly":true,"source_location":""}`, true},
	} {
		if err := json.Unmarshal([]byte(c.in), &e); err != nil || e.TypeOnly() != c.want {
			t.Fatalf("%s: %v, type_only %v", c.in, err, e.TypeOnly())
		}
	}
	if e.Loc() != 0 || e.SourceLocation == 0 {
		t.Fatalf("location %#x", e.SourceLocation)
	}
	if err := json.Unmarshal([]byte(`{"type_only":1}`), &e); err == nil {
		t.Fatal("no error for a number")
	}
}
