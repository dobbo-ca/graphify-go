package export

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// oldNode is what the anti-shrink guard reads of a node on disk. An alias:
// a decode error prints the type, and v0.10.1 had it unnamed.
type oldNode = struct {
	ID         string `json:"id"`
	SourceFile string `json:"source_file"`
}

// oldNodes returns the nodes of the graph.json at path: nil for an absent,
// unreadable or empty file, an error for one that does not parse.
func oldNodes(path string) ([]oldNode, error) {
	if f, err := os.Open(path); err == nil {
		nodes, ok := scanOldNodes(f)
		f.Close()
		if ok {
			return nodes, nil
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var existing struct {
		Nodes []oldNode `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &existing); err != nil {
		return nil, err
	}
	return existing.Nodes, nil
}

// scanOldNodes reads the nodes of a graph.json without holding the file in
// memory: on a large repo that buffer was the peak of an update. It takes
// the plain case only, an object with one "nodes" array, and reports false
// for everything else, a syntax error included; the caller then decodes the
// file whole. So it may refuse a document json.Unmarshal accepts, but what
// it accepts must decode to the same nodes.
func scanOldNodes(r io.Reader) ([]oldNode, bool) {
	s := &nodeScan{r: r, buf: make([]byte, 256<<10), from: -1}
	if s.space() != '{' {
		return nil, false
	}
	var nodes []oldNode
	seen := false
	c := s.space()
	for c != '}' {
		if c != '"' {
			return nil, false
		}
		key, ok := s.key()
		if !ok || s.space() != ':' {
			return nil, false
		}
		c = s.space()
		switch {
		case key == "nodes":
			if seen || c != '[' {
				return nil, false
			}
			seen = true
			if nodes, ok = s.nodes(); !ok {
				return nil, false
			}
		case strings.EqualFold(key, "nodes"):
			// The decoder matches names without case.
			return nil, false
		case !s.value(c):
			return nil, false
		}
		if c = s.space(); c == ',' {
			if c = s.space(); c == '}' {
				return nil, false
			}
		} else if c != '}' {
			return nil, false
		}
	}
	if s.space() != 0 || !s.eof {
		return nil, false
	}
	return nodes, true
}

// nodeScan walks JSON text from r. A zero byte stands for the end of the
// input; no caller takes one as valid, so a literal NUL fails the same way.
type nodeScan struct {
	r    io.Reader
	buf  []byte
	i, n int
	eof  bool
	// While from >= 0 the bytes read since buf[from] are being copied to keep.
	keep []byte
	from int
}

func (s *nodeScan) fill() bool {
	if s.eof {
		return false
	}
	if s.from >= 0 {
		s.keep = append(s.keep, s.buf[s.from:s.i]...)
		s.from = 0
	}
	for {
		n, err := s.r.Read(s.buf)
		if n > 0 {
			s.i, s.n = 0, n
			return true
		}
		if err != nil {
			s.eof = err == io.EOF
			s.i, s.n = 0, 0
			return false
		}
	}
}

func (s *nodeScan) next() byte {
	if s.i == s.n && !s.fill() {
		return 0
	}
	c := s.buf[s.i]
	s.i++
	return c
}

// space returns the next byte that is not white space.
func (s *nodeScan) space() byte {
	for {
		// Indentation comes in runs.
		for s.i < s.n && s.buf[s.i] == ' ' {
			s.i++
		}
		if c := s.next(); c != ' ' && c != '\n' && c != '\t' && c != '\r' {
			return c
		}
	}
}

// key reads a top-level member name after its opening quote. Names with an
// escape or a byte outside ASCII are refused: none of ours has one.
func (s *nodeScan) key() (string, bool) {
	var k []byte
	for {
		c := s.next()
		if c == '"' {
			return string(k), true
		}
		if c < 0x20 || c == '\\' || c >= 0x80 || len(k) == 64 {
			return "", false
		}
		k = append(k, c)
	}
}

// nodes reads the elements of the nodes array after its opening bracket,
// decoding them a batch at a time.
func (s *nodeScan) nodes() ([]oldNode, bool) {
	var nodes []oldNode
	s.keep = append(s.keep[:0], '[')
	decode := func() bool {
		// Fresh each time: the decoder does not clear elements it reuses.
		var part []oldNode
		if json.Unmarshal(append(s.keep, ']'), &part) != nil {
			return false
		}
		nodes = append(nodes, part...)
		s.keep = append(s.keep[:0], '[')
		return true
	}
	c := s.space()
	if c == ']' {
		return nil, true
	}
	for {
		s.from = s.i - 1
		if !s.value(c) {
			return nil, false
		}
		s.keep = append(s.keep, s.buf[s.from:s.i]...)
		s.from = -1
		switch s.space() {
		case ',':
			if len(s.keep) < 256<<10 {
				s.keep = append(s.keep, ',')
			} else if !decode() {
				return nil, false
			}
			c = s.space()
		case ']':
			return nodes, decode()
		default:
			return nil, false
		}
	}
}

// value consumes the value that starts with c and reports whether it is
// well formed.
func (s *nodeScan) value(c byte) bool {
	var open []byte // '{' or '[' per level
	for {
		switch {
		case c == '{' || c == '[':
			if len(open) == 1000 {
				return false
			}
			open = append(open, c)
			c = s.space()
			if c == open[len(open)-1]+2 { // empty: '}' is '{'+2, ']' is '['+2
				open = open[:len(open)-1]
				break
			}
			if open[len(open)-1] == '{' {
				if c != '"' || !s.str() || s.space() != ':' {
					return false
				}
				c = s.space()
			}
			continue
		case c == '"':
			if !s.str() {
				return false
			}
		case c == '-' || c >= '0' && c <= '9':
			if !s.number(c) {
				return false
			}
		case c == 't':
			if s.next() != 'r' || s.next() != 'u' || s.next() != 'e' {
				return false
			}
		case c == 'f':
			if s.next() != 'a' || s.next() != 'l' || s.next() != 's' || s.next() != 'e' {
				return false
			}
		case c == 'n':
			if s.next() != 'u' || s.next() != 'l' || s.next() != 'l' {
				return false
			}
		default:
			return false
		}
		// A value just ended: close levels until a comma asks for another.
		for {
			if len(open) == 0 {
				return true
			}
			top := open[len(open)-1]
			c = s.space()
			if c == top+2 {
				open = open[:len(open)-1]
				continue
			}
			if c != ',' {
				return false
			}
			c = s.space()
			if top == '{' {
				if c != '"' || !s.str() || s.space() != ':' {
					return false
				}
				c = s.space()
			}
			break
		}
	}
}

// str consumes a string after its opening quote.
func (s *nodeScan) str() bool {
	for {
		// Most of the file is string bytes: run over them in a local.
		b := s.buf[s.i:s.n]
		j := 0
		for j < len(b) && b[j] >= 0x20 && b[j] != '"' && b[j] != '\\' {
			j++
		}
		s.i += j
		switch c := s.next(); {
		case c == '"':
			return true
		case c == '\\':
			switch s.next() {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			case 'u':
				for range 4 {
					if c := s.next(); (c < '0' || c > '9') && (c|0x20 < 'a' || c|0x20 > 'f') {
						return false
					}
				}
			default:
				return false
			}
		case c < 0x20:
			return false
		}
	}
}

// number consumes a number that starts with c, leaving the byte after it
// unread.
func (s *nodeScan) number(c byte) bool {
	digits := func() bool {
		if c < '0' || c > '9' {
			return false
		}
		for c >= '0' && c <= '9' {
			c = s.next()
		}
		return true
	}
	if c == '-' {
		c = s.next()
	}
	if c == '0' {
		c = s.next()
	} else if !digits() {
		return false
	}
	if c == '.' {
		if c = s.next(); !digits() {
			return false
		}
	}
	if c == 'e' || c == 'E' {
		if c = s.next(); c == '+' || c == '-' {
			c = s.next()
		}
		if !digits() {
			return false
		}
	}
	if c == 0 {
		return false // the input ended: nothing to unread
	}
	s.i--
	return true
}
