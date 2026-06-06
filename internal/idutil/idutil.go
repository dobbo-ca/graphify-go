// Package idutil builds stable node IDs from name parts. It mirrors the Python
// original's extract._make_id and build._normalize_id so IDs generated here are
// byte-for-byte compatible with upstream graph.json files: NFKC-normalize,
// replace every non-word (Unicode) run with "_", collapse repeats, strip, fold.
package idutil

import (
	"regexp"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// nonWord matches any run of characters that are not Unicode letters, digits, or
// underscore — the equivalent of Python's re.sub(r"[^\w]+", ...) under re.UNICODE.
var nonWord = regexp.MustCompile(`[^\p{L}\p{N}_]+`)

// underscores collapses runs of "_" left behind once non-word chars are replaced
// or carried in from the source text (Python's second re.sub(r"_+", "_", ...)).
var underscores = regexp.MustCompile(`_+`)

var fold = cases.Fold()

// MakeID joins one or more name parts into a canonical node ID. Empty parts are
// skipped; each part is stripped of leading/trailing "_" and "." before joining.
func MakeID(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if p = strings.Trim(p, "_."); p != "" {
			kept = append(kept, p)
		}
	}
	return clean(strings.Join(kept, "_"))
}

// NormalizeID canonicalizes an already-joined ID the same way MakeID does. Used
// to reconcile edge endpoints whose IDs differ only in casing or punctuation.
func NormalizeID(s string) string { return clean(s) }

func clean(s string) string {
	s = norm.NFKC.String(s)
	s = nonWord.ReplaceAllString(s, "_")
	s = underscores.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	return fold.String(s)
}
