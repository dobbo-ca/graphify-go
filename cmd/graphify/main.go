// Command graphify builds a knowledge graph from a source tree and answers
// queries against it. Build writes graphify-out/{graph.json,graph.html,
// GRAPH_REPORT.md}; query/path/explain read graph.json so an agent can navigate
// the codebase without grepping.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/detect"
	"github.com/dobbo-ca/graphify-go/internal/export"
	"github.com/dobbo-ca/graphify-go/internal/extract"
	"github.com/dobbo-ca/graphify-go/internal/graph"
	"github.com/dobbo-ca/graphify-go/internal/query"
	"github.com/dobbo-ca/graphify-go/internal/report"
)

const defaultGraphPath = "graphify-out/graph.json"

// Build metadata, injected via -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Printf("graphify %s (commit %s, built %s)\n", version, commit, date)
		return
	case "build":
		err = cmdBuild(arg(2, "."))
	case "query":
		err = cmdQuery(mustArg(2, "query <pattern>"))
	case "explain":
		err = cmdExplain(mustArg(2, "explain <node>"))
	case "path":
		if len(os.Args) < 4 {
			err = fmt.Errorf("usage: graphify path <from> <to>")
		} else {
			err = cmdPath(os.Args[2], os.Args[3])
		}
	case "extract":
		err = cmdExtract(mustArg(2, "extract <file>"))
	case "-h", "--help", "help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "graphify:", err)
		os.Exit(1)
	}
}

func cmdBuild(root string) error {
	files, err := detect.CollectFiles(root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no supported source files found under %s", root)
	}
	results := make([]extract.Result, 0, len(files))
	for _, f := range files {
		r, err := extract.File(root, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: skipped %s (%v)\n", f, err)
			continue
		}
		results = append(results, r)
	}
	g := graph.Build(extract.Resolve(results, files))
	communities := cluster.Cluster(g)
	commit := gitHead(root)

	outDir := filepath.Join(root, "graphify-out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := export.ToJSON(g, communities, filepath.Join(outDir, "graph.json"), commit); err != nil {
		return err
	}
	if err := export.ToHTML(g, communities, filepath.Join(outDir, "graph.html")); err != nil {
		return err
	}
	md := report.Generate(g, communities, root, commit)
	if err := os.WriteFile(filepath.Join(outDir, "GRAPH_REPORT.md"), []byte(md), 0o644); err != nil {
		return err
	}
	fmt.Printf("built graph: %d files · %d nodes · %d edges · %d communities → %s\n",
		len(files), g.NumNodes(), g.NumEdges(), len(communities), outDir)
	return nil
}

func cmdQuery(pattern string) error {
	g, err := load()
	if err != nil {
		return err
	}
	matches, err := query.Query(g, pattern)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		fmt.Println("no matches")
		return nil
	}
	for _, m := range matches {
		fmt.Printf("%-40s %s\n", m.Label, m.Location)
	}
	return nil
}

func cmdExplain(id string) error {
	g, err := load()
	if err != nil {
		return err
	}
	ex, err := query.Explain(g, id)
	if err != nil {
		return err
	}
	fmt.Printf("%s  [%s]\n  %s\n", ex.Node.Label, ex.Node.FileType, locOf(ex.Node))
	if len(ex.Neighbors) == 0 {
		fmt.Println("  (no connections)")
	}
	for _, n := range ex.Neighbors {
		fmt.Printf("  %s %-12s %-32s %s\n", n.Direction, n.Relation, n.Label, n.Location)
	}
	return nil
}

func cmdPath(from, to string) error {
	g, err := load()
	if err != nil {
		return err
	}
	nodes, err := query.Path(g, from, to)
	if err != nil {
		return err
	}
	parts := make([]string, len(nodes))
	for i, n := range nodes {
		parts[i] = n.Label
	}
	fmt.Println(strings.Join(parts, " -> "))
	return nil
}

func cmdExtract(file string) error {
	root, rel := ".", file
	if dir := filepath.Dir(file); dir != "." {
		root, rel = dir, filepath.Base(file)
	}
	r, err := extract.File(root, rel)
	if err != nil {
		return err
	}
	ext := extract.Resolve([]extract.Result{r}, []string{rel})
	for _, n := range ext.Nodes {
		fmt.Printf("node  %-40s %s %s\n", n.Label, n.SourceFile, n.SourceLocation)
	}
	for _, e := range ext.Edges {
		fmt.Printf("edge  %-12s %s -> %s\n", e.Relation, e.Source, e.Target)
	}
	return nil
}

func load() (*query.Graph, error) { return query.Load(defaultGraphPath) }

func locOf(n *query.Node) string {
	if n.SourceFile == "" {
		return "(external)"
	}
	return n.SourceFile + " " + n.SourceLocation
}

// gitHead returns the current commit SHA of the repo at root, or "" if unavailable.
func gitHead(root string) string {
	cmd := exec.Command("git", "-C", root, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func arg(i int, def string) string {
	if len(os.Args) > i {
		return os.Args[i]
	}
	return def
}

func mustArg(i int, usageStr string) string {
	if len(os.Args) <= i {
		fmt.Fprintln(os.Stderr, "usage: graphify "+usageStr)
		os.Exit(2)
	}
	return os.Args[i]
}

func usage() {
	fmt.Println(`graphify - code knowledge graph

usage:
  graphify build [path]        build graph.json + report under <path>/graphify-out
  graphify query <pattern>     find nodes by name (regex, case-insensitive)
  graphify explain <node>      show a node and its neighbours
  graphify path <from> <to>    shortest dependency path between two nodes
  graphify extract <file>      print one file's extracted nodes/edges (debug)
  graphify version             print version`)
}
