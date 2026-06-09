package query

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/dobbo-ca/graphify-go/internal/security"
)

// Validate reads graph.json at path and reports structural problems: links that
// reference a missing node (dangling endpoints), duplicate node IDs, and nodes
// with an empty ID. It returns the issues (empty when the graph is sound) plus
// node and edge counts for a summary. Reading directly (rather than via Load)
// lets it catch duplicate IDs, which Load's id map would otherwise collapse.
func Validate(path string) (issues []string, nodes, links int, err error) {
	if err = security.CheckGraphFileSize(path); err != nil {
		return nil, 0, 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, 0, err
	}
	var g Graph
	if err = json.Unmarshal(data, &g); err != nil {
		return nil, 0, 0, fmt.Errorf("parsing %s: %w", path, err)
	}

	seen := map[string]int{}
	for _, n := range g.Nodes {
		if n.ID == "" {
			issues = append(issues, fmt.Sprintf("node with empty id (label %q)", n.Label))
			continue
		}
		seen[n.ID]++
	}
	for id, count := range seen {
		if count > 1 {
			issues = append(issues, fmt.Sprintf("duplicate node id %q (%d times)", id, count))
		}
	}
	for _, l := range g.Links {
		if seen[l.Source] == 0 {
			issues = append(issues, fmt.Sprintf("edge %s --%s--> %s: source node missing", l.Source, l.Relation, l.Target))
		}
		if seen[l.Target] == 0 {
			issues = append(issues, fmt.Sprintf("edge %s --%s--> %s: target node missing", l.Source, l.Relation, l.Target))
		}
	}
	sort.Strings(issues)
	return issues, len(g.Nodes), len(g.Links), nil
}
