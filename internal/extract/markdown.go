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

// MDRef is an unresolved markdown link: the concept node FromID linked to the
// path Target (the raw [text](Target) destination, before corpus resolution).
type MDRef struct {
	FromID, Target, File, Loc string
}

// extractMarkdown turns one Markdown/OKF bundle file into a single concept node.
// The node's id is the repo-relative path without its extension (so a link
// [text](/tables/orders.md) can resolve to it), its file_type is the frontmatter
// `type` (defaulting to "document"), and its title/description/tags are carried
// as label and computed_name. Each inline markdown link becomes a raw MDRef that
// Resolve stitches into a `references` edge once it has the whole-corpus view.
// Headings are intentionally not emitted as sub-nodes (one node per concept).
func extractMarkdown(rel string, src []byte) Result {
	rel = filepath.ToSlash(rel)
	conceptID := idutil.MakeID(strings.TrimSuffix(rel, filepath.Ext(rel)))

	fm, body := splitFrontmatter(string(src))
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

	for _, m := range mdLink.FindAllStringSubmatch(body, -1) {
		target := m[1]
		if target == "" {
			continue
		}
		res.MDRefs = append(res.MDRefs, MDRef{FromID: conceptID, Target: target, File: rel, Loc: "L1"})
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
