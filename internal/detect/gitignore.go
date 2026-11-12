package detect

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// ignorer answers whether a path is excluded by the .gitignore (and
// .graphifyignore) files in a tree. It mirrors the subset of gitignore
// semantics that matters for graphing a repo: per-directory .gitignore files,
// negation (!), directory-only patterns (trailing /), anchoring (a leading or
// embedded /), and the *, ?, [..], ** globs. Patterns from a deeper .gitignore
// override those from a shallower one, and within one file the last matching
// pattern wins — matching git. Because the walk prunes ignored directories (git
// never descends into them), evaluating each entry against its ancestor
// .gitignore files is sufficient. Ignore files in directories above the scan
// root, up to the VCS root, apply too (see initAncestors).
type ignorer struct {
	root  string
	cache sync.Map // dir (slash-rel to root, "" = root) -> *ignoreFile, parsed ignore rules (nil = none)

	ancOnce     sync.Once
	anc         []*ignoreFile // dirs above root, outermost first
	ceilingRoot bool          // root is its own VCS ceiling
}

type ignoreFile struct {
	base  string // directory holding this .gitignore: slash-relative to root ("" for root); for ancestors, root relative to that dir
	rules []ignoreRule
}

type ignoreRule struct {
	re       *regexp.Regexp // unanchored: matched against the basename
	segs     []seg          // anchored: matched segment-wise against the sub-path
	globstar bool           // segs contains "**"
	negate   bool
	dirOnly  bool
	anchored bool // match the full sub-path (has a slash) vs. just the basename

	// Literal bytes every match starts with and, for a basename pattern, ends
	// with: checked first, they spare most regexp runs.
	prefix, suffix string
}

// seg is one "/"-separated piece of an anchored pattern.
type seg struct {
	globstar bool
	lit      string         // used when re is nil
	re       *regexp.Regexp // nil for a pattern with no wildcard
}

func (s seg) match(name string) bool {
	if s.re == nil {
		return name == s.lit
	}
	return s.re.MatchString(name)
}

func newIgnorer(root string) *ignorer {
	return &ignorer{root: root}
}

// readRegular reads path only when it is a regular file, so a FIFO or device
// named .gitignore can never block the scan.
func readRegular(path string) ([]byte, bool) {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return nil, false
	}
	data, err := os.ReadFile(path)
	return data, err == nil
}

// load returns the parsed ignore rules for the directory dir (slash-relative to
// root), reading and caching them on first use. Both .gitignore and
// .graphifyignore are read; .graphifyignore rules are appended after the
// .gitignore rules so they win on conflict via last-match-wins — letting a repo
// exclude neutrally-named sensitive files (e.g. prod-dump.sql) from the graph
// artifacts without touching .gitignore. A directory with neither file caches
// as nil.
func (g *ignorer) load(dir string) *ignoreFile {
	if f, ok := g.cache.Load(dir); ok {
		return f.(*ignoreFile)
	}
	var f *ignoreFile
	// At the VCS root, $GIT_DIR/info/exclude patterns apply repo-wide at the
	// LOWEST precedence: load them first so a nearer .gitignore/.graphifyignore
	// negation still wins via last-match-wins. This keeps a nested `git worktree
	// add` copy (recorded only in .git/info/exclude, not .gitignore) from being
	// fully indexed (#1810, #1809). When the VCS root is above the scan root the
	// ancestor layer carries it instead.
	if dir == "" && g.ceilingIsRoot() {
		if excl := infoExcludeOf(g.root); excl != "" {
			if data, ok := readRegular(excl); ok {
				f = parseIgnore("", string(data))
			}
		}
	}
	for _, name := range []string{".gitignore", ".graphifyignore"} {
		data, ok := readRegular(filepath.Join(g.root, filepath.FromSlash(dir), name))
		if !ok {
			continue
		}
		parsed := parseIgnore(dir, string(data))
		if f == nil {
			f = parsed
		} else {
			f.rules = append(f.rules, parsed.rules...)
		}
	}
	g.cache.Store(dir, f) // two walkers may both read it; same result
	return f
}

// infoExcludeOf returns the path to the repo's $GIT_DIR/info/exclude for the
// repo rooted at dir, or "" when dir is not a git repo or has no exclude file.
// When <dir>/.git is a FILE (a linked worktree or submodule), it follows the
// `gitdir:` pointer and then the worktree's `commondir` to the shared git dir,
// where info/exclude lives — mirroring git's own resolution (#1809).
func infoExcludeOf(dir string) string {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	gitDir := gitPath
	isFile := !info.IsDir()
	if isFile {
		data, ok := readRegular(gitPath)
		if !ok {
			return ""
		}
		line := strings.TrimSpace(string(data))
		const prefix = "gitdir:"
		if !strings.HasPrefix(line, prefix) {
			return ""
		}
		gitDir = strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(dir, gitDir)
		}
	}
	// A linked worktree's gitdir carries a `commondir` pointing at the shared git
	// dir, which is where info/exclude actually lives.
	commonDir := gitDir
	data, ok := []byte(nil), false
	if isFile { // upstream skips commondir for a .git dir
		data, ok = readRegular(filepath.Join(gitDir, "commondir"))
	}
	if ok {
		if cd := strings.TrimSpace(string(data)); cd != "" {
			if !filepath.IsAbs(cd) {
				cd = filepath.Join(gitDir, cd)
			}
			commonDir = filepath.Clean(cd)
		}
	}
	excl := filepath.Join(commonDir, "info", "exclude")
	if st, err := os.Stat(excl); err != nil || !st.Mode().IsRegular() {
		return ""
	}
	return excl
}

// isValidGitMarker mirrors upstream: a .git dir needs HEAD, objects/ and
// refs/; a .git file must start with "gitdir:".
func isValidGitMarker(dotGit string) bool {
	st, err := os.Stat(dotGit)
	if err != nil {
		return false
	}
	if st.IsDir() {
		head, err := os.Stat(filepath.Join(dotGit, "HEAD"))
		if err != nil || !head.Mode().IsRegular() {
			return false
		}
		for _, d := range []string{"objects", "refs"} {
			if s, err := os.Stat(filepath.Join(dotGit, d)); err != nil || !s.IsDir() {
				return false
			}
		}
		return true
	}
	if !st.Mode().IsRegular() {
		return false
	}
	f, err := os.Open(dotGit)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 7)
	n, _ := f.Read(buf)
	return string(buf[:n]) == "gitdir:"
}

func hasVCSMarker(dir string) bool {
	if isValidGitMarker(filepath.Join(dir, ".git")) {
		return true
	}
	for _, m := range []string{".hg", ".svn", "_darcs", ".fossil"} {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	return false
}

// findVCSRoot walks up from start (already resolved) to the first directory
// with a VCS marker, giving up at the filesystem root or $HOME.
func findVCSRoot(start string) string {
	home, _ := os.UserHomeDir()
	home = filepath.Clean(home)
	cur := start
	for {
		if hasVCSMarker(cur) {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur || cur == home {
			return ""
		}
		cur = parent
	}
}

func (g *ignorer) ceilingIsRoot() bool {
	g.ancOnce.Do(g.initAncestors)
	return g.ceilingRoot
}

// initAncestors loads the ignore files of every directory from the VCS root
// down to (not including) the scan root, plus the VCS root's info/exclude.
func (g *ignorer) initAncestors() {
	g.ceilingRoot = true
	root := resolveRoot(g.root)
	ceiling := findVCSRoot(root)
	if ceiling == "" || ceiling == root {
		return
	}
	g.ceilingRoot = false
	var dirs []string // innermost first
	for d := filepath.Dir(root); ; d = filepath.Dir(d) {
		dirs = append(dirs, d)
		if d == ceiling || filepath.Dir(d) == d {
			break
		}
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		d := dirs[i]
		rel, err := filepath.Rel(d, root)
		if err != nil {
			continue
		}
		f := &ignoreFile{base: filepath.ToSlash(rel)}
		if d == ceiling {
			if excl := infoExcludeOf(d); excl != "" {
				if data, ok := readRegular(excl); ok {
					f.rules = append(f.rules, parseIgnore("", string(data)).rules...)
				}
			}
		}
		for _, name := range []string{".gitignore", ".graphifyignore"} {
			if data, ok := readRegular(filepath.Join(d, name)); ok {
				f.rules = append(f.rules, parseIgnore("", string(data)).rules...)
			}
		}
		if len(f.rules) > 0 {
			g.anc = append(g.anc, f)
		}
	}
}

// ignored reports whether rel (slash-relative to root) is excluded, given
// whether it is a directory. It consults every ancestor directory's .gitignore
// from the root down, so deeper files take precedence.
func (g *ignorer) ignored(rel string, isDir bool) bool {
	g.ancOnce.Do(g.initAncestors)
	decision := false
	for _, f := range g.anc {
		decision = applyRules(f.rules, f.base+"/"+rel, isDir, decision)
	}
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
		decision = applyRules(f.rules, sub, isDir, decision)
	}
	return decision
}

// applyRules runs rules in order over sub and returns the last match's verdict
// (or decision when nothing matches).
func applyRules(rules []ignoreRule, sub string, isDir, decision bool) bool {
	base := sub
	if i := strings.LastIndexByte(sub, '/'); i >= 0 {
		base = sub[i+1:]
	}
	for i := range rules {
		r := &rules[i]
		if r.dirOnly && !isDir {
			continue
		}
		target := base
		if r.anchored {
			target = sub
		}
		if strings.HasPrefix(target, r.prefix) && strings.HasSuffix(target, r.suffix) && r.matches(target) {
			decision = !r.negate
		}
	}
	return decision
}

// matches tests target (the basename, or the sub-path for anchored rules).
func (r *ignoreRule) matches(target string) bool {
	if !r.anchored {
		return r.re.MatchString(target)
	}
	if !r.globstar {
		rest := target
		for i, s := range r.segs {
			part, tail, more := strings.Cut(rest, "/")
			if more != (i < len(r.segs)-1) || !s.match(part) {
				return false
			}
			rest = tail
		}
		return true
	}
	return matchGlobstar(strings.Split(target, "/"), r.segs, 0, 0, map[[2]int]bool{})
}

// matchGlobstar ports upstream _match_globstar_parts: "**" spans zero or more
// segments, but one or more when it is the last segment.
func matchGlobstar(path []string, pat []seg, pi, qi int, memo map[[2]int]bool) bool {
	key := [2]int{pi, qi}
	if v, ok := memo[key]; ok {
		return v
	}
	var res bool
	switch {
	case qi == len(pat):
		res = pi == len(path)
	case pat[qi].globstar:
		if qi == len(pat)-1 {
			res = pi < len(path)
		} else {
			res = matchGlobstar(path, pat, pi, qi+1, memo) ||
				(pi < len(path) && matchGlobstar(path, pat, pi+1, qi, memo))
		}
	default:
		res = pi < len(path) && pat[qi].match(path[pi]) && matchGlobstar(path, pat, pi+1, qi+1, memo)
	}
	memo[key] = res
	return res
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
		if r.anchored {
			ok := true
			for _, part := range strings.Split(trimmed, "/") {
				s := seg{lit: part}
				switch {
				case part == "**":
					s.globstar = true
					r.globstar = true
				case strings.ContainsAny(part, "*?["):
					re, err := regexp.Compile(fnmatchRegex(part))
					if err != nil {
						ok = false
					}
					s.re = re
				}
				r.segs = append(r.segs, s)
			}
			if !ok {
				continue
			}
		} else {
			re, err := regexp.Compile(fnmatchRegex(trimmed))
			if err != nil {
				continue
			}
			r.re = re
		}
		// Only *, ? and [ start a wildcard; a "[" with no closing "]" stays
		// literal, so stopping the literals there is merely conservative.
		r.prefix = trimmed
		if i := strings.IndexAny(trimmed, "*?["); i >= 0 {
			r.prefix = trimmed[:i]
			if !r.anchored && !strings.Contains(trimmed, "[") {
				r.suffix = trimmed[strings.LastIndexAny(trimmed, "*?")+1:]
			}
		}
		f.rules = append(f.rules, r)
	}
	return f
}

// fnmatchRegex ports Python fnmatch.translate to an anchored RE2 expression.
func fnmatchRegex(pat string) string {
	p := []rune(pat)
	n := len(p)
	var b strings.Builder
	b.WriteString("^(?s:")
	i := 0
	for i < n {
		c := p[i]
		i++
		switch c {
		case '*':
			b.WriteString(".*")
			for i < n && p[i] == '*' {
				i++
			}
		case '?':
			b.WriteByte('.')
		case '[':
			j := i
			if j < n && p[j] == '!' {
				j++
			}
			if j < n && p[j] == ']' {
				j++
			}
			for j < n && p[j] != ']' {
				j++
			}
			if j >= n {
				b.WriteString(`\[`)
				continue
			}
			chunks := classChunks(p, i, j)
			i = j + 1
			stuff := strings.Join(chunks, "-")
			switch {
			case stuff == "":
				b.WriteString(`[^\x00-\x{10FFFF}]`) // never matches
			case stuff == "!":
				b.WriteByte('.')
			default:
				b.WriteByte('[')
				if strings.HasPrefix(stuff, "!") {
					b.WriteByte('^')
					chunks[0] = chunks[0][1:]
				}
				for ci, ch := range chunks {
					if ci > 0 {
						b.WriteByte('-')
					}
					for _, r := range ch {
						if strings.ContainsRune(`\]^[-`, r) {
							b.WriteByte('\\')
						}
						b.WriteRune(r)
					}
				}
				b.WriteByte(']')
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString(")$")
	return b.String()
}

// classChunks splits the class body p[i:j] at range hyphens and drops
// reversed ranges, exactly as fnmatch.translate does.
func classChunks(p []rune, i, j int) []string {
	hyphen := func(from int) int {
		for k := from; k < j; k++ {
			if p[k] == '-' {
				return k
			}
		}
		return -1
	}
	if hyphen(i) < 0 {
		return []string{string(p[i:j])}
	}
	var chunks [][]rune
	k := i + 1
	if p[i] == '!' {
		k = i + 2
	}
	for {
		k = hyphen(k)
		if k < 0 {
			break
		}
		chunks = append(chunks, p[i:k])
		i = k + 1
		k += 3
	}
	if i < j {
		chunks = append(chunks, p[i:j])
	} else if len(chunks) > 0 {
		last := append([]rune{}, chunks[len(chunks)-1]...)
		chunks[len(chunks)-1] = append(last, '-')
	}
	for k := len(chunks) - 1; k > 0; k-- {
		a, c := chunks[k-1], chunks[k]
		if len(a) > 0 && len(c) > 0 && a[len(a)-1] > c[0] {
			merged := append(append([]rune{}, a[:len(a)-1]...), c[1:]...)
			chunks[k-1] = merged
			chunks = append(chunks[:k], chunks[k+1:]...)
		}
	}
	out := make([]string, len(chunks))
	for k, c := range chunks {
		out[k] = string(c)
	}
	return out
}
