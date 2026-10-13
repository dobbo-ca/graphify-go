package model

import (
	"encoding/json"
	"math"
	"strconv"
)

// The Append functions write what encoding/json writes for the plain cases
// and hand everything else to it.

// AppendJSONString appends s quoted. Printable ASCII with nothing to escape
// is copied.
func AppendJSONString(b []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c >= 0x7f || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			q, _ := json.Marshal(s)
			return append(b, q...)
		}
	}
	b = append(b, '"')
	b = append(b, s...)
	return append(b, '"')
}

// AppendJSONFloat appends f; the exponent forms and the NaN and Inf errors
// are encoding/json's.
func AppendJSONFloat(b []byte, f float64) ([]byte, error) {
	if a := math.Abs(f); a == 0 || a >= 1e-6 && a < 1e21 {
		return strconv.AppendFloat(b, f, 'f', -1, 64), nil
	}
	q, err := json.Marshal(f)
	return append(b, q...), err
}
