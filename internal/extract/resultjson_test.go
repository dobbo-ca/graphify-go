package extract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// fillJSON sets every exported field under v to a non-zero value, so a field
// AppendJSON does not know shows up as a difference from json.Marshal.
func fillJSON(v reflect.Value, n *int) {
	*n++
	switch v.Kind() {
	case reflect.String:
		v.SetString(fmt.Sprint("s", *n))
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Float64:
		v.SetFloat(float64(*n) / 4)
	case reflect.Int, reflect.Int64:
		v.SetInt(int64(*n))
	case reflect.Uint8, reflect.Uint16, reflect.Uint32: // an edge code that both tables hold
		v.SetUint(uint64(1 + *n%3))
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 2, 2))
		for i := range 2 {
			fillJSON(v.Index(i), n)
		}
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
		for range 2 {
			k, e := reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem()
			fillJSON(k, n)
			fillJSON(e, n)
			v.SetMapIndex(k, e)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				fillJSON(v.Field(i), n)
			}
		}
	default:
		panic("fillJSON: add kind " + v.Kind().String())
	}
}

func TestResultAppendJSONMatchesMarshal(t *testing.T) {
	for _, f := range []string{"a.go", "b.go", "c.go"} { // codes 1 to 3 must exist
		model.FileOf(f)
	}
	var full Result
	n := 0
	fillJSON(reflect.ValueOf(&full).Elem(), &n)
	odd := "<é&> \"q\\\x00\x7f\xff\u2028日本"
	typeOnly, typeOnlyAt := model.Edge{Weight: 3}, model.Edge{SourceLocation: model.Line(7)}
	typeOnly.SetTypeOnly(true)
	typeOnlyAt.SetTypeOnly(true)
	deferred := model.Edge{SourceLocation: model.Line(4)}
	deferred.SetDeferred(true)
	cases := map[string]Result{
		"zero": {},
		"full": full,
		"empty slices": {Nodes: []model.Node{}, Edges: []model.Edge{}, Defs: []Def{}, Calls: []Call{}, CallText: []string{}, TypeRefs: []TypeRef{},
			Imps: []Imp{{Names: []string{}}}, ModRefs: []ModRef{}, NullLabels: []NullLabelRef{}, ModInvokes: []ModInvoke{},
			ImportAliases: []ImportAlias{}, MDRefs: []MDRef{}, GoMods: map[string]string{}},
		"omitted and escaped": {
			Nodes: []model.Node{{ID: odd, Label: odd}, {Attributes: map[string]string{}}, {Attributes: map[string]string{"b": odd, "a": "1"}}},
			Edges: []model.Edge{{Source: odd}, {Weight: 1e-7, ConfidenceScore: 1e21}, {Weight: math.Copysign(0, -1), ConfidenceScore: -2.5}, typeOnly, typeOnlyAt, deferred},
			Defs:  []Def{{ID: odd, Abstract: true, Value: true}, {}},
			Calls: []Call{{Callee: 1}}, CallText: []string{"", odd},
			Imps: []Imp{{Spec: odd, Names: []string{odd, ""}}, {Rel: true}, {TypeOnly: true}, {Dyn: 2}},
		},
	}
	for name, r := range cases {
		want, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.AppendJSON([]byte("x"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got[1:], want) {
			t.Errorf("%s:\n got: %s\nwant: %s", name, got[1:], want)
		}
	}
	r := Result{Edges: []model.Edge{typeOnly, typeOnlyAt}}
	if got, _ := r.AppendJSON(nil); !bytes.Contains(got, []byte(`"type_only":true,"weight":3}`)) ||
		!bytes.Contains(got, []byte(`"type_only":true,"source_location":"L7"}`)) {
		t.Errorf("type-only edges: %s", got)
	}
	// An unencodable float is an error, as it is for json.Marshal.
	for _, e := range []model.Edge{{Weight: math.NaN()}, {ConfidenceScore: math.Inf(1)}} {
		r := Result{Edges: []model.Edge{e}}
		if _, err := r.AppendJSON(nil); err == nil {
			t.Errorf("%+v: no error", e)
		}
	}
}
