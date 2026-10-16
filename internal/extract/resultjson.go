package extract

import (
	"encoding/json"
	"strconv"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// AppendJSON appends the bytes json.Marshal(r) produces. The cache encode
// was the largest CPU user outside the parser, half of it garbage; the common
// records are appended directly and the rare ones go through encoding/json.
// TestResultAppendJSONMatchesMarshal fails when a field is added and not
// encoded here.
func (r *Result) AppendJSON(b []byte) ([]byte, error) {
	var err error
	b = append(b, `{"Nodes":`...)
	if b, err = appendJSONArr(b, r.Nodes, appendJSONNode); err != nil {
		return b, err
	}
	b = append(b, `,"Edges":`...)
	if b, err = appendJSONArr(b, r.Edges, appendJSONEdge); err != nil {
		return b, err
	}
	b = append(b, `,"Defs":`...)
	b, _ = appendJSONArr(b, r.Defs, func(b []byte, d *Def) ([]byte, error) {
		b = appendJSONFields(b, `{"ID":`, d.ID, `,"Name":`, d.Name, `,"File":`, d.File, `,"Owner":`, d.Owner)
		b = appendJSONBool(append(b, `,"Abstract":`...), d.Abstract, ',')
		b = appendJSONBool(append(b, `"Value":`...), d.Value, ',')
		return appendJSONBool(append(b, `"Export":`...), d.Export, '}'), nil
	})
	b = append(b, `,"Calls":`...)
	b, _ = appendJSONArr(b, r.Calls, func(b []byte, c *Call) ([]byte, error) {
		b = strconv.AppendUint(append(b, `{"CallerID":`...), uint64(c.CallerID), 10)
		b = strconv.AppendUint(append(b, `,"Callee":`...), uint64(c.Callee), 10)
		b = c.Loc.AppendJSON(append(b, `,"Loc":`...))
		b = strconv.AppendUint(append(b, `,"Recv":`...), uint64(c.Recv), 10)
		return append(b, '}'), nil
	})
	b = append(b, `,"CallText":`...)
	b, _ = appendJSONArr(b, r.CallText, func(b []byte, s *string) ([]byte, error) {
		return model.AppendJSONString(b, *s), nil
	})
	b = appendJSONFields(b, `,"File":`, r.File)
	b = append(b, `,"CallMods":`...)
	b, _ = appendJSONArr(b, r.CallMods, func(b []byte, s *string) ([]byte, error) {
		return model.AppendJSONString(b, *s), nil
	})
	b = append(b, `,"TypeRefs":`...)
	b, _ = appendJSONArr(b, r.TypeRefs, func(b []byte, t *TypeRef) ([]byte, error) {
		b = appendJSONFields(b, `{"FromID":`, t.FromID, `,"Name":`, t.Name, `,"Relation":`, t.Relation, `,"File":`, t.File)
		b = t.Loc.AppendJSON(append(b, `,"Loc":`...))
		if t.Stub {
			b = append(b, `,"Stub":true`...)
		}
		if t.Deco {
			b = append(b, `,"Deco":true`...)
		}
		if t.Qual != "" {
			b = appendJSONFields(b, `,"Qual":`, t.Qual)
		}
		return append(b, '}'), nil
	})
	b = append(b, `,"Imps":`...)
	b, _ = appendJSONArr(b, r.Imps, func(b []byte, i *Imp) ([]byte, error) {
		b = appendJSONFields(b, `{"FileID":`, i.FileID, `,"File":`, i.File, `,"Spec":`, i.Spec)
		b = i.Loc.AppendJSON(append(b, `,"Loc":`...))
		b = appendJSONBool(append(b, `,"TypeOnly":`...), i.TypeOnly, ',')
		b = appendJSONBool(append(b, `"Rel":`...), i.Rel, ',')
		if i.Export {
			b = append(b, `"Export":true,`...)
		}
		b = append(b, `"Names":`...)
		b, _ = appendJSONArr(b, i.Names, func(b []byte, s *string) ([]byte, error) {
			return model.AppendJSONString(b, *s), nil
		})
		return append(b, '}'), nil
	})
	b = append(b, `,"ModRefs":`...)
	if b, err = appendJSONAny(b, r.ModRefs == nil, r.ModRefs); err != nil {
		return b, err
	}
	b = append(b, `,"NullLabels":`...)
	if b, err = appendJSONAny(b, r.NullLabels == nil, r.NullLabels); err != nil {
		return b, err
	}
	b = append(b, `,"ModInvokes":`...)
	if b, err = appendJSONAny(b, r.ModInvokes == nil, r.ModInvokes); err != nil {
		return b, err
	}
	b = append(b, `,"ImportAliases":`...)
	b, _ = appendJSONArr(b, r.ImportAliases, func(b []byte, a *ImportAlias) ([]byte, error) {
		b = appendJSONFields(b, `{"Local":`, a.Local, `,"Imported":`, a.Imported, `,"ModuleStem":`, a.ModuleStem, `,"Spec":`, a.Spec)
		b = a.Loc.AppendJSON(append(b, `,"Loc":`...))
		if a.InFunc {
			b = append(b, `,"InFunc":true`...)
		}
		return append(b, '}'), nil
	})
	b = append(b, `,"MDRefs":`...)
	b, _ = appendJSONArr(b, r.MDRefs, func(b []byte, m *MDRef) ([]byte, error) {
		b = appendJSONFields(b, `{"FromID":`, m.FromID, `,"Target":`, m.Target, `,"File":`, m.File)
		return append(m.Loc.AppendJSON(append(b, `,"Loc":`...)), '}'), nil
	})
	b = append(b, `,"GoMods":`...)
	if b, err = appendJSONAny(b, r.GoMods == nil, r.GoMods); err != nil {
		return b, err
	}
	if r.Default != "" {
		b = appendJSONFields(b, `,"Default":`, r.Default)
	}
	if len(r.Exports) > 0 {
		b = append(b, `,"Exports":`...)
		b, _ = appendJSONArr(b, r.Exports, func(b []byte, s *string) ([]byte, error) {
			return model.AppendJSONString(b, *s), nil
		})
	}
	if len(r.TypeAliases) > 0 {
		b = append(b, `,"TypeAliases":`...)
		b, _ = appendJSONArr(b, r.TypeAliases, func(b []byte, s *string) ([]byte, error) {
			return model.AppendJSONString(b, *s), nil
		})
	}
	if r.OverBudget {
		b = append(b, `,"OverBudget":true`...)
	}
	return append(b, '}'), nil
}

// appendJSONArr appends s as an array, null when nil as encoding/json does.
func appendJSONArr[T any](b []byte, s []T, elem func([]byte, *T) ([]byte, error)) ([]byte, error) {
	if s == nil {
		return append(b, "null"...), nil
	}
	b = append(b, '[')
	var err error
	for i := range s {
		if i > 0 {
			b = append(b, ',')
		}
		if b, err = elem(b, &s[i]); err != nil {
			return b, err
		}
	}
	return append(b, ']'), nil
}

// appendJSONAny is the fallback for the rare fields.
func appendJSONAny(b []byte, isNil bool, v any) ([]byte, error) {
	if isNil {
		return append(b, "null"...), nil
	}
	q, err := json.Marshal(v)
	return append(b, q...), err
}

// appendJSONFields appends key, string value pairs; a key carries its
// punctuation.
func appendJSONFields(b []byte, kv ...string) []byte {
	for i := 0; i < len(kv); i += 2 {
		b = append(b, kv[i]...)
		b = model.AppendJSONString(b, kv[i+1])
	}
	return b
}

func appendJSONBool(b []byte, v bool, end byte) []byte {
	if v {
		b = append(b, "true"...)
	} else {
		b = append(b, "false"...)
	}
	return append(b, end)
}

func appendJSONNode(b []byte, n *model.Node) ([]byte, error) {
	b = appendJSONFields(b, `{"id":`, n.ID, `,"label":`, n.Label, `,"file_type":`, n.FileType, `,"source_file":`, n.SourceFile)
	if n.SourceLocation != "" {
		b = appendJSONFields(b, `,"source_location":`, n.SourceLocation)
	}
	if n.ComputedName != "" {
		b = appendJSONFields(b, `,"computed_name":`, n.ComputedName)
	}
	if len(n.Attributes) > 0 {
		q, err := json.Marshal(n.Attributes)
		if err != nil {
			return b, err
		}
		b = append(append(b, `,"attributes":`...), q...)
	}
	return append(b, '}'), nil
}

func appendJSONEdge(b []byte, e *model.Edge) ([]byte, error) {
	b = appendJSONFields(b, `{"source":`, e.Source, `,"target":`, e.Target, `,"relation":`, e.Relation.String(), `,"confidence":`, e.Confidence.String())
	if e.TypeOnly() {
		b = append(b, `,"type_only":true`...)
	}
	if e.SourceFile != 0 {
		b = appendJSONFields(b, `,"source_file":`, e.SourceFile.String())
	}
	if l := e.Loc(); l != 0 {
		b = l.AppendJSON(append(b, `,"source_location":`...))
	}
	var err error
	if e.Weight != 0 {
		if b, err = model.AppendJSONFloat(append(b, `,"weight":`...), e.Weight); err != nil {
			return b, err
		}
	}
	if e.ConfidenceScore != 0 {
		if b, err = model.AppendJSONFloat(append(b, `,"confidence_score":`...), e.ConfidenceScore); err != nil {
			return b, err
		}
	}
	return append(b, '}'), nil
}
