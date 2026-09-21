package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dobbo-ca/graphify-go/internal/security"
)

// memoryDir holds the Q&A docs `graphify save-result` writes. The walk enters
// it even though graphify-out is otherwise pruned, so a saved answer becomes
// graph content on the next update.
const memoryDirPath = "graphify-out/memory"

// outcomes are the work-memory signals a saved result may carry.
var outcomes = map[string]bool{"useful": true, "dead_end": true, "corrected": true}

// slugUnsafe matches every character that must not appear in a slug (mirrors
// upstream's [^\w] with the underscore kept).
var slugUnsafe = regexp.MustCompile(`[^0-9a-zA-Z_]+`)

// cmdSaveResult writes a Q&A result as a markdown file with YAML frontmatter
// under graphify-out/memory/, which the markdown extractor picks up on the next
// `graphify update` — so what an agent learned from a query becomes graph
// content for the next session. Mirrors upstream ingest.save_query_result.
func cmdSaveResult(args []string) error {
	usageStr := `usage: graphify save-result --question Q --answer A [--type T] [--nodes N...] [--outcome useful|dead_end|corrected] [--correction TEXT]`
	var question, answer, correction, outcome string
	queryType := "query"
	var nodes []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val := a, ""
		hasVal := false
		if eq := strings.Index(a, "="); strings.HasPrefix(a, "--") && eq > 0 {
			name, val, hasVal = a[:eq], a[eq+1:], true
		}
		next := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value\n%s", name, usageStr)
			}
			i++
			return args[i], nil
		}
		var err error
		switch name {
		case "--question":
			question, err = next()
		case "--answer":
			answer, err = next()
		case "--type":
			queryType, err = next()
		case "--outcome":
			outcome, err = next()
		case "--correction":
			correction, err = next()
		case "--nodes":
			// variadic: every following non-flag argument is a node id
			if hasVal {
				nodes = append(nodes, val)
				break
			}
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				nodes = append(nodes, args[i])
			}
		default:
			return fmt.Errorf("unknown flag %q\n%s", a, usageStr)
		}
		if err != nil {
			return err
		}
	}
	if question == "" {
		return fmt.Errorf("--question is required\n%s", usageStr)
	}
	if answer == "" {
		return fmt.Errorf("--answer is required\n%s", usageStr)
	}
	if outcome != "" && !outcomes[outcome] {
		return fmt.Errorf("--outcome must be one of useful, dead_end, corrected (got %q)", outcome)
	}

	path, err := saveResult(memoryDirPath, question, answer, queryType, outcome, correction, nodes)
	if err != nil {
		return err
	}
	fmt.Println("Saved to", path)
	return nil
}

// saveResult writes the memory doc into dir and returns its path.
func saveResult(dir, question, answer, queryType, outcome, correction string, nodes []string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	// The second-granularity stamp plus the slug is not unique: two saves in the
	// same second whose questions share a prefix would resolve to one path and
	// the later write would silently replace the earlier one. The random suffix
	// makes every save its own file (upstream 2f743ae).
	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	name := fmt.Sprintf("query_%s_%s_%s.md", now.Format("20060102_150405"), hex.EncodeToString(rnd[:]), slugify(question))
	path := filepath.Join(dir, name)

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "type: %s\n", yamlStr(queryType))
	fmt.Fprintf(&b, "date: %s\n", yamlStr(now.Format(time.RFC3339)))
	fmt.Fprintf(&b, "question: %s\n", yamlStr(question))
	b.WriteString("contributor: \"graphify\"\n")
	if outcome != "" {
		fmt.Fprintf(&b, "outcome: %s\n", yamlStr(outcome))
	}
	if correction != "" {
		fmt.Fprintf(&b, "correction: %s\n", yamlStr(correction))
	}
	if len(nodes) > 0 {
		quoted := make([]string, 0, 10)
		for _, n := range nodes {
			if len(quoted) == 10 {
				break
			}
			quoted = append(quoted, yamlStr(n))
		}
		fmt.Fprintf(&b, "source_nodes: [%s]\n", strings.Join(quoted, ", "))
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# Q: %s\n\n## Answer\n\n%s\n", question, answer)
	if outcome != "" || correction != "" {
		b.WriteString("\n## Outcome\n\n")
		if outcome != "" {
			fmt.Fprintf(&b, "- Signal: %s\n", outcome)
		}
		if correction != "" {
			fmt.Fprintf(&b, "- Correction: %s\n", correction)
		}
	}
	if len(nodes) > 0 {
		b.WriteString("\n## Source Nodes\n\n")
		for _, n := range nodes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

// yamlStr renders s as a YAML double-quoted scalar. security.SanitizeLabel
// drops the control characters (newlines included) that could break out of the
// scalar and inject sibling keys; the backslash and quote escapes cover the
// rest.
func yamlStr(s string) string {
	s = security.SanitizeLabel(s)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// slugify turns a question into the filename-safe tail of a memory doc.
func slugify(q string) string {
	s := slugUnsafe.ReplaceAllString(strings.ToLower(q), "_")
	if len(s) > 50 {
		s = s[:50]
	}
	return strings.Trim(s, "_")
}
