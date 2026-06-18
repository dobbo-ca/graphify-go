// Command graphify builds a knowledge graph from a source tree and answers
// queries against it. Build writes graphify-out/{graph.json,graph.html,
// GRAPH_REPORT.md}; query/path/explain read graph.json so an agent can navigate
// the codebase without grepping.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/dobbo-ca/graphify-go/internal/cache"
	"github.com/dobbo-ca/graphify-go/internal/cluster"
	"github.com/dobbo-ca/graphify-go/internal/detect"
	"github.com/dobbo-ca/graphify-go/internal/export"
	"github.com/dobbo-ca/graphify-go/internal/extract"
	"github.com/dobbo-ca/graphify-go/internal/graph"
	"github.com/dobbo-ca/graphify-go/internal/model"
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
	case "update":
		err = cmdUpdate(arg(2, "."))
	case "watch":
		err = cmdWatch(arg(2, "."))
	case "hook":
		err = cmdHook(os.Args[2:])
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
	case "export":
		err = cmdExport(mustArg(2, "export <graphml|dot|csv> [path]"), arg(3, "."))
	case "affected":
		err = cmdAffected(os.Args[2:])
	case "diff":
		err = cmdDiff(os.Args[2:])
	case "validate":
		err = cmdValidate()
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

// assembleStats reports how an assemble run used the cache.
type assembleStats struct{ parsed, reused, dropped int }

// assemble produces one extraction result per file, in files order (so graph
// output stays deterministic), re-parsing only files whose content hash differs
// from prev. Files unchanged since prev reuse their cached result, skipping the
// expensive tree-sitter parse. It also returns the cache to persist and counts
// for the summary line. Files that fail to read are skipped with a warning.
func assemble(root string, files []string, prev cache.Cache) ([]extract.Result, cache.Cache, assembleStats) {
	type slot struct {
		entry  cache.Entry
		ok     bool
		reused bool
	}
	slots := make([]slot, len(files))

	workers := runtime.NumCPU()
	if workers > len(files) {
		workers = len(files)
	}
	if workers < 1 {
		workers = 1
	}
	idx := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				src, err := os.ReadFile(filepath.Join(root, files[i]))
				if err != nil {
					fmt.Fprintf(os.Stderr, "  warning: skipped %s (%v)\n", files[i], err)
					continue
				}
				h := cache.HashBytes(src)
				if e, ok := prev[filepath.ToSlash(files[i])]; ok && e.Hash == h {
					slots[i] = slot{entry: e, ok: true, reused: true}
					continue
				}
				res := extract.FileFromBytes(files[i], src)
				slots[i] = slot{entry: cache.Entry{Hash: h, Result: res}, ok: true}
			}
		}()
	}
	for i := range files {
		idx <- i
	}
	close(idx)
	wg.Wait()

	results := make([]extract.Result, 0, len(files))
	newCache := make(cache.Cache, len(files))
	var stats assembleStats
	for i, s := range slots {
		if !s.ok {
			continue
		}
		results = append(results, s.entry.Result)
		newCache[filepath.ToSlash(files[i])] = s.entry
		if s.reused {
			stats.reused++
		} else {
			stats.parsed++
		}
	}
	for f := range prev {
		if _, ok := newCache[f]; !ok {
			stats.dropped++
		}
	}
	return results, newCache, stats
}

// writeOutputs resolves the per-file results into a graph and writes graph.json,
// graph.html, GRAPH_REPORT.md, and the incremental cache under <root>/graphify-out.
func writeOutputs(root string, files []string, results []extract.Result, newCache cache.Cache) (*model.Graph, map[int][]string, error) {
	g := graph.Build(extract.Resolve(results, files))
	communities := cluster.Cluster(g)
	commit := gitHead(root)

	outDir := filepath.Join(root, "graphify-out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, nil, err
	}
	if err := export.ToJSON(g, communities, filepath.Join(outDir, "graph.json"), commit); err != nil {
		return nil, nil, err
	}
	if err := export.ToHTML(g, communities, filepath.Join(outDir, "graph.html")); err != nil {
		return nil, nil, err
	}
	md := report.Generate(g, communities, root, commit)
	if err := os.WriteFile(filepath.Join(outDir, "GRAPH_REPORT.md"), []byte(md), 0o644); err != nil {
		return nil, nil, err
	}
	if err := cache.Save(filepath.Join(outDir, cache.FileName), newCache); err != nil {
		return nil, nil, err
	}
	return g, communities, nil
}

func cmdBuild(root string) error {
	files, err := detect.CollectFiles(root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no supported source files found under %s", root)
	}
	results, newCache, _ := assemble(root, files, nil)
	g, communities, err := writeOutputs(root, files, results, newCache)
	if err != nil {
		return err
	}
	fmt.Printf("built graph: %d files · %d nodes · %d edges · %d communities → %s\n",
		len(files), g.NumNodes(), g.NumEdges(), len(communities), filepath.Join(root, "graphify-out"))
	return nil
}

// cmdUpdate rebuilds the graph incrementally: it re-parses only files whose
// content changed since the last build/update, reusing cached results for the
// rest, then resolves and writes the same outputs as build. With no existing
// cache it transparently degrades to a full build.
func cmdUpdate(root string) error {
	files, err := detect.CollectFiles(root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no supported source files found under %s", root)
	}
	prev := cache.Load(filepath.Join(root, "graphify-out", cache.FileName))
	results, newCache, stats := assemble(root, files, prev)
	g, communities, err := writeOutputs(root, files, results, newCache)
	if err != nil {
		return err
	}
	fmt.Printf("updated graph: %d files (%d reparsed, %d reused, %d removed) · %d nodes · %d edges · %d communities → %s\n",
		len(files), stats.parsed, stats.reused, stats.dropped,
		g.NumNodes(), g.NumEdges(), len(communities), filepath.Join(root, "graphify-out"))
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

// cmdExport converts a built graph.json into another format under
// <root>/graphify-out. It reads the committed artifact rather than rebuilding.
func cmdExport(format, root string) error {
	outDir := filepath.Join(root, "graphify-out")
	jsonPath := filepath.Join(outDir, "graph.json")
	if _, err := os.Stat(jsonPath); err != nil {
		return fmt.Errorf("no graph at %s — run `graphify build` first", jsonPath)
	}
	switch format {
	case "graphml":
		out := filepath.Join(outDir, "graph.graphml")
		if err := export.GraphMLFromJSON(jsonPath, out); err != nil {
			return err
		}
		fmt.Println("wrote " + out)
	case "dot":
		out := filepath.Join(outDir, "graph.dot")
		if err := export.DOTFromJSON(jsonPath, out); err != nil {
			return err
		}
		fmt.Println("wrote " + out)
	case "csv":
		nodes := filepath.Join(outDir, "graph.nodes.csv")
		edges := filepath.Join(outDir, "graph.edges.csv")
		if err := export.CSVFromJSON(jsonPath, nodes, edges); err != nil {
			return err
		}
		fmt.Println("wrote " + nodes + " and " + edges)
	case "callflow-html":
		out := filepath.Join(outDir, "graph.callflow.html")
		if err := export.CallflowFromJSON(jsonPath, out); err != nil {
			return err
		}
		fmt.Println("wrote " + out)
	default:
		return fmt.Errorf("unknown export format %q (want: graphml, dot, csv, callflow-html)", format)
	}
	return nil
}

// cmdAffected prints the graph nodes defined in the given files and everything
// that transitively depends on them. With no files it derives them from the
// working tree's uncommitted changes (git diff against HEAD).
func cmdAffected(files []string) error {
	g, err := load()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		files = gitChangedFiles(".")
		if len(files) == 0 {
			return fmt.Errorf("no files given and no uncommitted changes detected (usage: graphify affected [file...])")
		}
		fmt.Printf("changed files (from git): %s\n", strings.Join(files, ", "))
	}
	res := query.Affected(g, files)
	if len(res.Changed) == 0 {
		fmt.Println("no graph nodes are defined in those files")
		return nil
	}
	printNodes := func(title string, ns []query.Node) {
		fmt.Printf("%s (%d):\n", title, len(ns))
		for i := range ns {
			fmt.Printf("  %-40s %s\n", ns[i].Label, locOf(&ns[i]))
		}
	}
	printNodes("changed", res.Changed)
	printNodes("impacted", res.Impacted)
	return nil
}

// cmdDiff compares two graph.json snapshots and prints the nodes and edges added
// or removed between them — the realized delta of a change, complementing the
// predicted blast radius of `affected`. With --json it emits the full delta as
// machine-readable JSON instead of a human summary.
func cmdDiff(args []string) error {
	var asJSON bool
	var paths []string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
			continue
		}
		paths = append(paths, a)
	}
	if len(paths) != 2 {
		return fmt.Errorf("usage: graphify diff <old.json> <new.json> [--json]")
	}
	oldG, err := query.Load(paths[0])
	if err != nil {
		return err
	}
	newG, err := query.Load(paths[1])
	if err != nil {
		return err
	}
	res := query.Diff(oldG, newG)
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	fmt.Println(res.Summary)
	printDiffNodes := func(title string, ns []query.DiffNode) {
		if len(ns) == 0 {
			return
		}
		fmt.Printf("%s (%d):\n", title, len(ns))
		for _, n := range ns {
			fmt.Printf("  %s\n", n.Label)
		}
	}
	printDiffEdges := func(title string, es []query.DiffEdge) {
		if len(es) == 0 {
			return
		}
		fmt.Printf("%s (%d):\n", title, len(es))
		for _, e := range es {
			fmt.Printf("  %s -%s-> %s\n", e.Source, e.Relation, e.Target)
		}
	}
	printDiffNodes("new nodes", res.NewNodes)
	printDiffNodes("removed nodes", res.RemovedNodes)
	printDiffEdges("new edges", res.NewEdges)
	printDiffEdges("removed edges", res.RemovedEdges)
	return nil
}

// cmdValidate checks graph.json for structural problems and exits non-zero if
// any are found, so it can gate CI.
func cmdValidate() error {
	issues, nodes, links, err := query.Validate(defaultGraphPath)
	if err != nil {
		return err
	}
	if len(issues) == 0 {
		fmt.Printf("graph OK: %d nodes · %d edges, no issues\n", nodes, links)
		return nil
	}
	fmt.Printf("graph has %d issue(s) across %d nodes · %d edges:\n", len(issues), nodes, links)
	for _, is := range issues {
		fmt.Println("  - " + is)
	}
	return fmt.Errorf("%d validation issue(s)", len(issues))
}

// gitChangedFiles lists files that differ from HEAD in the working tree, or nil
// if git is unavailable or there are no changes.
func gitChangedFiles(root string) []string {
	out, err := exec.Command("git", "-C", root, "diff", "--name-only", "HEAD").Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
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
  graphify update [path]       rebuild incrementally, re-parsing only changed files
  graphify watch [path]        rebuild incrementally as files change (Ctrl-C to stop)
  graphify hook install [path] install git hooks that update the graph after commits
  graphify query <pattern>     find nodes by name (regex, case-insensitive)
  graphify explain <node>      show a node and its neighbours
  graphify path <from> <to>    shortest dependency path between two nodes
  graphify affected [file...]  nodes defined in changed files + their dependents
  graphify diff <old> <new>    node/edge delta between two graph.json snapshots
  graphify validate            check graph.json for structural problems
  graphify extract <file>      print one file's extracted nodes/edges (debug)
  graphify export <fmt> [path] convert graph.json to graphml, dot, or csv
  graphify version             print version`)
}
