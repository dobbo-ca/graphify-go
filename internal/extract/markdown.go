package extract

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// mdLink matches an inline markdown link [text](target). The target is captured
// up to the first space or closing paren so optional `"titles"` are dropped.
var mdLink = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)`)

// mdRefDef matches a reference-style link definition `[label]: target` at the
// start of a line, dropping an optional <...> wrapper and any trailing title.
var mdRefDef = regexp.MustCompile(`^\s{0,3}\[[^\]]+\]:\s*<?([^\s>]+)>?`)

// mdWikilink matches a `[[target]]` wikilink, discarding any `#section` anchor
// or `|alias` display text so only the target survives.
var mdWikilink = regexp.MustCompile(`\[\[([^\]|#]+)(?:[#|][^\]]*)?\]\]`)

// mdHeading matches an ATX heading line (one to six `#` then heading text); the
// `#` count is the heading level used for nesting.
var mdHeading = regexp.MustCompile(`^(#{1,6})\s+(.+)`)

// mdCodeSpan matches an inline `code` span. A single-token span is a candidate
// reference to that bare symbol name (resolved, if at all, in the resolve pass).
var mdCodeSpan = regexp.MustCompile("`([^`\n]+)`")

// MDRef is an unresolved markdown reference: the FromID node linked to the raw
// Target before corpus resolution. Target is a link/wikilink/reference-def
// destination path, or a bare `symbol` name captured from an inline code span.
type MDRef struct {
	FromID, Target, File, Loc string
}

// extractMarkdown turns one Markdown/OKF bundle file into a concept node plus a
// node per ATX heading. The concept node's id is the repo-relative path without
// its extension (so a link [text](/tables/orders.md) can resolve to it), its
// file_type is the frontmatter `type` (defaulting to "document"), and its
// title/description/tags are carried as label and computed_name. Each heading is
// a "heading" node with a `contains` edge from the concept node (or its nearest
// enclosing heading, by heading level). Inline links, `[[wikilinks]]`,
// `[label]: url` reference definitions and single-token inline `code` spans each
// become a raw MDRef that Resolve stitches into a `references` edge once it has
// the whole-corpus view. Fenced code blocks are skipped so their contents are
// not mistaken for headings or references.
func extractMarkdown(rel string, src []byte) Result {
	rel = filepath.ToSlash(rel)
	conceptID := idutil.MakeID(strings.TrimSuffix(rel, filepath.Ext(rel)))

	s := string(src)
	fm, body := splitFrontmatter(s)
	fileType := fm["type"]
	if fileType == "" {
		fileType = "document"
	}
	label := fm["title"]
	if label == "" {
		label = filepath.Base(rel)
	}

	var res Result
	res.Nodes = append(res.Nodes, model.Node{
		ID: conceptID, Label: label, FileType: fileType,
		SourceFile: rel, SourceLocation: "L1", ComputedName: computedMeta(fm),
	})

	// body is an exact suffix of s, so the count of newlines dropped with the
	// frontmatter is the offset that keeps heading/reference lines file-absolute.
	offset := strings.Count(s[:len(s)-len(body)], "\n")

	addRef := func(target, loc string) {
		if target = strings.TrimSpace(target); target != "" {
			res.MDRefs = append(res.MDRefs, MDRef{FromID: conceptID, Target: target, File: rel, Loc: loc})
		}
	}

	seen := map[string]bool{conceptID: true}
	type hlevel struct {
		level int
		id    string
	}
	var stack []hlevel
	inCode := false

	for i, lineText := range strings.Split(body, "\n") {
		loc := "L" + itoa(offset+i+1)
		if strings.HasPrefix(strings.TrimSpace(lineText), "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}

		// References anywhere in the doc (scanned on heading lines too). Inline
		// `code` spans are candidate references to a bare symbol name.
		for _, m := range mdLink.FindAllStringSubmatch(lineText, -1) {
			addRef(m[1], loc)
		}
		for _, m := range mdWikilink.FindAllStringSubmatch(lineText, -1) {
			addRef(m[1], loc)
		}
		if m := mdRefDef.FindStringSubmatch(lineText); m != nil {
			addRef(m[1], loc)
		}
		for _, m := range mdCodeSpan.FindAllStringSubmatch(lineText, -1) {
			if sym := strings.TrimSpace(m[1]); sym != "" && !strings.ContainsAny(sym, " \t") {
				addRef(sym, loc)
			}
		}

		m := mdHeading.FindStringSubmatch(lineText)
		if m == nil {
			continue
		}
		level := len(m[1])
		title := strings.TrimSpace(m[2])
		hid := idutil.MakeID(conceptID, title)
		if seen[hid] { // disambiguate a repeated heading title by its line
			hid = idutil.MakeID(conceptID, title, itoa(offset+i+1))
		}
		if !seen[hid] {
			seen[hid] = true
			res.Nodes = append(res.Nodes, model.Node{
				ID: hid, Label: title, FileType: "heading",
				SourceFile: rel, SourceLocation: loc,
			})
		}
		// Pop headings at the same or a deeper level, then hang this heading off
		// its nearest shallower ancestor (or the concept node for a top heading).
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			stack = stack[:len(stack)-1]
		}
		parent := conceptID
		if len(stack) > 0 {
			parent = stack[len(stack)-1].id
		}
		res.Edges = append(res.Edges, model.Edge{
			Source: parent, Target: hid, Relation: "contains",
			Confidence: "EXTRACTED", SourceFile: rel, SourceLocation: loc,
		})
		stack = append(stack, hlevel{level: level, id: hid})
	}
	return res
}

// splitFrontmatter separates a leading YAML frontmatter block (delimited by
// `---` lines) from the markdown body, returning the parsed scalar/list keys it
// needs and the remaining body. Only the flat keys used by the OKF extractor are
// parsed; nested structures are ignored. When no frontmatter is present the
// whole source is the body.
func splitFrontmatter(src string) (map[string]string, string) {
	fm := map[string]string{}
	lines := strings.Split(src, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return fm, src
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			return fm, strings.Join(lines[i+1:], "\n")
		}
		line := strings.TrimRight(lines[i], "\r")
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fm[strings.TrimSpace(key)] = unquoteFM(strings.TrimSpace(val))
	}
	// Unterminated frontmatter — treat the whole source as body.
	return map[string]string{}, src
}

// computedMeta packs the description and tags into the node's ComputedName so
// they ride along as searchable metadata without expanding the node schema.
func computedMeta(fm map[string]string) string {
	parts := make([]string, 0, 2)
	if d := fm["description"]; d != "" {
		parts = append(parts, d)
	}
	if t := fm["tags"]; t != "" {
		parts = append(parts, strings.Trim(t, "[]"))
	}
	return strings.Join(parts, " ")
}

// unquoteFM strips matching surrounding single or double quotes from a
// frontmatter scalar value.
func unquoteFM(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
