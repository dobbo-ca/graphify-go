package detect

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ignorer answers whether a path is excluded by the .gitignore files in a tree.
// It mirrors the subset of gitignore semantics that matters for graphing a
// repo: per-directory .gitignore files, negation (!), directory-only patterns
// (trailing /), anchoring (a leading or embedded /), and the *, ?, ** globs.
// Patterns from a deeper .gitignore override those from a shallower one, and
// within one file the last matching pattern wins — matching git. Because the
// walk prunes ignored directories (git never descends into them), evaluating
// each entry against its ancestor .gitignore files is sufficient.
type ignorer struct {
	root  string
	cache map[string]*ignoreFile // dir (slash-rel to root, "" = root) -> parsed .gitignore (nil = none)
}

type ignoreFile struct {
	base  string // directory holding this .gitignore, slash-relative to root ("" for root)
	rules []ignoreRule
}

type ignoreRule struct {
	re       *regexp.Regexp
	negate   bool
	dirOnly  bool
	anchored bool // match the full sub-path (has a slash) vs. just the basename
}

func newIgnorer(root string) *ignorer {
	return &ignorer{root: root, cache: map[string]*ignoreFile{}}
}

// load returns the parsed .gitignore for the directory dir (slash-relative to
// root), reading and caching it on first use. A missing file caches as nil.
func (g *ignorer) load(dir string) *ignoreFile {
	if f, ok := g.cache[dir]; ok {
		return f
	}
	data, err := os.ReadFile(filepath.Join(g.root, filepath.FromSlash(dir), ".gitignore"))
	if err != nil {
		g.cache[dir] = nil
		return nil
	}
	f := parseIgnore(dir, string(data))
	g.cache[dir] = f
	return f
}

// ignored reports whether rel (slash-relative to root) is excluded, given
// whether it is a directory. It consults every ancestor directory's .gitignore
// from the root down, so deeper files take precedence.
func (g *ignorer) ignored(rel string, isDir bool) bool {
	decision := false
	for _, dir := range ancestorDirs(rel) {
		f := g.load(dir)
		if f == nil {
			continue
		}
		// Path of rel relative to this .gitignore's directory.
		sub := rel
		if dir != "" {
			sub = strings.TrimPrefix(rel, dir+"/")
		}
		base := sub
		if i := strings.LastIndexByte(sub, '/'); i >= 0 {
			base = sub[i+1:]
		}
		for _, r := range f.rules {
			if r.dirOnly && !isDir {
				continue
			}
			target := base
			if r.anchored {
				target = sub
			}
			if r.re.MatchString(target) {
				decision = !r.negate
			}
		}
	}
	return decision
}

// ancestorDirs returns the directories to consult for rel, from root ("") down
// to rel's immediate parent. For "a/b/c" -> ["", "a", "a/b"].
func ancestorDirs(rel string) []string {
	dirs := []string{""}
	parts := strings.Split(rel, "/")
	for i := 0; i < len(parts)-1; i++ {
		dirs = append(dirs, strings.Join(parts[:i+1], "/"))
	}
	return dirs
}

func parseIgnore(base, data string) *ignoreFile {
	f := &ignoreFile{base: base}
	for _, raw := range strings.Split(data, "\n") {
		line := strings.TrimRight(raw, "\r")
		line = strings.TrimRight(line, " ")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r := ignoreRule{}
		if strings.HasPrefix(line, "!") {
			r.negate = true
			line = line[1:]
		}
		line = strings.TrimPrefix(line, `\`) // escaped leading # or !
		if strings.HasSuffix(line, "/") {
			r.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		if line == "" {
			continue
		}
		// A pattern is anchored if it has a leading or embedded slash; otherwise
		// it matches the basename at any depth.
		leadingSlash := strings.HasPrefix(line, "/")
		trimmed := strings.TrimPrefix(line, "/")
		r.anchored = leadingSlash || strings.Contains(trimmed, "/")
		r.re = regexp.MustCompile("^" + globToRegex(trimmed, r.anchored) + "$")
		f.rules = append(f.rules, r)
	}
	return f
}

// globToRegex converts a gitignore glob to a regex body. When anchored, "/" is
// significant: "*" and "?" do not cross it and "**" spans path segments. When
// unanchored the pattern is matched against a single path component, so there
// are no slashes to worry about. An anchored pattern also matches everything
// under a matched directory (the trailing (/.*)? ).
func globToRegex(glob string, anchored bool) string {
	var b strings.Builder
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch c {
		case '*':
			if anchored && i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++ // consume the slash after ** so "**/" can match zero dirs
				}
			} else if anchored {
				b.WriteString("[^/]*")
			} else {
				b.WriteString(".*")
			}
		case '?':
			if anchored {
				b.WriteString("[^/]")
			} else {
				b.WriteString(".")
			}
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '\\', '[', ']':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	if anchored {
		b.WriteString("(/.*)?")
	}
	return b.String()
}
