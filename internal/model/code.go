package model

import (
	"strconv"
	"sync"
	"sync/atomic"
)

// Relation and Confidence are codes for strings kept in two process-wide
// tables: together they take 8 bytes of an Edge, not 32. Any string is a
// valid value; Rel and Conf give its code, String the text.
type (
	Relation   uint32
	Confidence uint32
)

// The values the pipeline itself writes or tests. The order is the order of
// relNames and confNames; code 0 is the empty string.
const (
	RelContains Relation = iota + 1
	RelCalls
	RelImports
	RelImportsFrom
	RelReferences
	RelInherits
	RelDependsOn
	RelCrateDependsOn
	RelCaseOf
	RelDispatchesTo
	RelRationaleFor
	RelCites
	RelImplements
	RelReExports
)

const (
	ConfExtracted Confidence = iota + 1
	ConfInferred
	ConfAmbiguous
)

var (
	relTable = newCodeTable("", "contains", "calls", "imports", "imports_from", "references", "inherits",
		"depends_on", "crate_depends_on", "case_of", "dispatches_to", "rationale_for", "cites", "implements", "re_exports")
	confTable = newCodeTable("", "EXTRACTED", "INFERRED", "AMBIGUOUS")
)

// codeTable gives each string a number. Lookups by number take no lock.
// No cap: a code wraps past 2^30 strings (Loc), a table over 20 GB.
type codeTable struct {
	mu    sync.RWMutex
	ids   map[string]uint32
	names atomic.Pointer[[]string] // replaced, never changed in place
}

func newCodeTable(names ...string) *codeTable {
	t := &codeTable{ids: make(map[string]uint32, len(names))}
	for i, s := range names {
		t.ids[s] = uint32(i)
	}
	t.names.Store(&names)
	return t
}

func (t *codeTable) code(s string) uint32 {
	t.mu.RLock()
	id, ok := t.ids[s]
	t.mu.RUnlock()
	if ok {
		return id
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if id, ok := t.ids[s]; ok {
		return id
	}
	old := *t.names.Load()
	id = uint32(len(old))
	// append copies only when it grows, and readers never look past their own length
	names := append(old, s)
	t.ids[s] = id
	t.names.Store(&names)
	return id
}

func (t *codeTable) name(id uint32) string { return (*t.names.Load())[id] }

// Rel returns the code of a relation.
func Rel(s string) Relation { return Relation(relTable.code(s)) }

// Conf returns the code of a confidence.
func Conf(s string) Confidence { return Confidence(confTable.code(s)) }

func (r Relation) String() string   { return relTable.name(uint32(r)) }
func (c Confidence) String() string { return confTable.name(uint32(c)) }

// As text, so encoding/json reads and writes them as the strings they stand for.
func (r Relation) MarshalText() ([]byte, error)   { return []byte(r.String()), nil }
func (c Confidence) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

func (r *Relation) UnmarshalText(b []byte) error {
	*r = Rel(string(b))
	return nil
}

func (c *Confidence) UnmarshalText(b []byte) error {
	*c = Conf(string(b))
	return nil
}

// Loc is a source location. A line "L<n>" is held as the number n+1 and any
// other text as locText plus a code into a process-wide table; 0 is none.
// It takes 4 bytes of an Edge or a call record, not 16 and a string.
// Bit 30 is an Edge's type-only flag, which the methods ignore.
type Loc uint32

const (
	locText     = 1 << 31
	locTypeOnly = 1 << 30
)

var locTable = newCodeTable("")

// Line returns the location of line n.
func Line(n int) Loc {
	if n < 0 || n > locTypeOnly-2 {
		return LocOf("L" + strconv.Itoa(n))
	}
	return Loc(n + 1)
}

// LocOf returns the location s names.
func LocOf(s string) Loc {
	if s == "" {
		return 0
	}
	// only the spelling String gives back: no sign, no leading zero
	if len(s) >= 2 && len(s) <= 11 && s[0] == 'L' && (s[1] != '0' || len(s) == 2) {
		n := 0
		for i := 1; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				n = -1
				break
			}
			n = n*10 + int(s[i]-'0')
		}
		if n >= 0 && n <= locTypeOnly-2 {
			return Loc(n + 1)
		}
	}
	return locText | Loc(locTable.code(s))&(locTypeOnly-1)
}

func (l Loc) String() string {
	l &^= locTypeOnly
	if l == 0 || l >= locText {
		return locTable.name(uint32(l &^ locText))
	}
	return "L" + strconv.Itoa(int(l-1))
}

// AppendJSON appends l as a JSON string.
func (l Loc) AppendJSON(b []byte) []byte {
	l &^= locTypeOnly
	if l == 0 || l >= locText {
		return AppendJSONString(b, l.String())
	}
	b = append(b, '"', 'L')
	b = strconv.AppendInt(b, int64(l-1), 10)
	return append(b, '"')
}

func (l Loc) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

// Keeps bit 30: "type_only" may come first.
func (l *Loc) UnmarshalText(b []byte) error {
	*l = *l&locTypeOnly | LocOf(string(b))
	return nil
}

// File is the path of a source file as a code into a process-wide table: 4
// bytes of an Edge, not 16. 0 is none.
type File uint32

var fileTable = newCodeTable("")

// FileOf returns the code of a path.
func FileOf(s string) File { return File(fileTable.code(s)) }

func (f File) String() string               { return fileTable.name(uint32(f)) }
func (f File) MarshalText() ([]byte, error) { return []byte(f.String()), nil }

func (f *File) UnmarshalText(b []byte) error {
	*f = FileOf(string(b))
	return nil
}
