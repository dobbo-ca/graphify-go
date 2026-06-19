package query

import (
	"encoding/json"
	"fmt"
	"os"
)

// Caps for the merge driver, mirroring the Python original. They are well above
// any realistic graph (typical graphs are <5 MB / <50k nodes); anything larger
// fails the merge so a human can investigate rather than have git silently
// accept a corrupt or poisoned graph.json.
const (
	mergeMaxBytes = 50 * 1024 * 1024
	mergeMaxNodes = 100_000
)

// rawGraph holds a graph.json with its nodes and links left as raw JSON, so a
// merge round-trips every node/edge attribute (community, confidence_score, …)
// without this package having to model them all.
type rawGraph struct {
	Directed      bool              `json:"directed"`
	Multigraph    bool              `json:"multigraph"`
	Graph         json.RawMessage   `json:"graph"`
	Nodes         []json.RawMessage `json:"nodes"`
	Links         []json.RawMessage `json:"links"`
	Hyperedges    json.RawMessage   `json:"hyperedges,omitempty"`
	BuiltAtCommit json.RawMessage   `json:"built_at_commit,omitempty"`
}

// nodeKey and linkKey extract just the dedup-relevant fields from a raw entry.
type nodeKey struct {
	ID string `json:"id"`
}
type linkKey struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
}

// Merge resolves a graph.json merge conflict by writing the node/edge union of
// currentPath and otherPath back to currentPath. Nodes dedupe by id (current
// wins), edges by direction-aware (source, target, relation). It returns an
// error on corrupt or oversized input (per-file byte cap, merged-node cap) so
// the caller can fail the git merge instead of accepting a poisoned graph.
func Merge(currentPath, otherPath string) error {
	cur, err := loadRawGraph(currentPath)
	if err != nil {
		return err
	}
	oth, err := loadRawGraph(otherPath)
	if err != nil {
		return err
	}

	out := rawGraph{
		Directed: cur.Directed, Multigraph: cur.Multigraph, Graph: cur.Graph,
		Hyperedges: cur.Hyperedges, BuiltAtCommit: cur.BuiltAtCommit,
	}

	seenNode := map[string]bool{}
	for _, src := range [][]json.RawMessage{cur.Nodes, oth.Nodes} {
		for _, raw := range src {
			var k nodeKey
			if err := json.Unmarshal(raw, &k); err != nil {
				return fmt.Errorf("parsing node: %w", err)
			}
			if seenNode[k.ID] {
				continue
			}
			seenNode[k.ID] = true
			out.Nodes = append(out.Nodes, raw)
		}
	}
	if len(out.Nodes) > mergeMaxNodes {
		return fmt.Errorf("merged graph has %d nodes, exceeds %d-node cap", len(out.Nodes), mergeMaxNodes)
	}

	seenLink := map[linkKey]bool{}
	for _, src := range [][]json.RawMessage{cur.Links, oth.Links} {
		for _, raw := range src {
			var k linkKey
			if err := json.Unmarshal(raw, &k); err != nil {
				return fmt.Errorf("parsing link: %w", err)
			}
			if seenLink[k] {
				continue
			}
			seenLink[k] = true
			out.Links = append(out.Links, raw)
		}
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(currentPath, data, 0o644)
}

// loadRawGraph reads a graph.json, rejecting files over the byte cap before
// parsing so a crafted oversized graph cannot exhaust memory.
func loadRawGraph(path string) (*rawGraph, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot stat %s: %w", path, err)
	}
	if info.Size() > mergeMaxBytes {
		return nil, fmt.Errorf("graph file %s is %d bytes, exceeds %d-byte cap", path, info.Size(), mergeMaxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g rawGraph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &g, nil
}
