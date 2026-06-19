package export

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/security"
)

// --- OKF (Open Knowledge Format) bundle ---
//
// OKFFromJSON serializes the graph at jsonPath as an Open Knowledge Format
// bundle under outDir: one markdown concept document per node (YAML frontmatter
// + a "# Relations" section of bundle-relative links), plus an index.md at each
// directory level. The bundle is human- and agent-friendly, SDK-free, and
// byte-stable across rebuilds (nodes, edges, and directory entries are sorted),
// so it diffs cleanly. Only graph metadata is emitted, never source file bodies.
//
// Concept paths mirror the source tree: a node from src/a.go lands at
// /src/a.go/<id>.md, and cross-links use the absolute, bundle-relative form
// (leading "/", ".md" suffix) recommended by the OKF spec §5.1.

// concept is one node placed at its bundle-relative path (no leading slash, no
// .md suffix in id; "link" is the absolute form used inside relation lists).
type concept struct {
	node jsonNode
	dir  string // bundle-relative directory, e.g. "src/a.go" ("" = root)
	file string // file name without extension, e.g. "src_a_go_add"
}

func (c concept) link() string {
	return "/" + path.Join(c.dir, c.file) + ".md"
}

// OKFFromJSON writes an OKF bundle for the graph at jsonPath into outDir.
func OKFFromJSON(jsonPath, outDir string) error {
	g, err := readGraphJSON(jsonPath)
	if err != nil {
		return err
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return err
	}

	// Place every node at a bundle path derived from its source file, then index
	// by ID so relation links can resolve endpoints.
	concepts := make([]concept, 0, len(g.Nodes))
	byID := make(map[string]concept, len(g.Nodes))
	for _, n := range g.Nodes {
		c := concept{node: n, dir: conceptDir(n.SourceFile), file: idutil.MakeID(n.ID)}
		concepts = append(concepts, c)
		byID[n.ID] = c
	}
	sort.Slice(concepts, func(i, j int) bool { return concepts[i].file < concepts[j].file })

	// Group edges by node so each concept lists its own relations (both directions),
	// mirroring `graphify explain`.
	rels := relationsByNode(g, byID)

	dirs := map[string]bool{"": true}
	for _, c := range concepts {
		body := conceptDoc(c, rels[c.node.ID])
		full, err := safeJoin(absOut, c.dir, c.file+".md")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			return err
		}
		for d := c.dir; ; {
			dirs[d] = true
			if d == "" {
				break
			}
			d = parentDir(d)
		}
	}

	return writeIndexes(absOut, dirs, concepts)
}

// relation is one edge as seen from a node: its direction marker, relation name,
// and the absolute bundle link to the other endpoint.
type relation struct {
	rel, dir, label, link string
}

func relationsByNode(g jsonGraph, byID map[string]concept) map[string][]relation {
	out := map[string][]relation{}
	add := func(from, rel, dir, other string) {
		c, ok := byID[other]
		if !ok {
			return
		}
		out[from] = append(out[from], relation{
			rel: rel, dir: dir, label: security.SanitizeLabel(c.node.Label), link: c.link(),
		})
	}
	for _, e := range g.Links {
		add(e.Source, e.Relation, "->", e.Target)
		add(e.Target, e.Relation, "<-", e.Source)
	}
	for id := range out {
		r := out[id]
		sort.Slice(r, func(i, j int) bool {
			if r[i].rel != r[j].rel {
				return r[i].rel < r[j].rel
			}
			if r[i].dir != r[j].dir {
				return r[i].dir < r[j].dir
			}
			return r[i].link < r[j].link
		})
	}
	return out
}

// conceptDoc renders one node as an OKF concept document: YAML frontmatter (with
// the required `type` field) followed by a "# Source" line and a grouped
// "# Relations" section.
func conceptDoc(c concept, rels []relation) string {
	n := c.node
	kind := n.FileType
	if kind == "" {
		kind = "concept"
	}
	desc := fmt.Sprintf("degree %d", len(rels))
	if n.Community != nil {
		desc = fmt.Sprintf("community %d, %s", *n.Community, desc)
	}
	resource := n.SourceFile
	if n.SourceLocation != "" {
		resource += ":" + n.SourceLocation
	}

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "type: %s\n", yamlString(kind))
	fmt.Fprintf(&b, "title: %s\n", yamlString(security.SanitizeLabel(n.Label)))
	fmt.Fprintf(&b, "description: %s\n", yamlString(desc))
	if resource != "" {
		fmt.Fprintf(&b, "resource: %s\n", yamlString(security.SanitizeLabel(resource)))
	}
	b.WriteString("tags:\n")
	if n.Community != nil {
		fmt.Fprintf(&b, "  - %s\n", yamlString("community:"+strconv.Itoa(*n.Community)))
	}
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n\n", security.SanitizeLabel(n.Label))
	if resource != "" {
		fmt.Fprintf(&b, "# Source\n\n%s\n\n", security.SanitizeLabel(resource))
	}

	b.WriteString("# Relations\n\n")
	if len(rels) == 0 {
		b.WriteString("_none_\n")
		return b.String()
	}
	lastRel := ""
	for _, r := range rels {
		if r.rel != lastRel {
			if lastRel != "" {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "## %s\n\n", r.rel)
			lastRel = r.rel
		}
		fmt.Fprintf(&b, "- %s [%s](%s)\n", r.dir, r.label, r.link)
	}
	return b.String()
}

// writeIndexes writes an index.md at every directory level for progressive
// disclosure (OKF §6): each lists the child concepts and subdirectories at that
// level with their descriptions.
func writeIndexes(absOut string, dirs map[string]bool, concepts []concept) error {
	childConcepts := map[string][]concept{}
	for _, c := range concepts {
		childConcepts[c.dir] = append(childConcepts[c.dir], c)
	}
	childDirs := map[string][]string{}
	for d := range dirs {
		if d == "" {
			continue
		}
		p := parentDir(d)
		childDirs[p] = append(childDirs[p], d)
	}

	for d := range dirs {
		var b strings.Builder
		title := d
		if title == "" {
			title = "/"
		}
		fmt.Fprintf(&b, "# %s\n\n", title)

		subs := childDirs[d]
		sort.Strings(subs)
		for _, sub := range subs {
			fmt.Fprintf(&b, "- [%s](/%s/index.md)\n", path.Base(sub), sub)
		}

		cs := append([]concept(nil), childConcepts[d]...)
		sort.Slice(cs, func(i, j int) bool { return cs[i].file < cs[j].file })
		for _, c := range cs {
			fmt.Fprintf(&b, "- [%s](%s) - %s\n",
				security.SanitizeLabel(c.node.Label), c.link(), conceptDescription(c.node))
		}

		full, err := safeJoin(absOut, d, "index.md")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(b.String()), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func conceptDescription(n jsonNode) string {
	if n.Community != nil {
		return fmt.Sprintf("community %d", *n.Community)
	}
	return n.FileType
}

// conceptDir maps a source file to a bundle-relative directory by sanitising each
// path segment with idutil. An empty source file places the node at the root.
func conceptDir(src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(src), "/")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := idutil.MakeID(p); id != "" {
			kept = append(kept, id)
		}
	}
	return strings.Join(kept, "/")
}

func parentDir(d string) string {
	i := strings.LastIndex(d, "/")
	if i < 0 {
		return ""
	}
	return d[:i]
}

// safeJoin joins bundle-relative parts under base and verifies the result stays
// inside base (defence against path traversal), mirroring the containment check
// in internal/security.
func safeJoin(base, dir, name string) (string, error) {
	full := filepath.Join(base, filepath.FromSlash(dir), name)
	rel, err := filepath.Rel(base, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("OKF path %q escapes bundle root %s", filepath.Join(dir, name), base)
	}
	return full, nil
}

// yamlString quotes a frontmatter scalar so colons, leading markers, and other
// YAML-significant characters can't break the document.
func yamlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
