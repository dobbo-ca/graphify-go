// Command graphify builds a knowledge graph from a source tree and answers
// queries against it. Build writes graphify-out/{graph.json,GRAPH_REPORT.md};
// query/path/explain read graph.json so an agent can navigate the codebase
// without grepping.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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
	"github.com/dobbo-ca/graphify-go/internal/security"
)

const defaultGraphPath = "graphify-out/graph.json"

// Build metadata, injected via -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	handleBrokenPipe()
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
		err = cmdBuild(os.Args[2:])
	case "update":
		err = cmdUpdate(os.Args[2:])
	case "watch":
		err = cmdWatch(arg(2, "."))
	case "hook":
		err = cmdHook(os.Args[2:])
	case "query":
		err = cmdQuery(mustArg(2, "query <pattern>"))
	case "ask":
		err = cmdAsk(os.Args[2:])
	case "explain":
		err = cmdExplain(os.Args[2:])
	case "path":
		err = cmdPath(os.Args[2:])
	case "extract":
		err = cmdExtract(mustArg(2, "extract <file>"))
	case "export":
		err = cmdExport(mustArg(2, "export <graphml|dot|csv|okf> [path]"), arg(3, "."))
	case "affected":
		err = cmdAffected(os.Args[2:])
	case "diff":
		err = cmdDiff(os.Args[2:])
	case "merge-driver":
		err = cmdMergeDriver(os.Args[2:])
	case "validate":
		err = cmdValidate()
	case "serve":
		err = cmdServe(defaultGraphPath)
	case "-h", "--help", "help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		if isBrokenPipe(err) {
			os.Exit(0) // a downstream reader closed the pipe early — not a failure
		}
		fmt.Fprintln(os.Stderr, "graphify:", err)
		os.Exit(1)
	}
}

// assembleStats reports how an assemble run used the cache.
type assembleStats struct{ parsed, reused, dropped int }

// assemble produces one extraction result per file, in files order (so graph
// output stays deterministic), re-parsing only files whose content hash differs
// from prev. Files unchanged since prev reuse their cached result, skipping the
// expensive tree-sitter parse. The stat sidecar lets it skip even reading and
// hashing files whose size+mtime are unchanged. It also returns the cache and
// stat index to persist and counts for the summary line. Files that fail to read
// are skipped with a warning.
func assemble(root string, files []string, prev cache.Cache, prevStat cache.StatIndex) ([]extract.Result, cache.Cache, cache.StatIndex, assembleStats) {
	type slot struct {
		entry  cache.Entry
		stat   cache.StatEntry
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
				key := filepath.ToSlash(files[i])
				ps, psOK := prevStat[key]
				h, statEntry, src, ok := cache.HashFile(filepath.Join(root, files[i]), ps, psOK)
				if !ok {
					fmt.Fprintf(os.Stderr, "  warning: skipped %s\n", files[i])
					continue
				}
				if e, ok := prev[key]; ok && e.Hash == h {
					slots[i] = slot{entry: e, stat: statEntry, ok: true, reused: true}
					continue
				}
				// Not a cache hit, so we must parse. If the stat fastpath skipped
				// the read (src == nil), read now — the cached result is absent or
				// stale and the bytes are needed to re-parse.
				if src == nil {
					b, err := os.ReadFile(filepath.Join(root, files[i]))
					if err != nil {
						fmt.Fprintf(os.Stderr, "  warning: skipped %s (%v)\n", files[i], err)
						continue
					}
					src = b
				}
				res := extract.FileFromBytes(files[i], src)
				slots[i] = slot{entry: cache.Entry{Hash: h, Result: res}, stat: statEntry, ok: true}
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
	newStat := make(cache.StatIndex, len(files))
	var stats assembleStats
	for i, s := range slots {
		if !s.ok {
			continue
		}
		key := filepath.ToSlash(files[i])
		results = append(results, s.entry.Result)
		newCache[key] = s.entry
		// Skip files HashFile could not stat (empty hash) so the index keeps
		// only entries that can match on a later run.
		if s.stat.Hash != "" {
			newStat[key] = s.stat
		}
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
	return results, newCache, newStat, stats
}

// writeOutputs resolves the per-file results into a graph and writes graph.json,
// GRAPH_REPORT.md, and the incremental cache and stat sidecar under
// <root>/graphify-out. When sem.enabled, an additive LLM enrichment pass runs
// between resolve and graph-build so communities reflect concepts; it never
// alters the deterministic core.
func writeOutputs(root string, files []string, results []extract.Result, newCache cache.Cache, newStat cache.StatIndex, sem semanticOpts, force, noCluster bool) (*model.Graph, map[int][]string, error) {
	ext := extract.Resolve(results, files)
	if sem.enabled {
		var err error
		ext, err = enrich(root, ext, sem)
		if err != nil {
			return nil, nil, err
		}
	}
	g := graph.Build(ext)
	// --no-cluster writes the raw extraction: skip Louvain community detection so
	// every node lands with no community assignment (mirrors upstream update
	// --no-cluster). An empty map flows through NodeCommunity as "no community".
	communities := map[int][]string{}
	if !noCluster {
		communities = cluster.Cluster(g)
	}
	commit := gitHead(root)

	outDir := filepath.Join(root, "graphify-out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, nil, err
	}
	// Anti-shrink guard: allow the write when the graph grew, or when every node
	// that would be lost belongs to a source that was rebuilt/deleted this run;
	// refuse (and keep the larger graph.json) only when a lost node's source file
	// was skipped this run — the silent loss (#479) this guards. --force /
	// GRAPHIFY_FORCE overrides. The skipped set gives CheckShrink the legitimate-
	// shrink carve-out the crude ToJSON node-count guard lacks (mirrors upstream
	// watch._check_shrink); on approval ToJSON runs with force=true so it does not
	// re-apply the crude guard and refuse a legitimate refactor.
	force = force || envForce()
	graphPath := filepath.Join(outDir, "graph.json")
	// Run the anti-shrink guard FIRST, before writing any output. On a refused
	// shrink (and no --force) every output is aborted so graphify-out stays
	// consistent: keeping the old graph.json while rewriting GRAPH_REPORT.md, the
	// cache, and the stat sidecar from the rejected smaller graph would leave the
	// report/cache describing a graph that graph.json does not match (mirrors
	// upstream watch, where a refused _check_shrink returns before any write).
	if err := export.CheckShrink(graphPath, g, skippedFiles(files, newCache), root, force); err != nil {
		// A shrink/unverifiable refusal is a fail-safe, not a build failure:
		// warn and leave graphify-out untouched rather than crashing the build.
		if errors.Is(err, export.ErrGraphShrink) || errors.Is(err, export.ErrGraphUnverifiable) {
			fmt.Fprintf(os.Stderr, "[graphify] WARNING: %v\n", err)
			return g, communities, nil
		}
		return nil, nil, err
	}
	// Shrink allowed (growth, a legitimate carve-out, or force set): write every
	// output. ToJSON runs with force=true so it does not re-apply its own crude
	// node-count guard and refuse a legitimate refactor CheckShrink already OK'd.
	if err := export.ToJSON(g, communities, graphPath, commit, true); err != nil {
		return nil, nil, err
	}
	md := report.Generate(g, communities, root, commit)
	if err := os.WriteFile(filepath.Join(outDir, "GRAPH_REPORT.md"), []byte(md), 0o644); err != nil {
		return nil, nil, err
	}
	if err := cache.Save(filepath.Join(outDir, cache.FileName), newCache); err != nil {
		return nil, nil, err
	}
	if err := cache.SaveStat(filepath.Join(outDir, cache.StatFileName), newStat); err != nil {
		return nil, nil, err
	}
	return g, communities, nil
}

// envForce reports whether GRAPHIFY_FORCE requests bypassing the anti-shrink
// guard, matching upstream's accepted values (1/true/yes, case-insensitive).
func envForce() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GRAPHIFY_FORCE"))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// skippedFiles returns the current source files that failed to process this run —
// present in files but absent from newCache — keyed by slash path to match node
// source_file. These are the files whose nodes vanishing would be an unexplained
// (silent) loss; a shrink whose lost nodes all come from rebuilt or deleted
// sources is legitimate and never lands here.
func skippedFiles(files []string, newCache cache.Cache) map[string]bool {
	skipped := map[string]bool{}
	for _, f := range files {
		key := filepath.ToSlash(f)
		if _, ok := newCache[key]; !ok {
			skipped[key] = true
		}
	}
	return skipped
}

// warnWalkReport prints a non-fatal stderr warning for each directory the detect
// walk could not read (its files are missing from this run) and a note counting
// files seen but skipped for lacking an extractor. Silent otherwise. Mirrors
// upstream detect.py's walk_errors / unclassified surfacing.
func warnWalkReport(rep detect.WalkReport) {
	for _, we := range rep.WalkErrors {
		fmt.Fprintf(os.Stderr, "[graphify] WARNING: could not scan %s; its files are missing from this run's enumeration.\n", we)
	}
	if rep.Skipped > 0 {
		fmt.Fprintf(os.Stderr, "[graphify] note: %d file(s) skipped (no extractor for their type).\n", rep.Skipped)
	}
}

func cmdBuild(args []string) error {
	opts, err := parseBuildOpts(args)
	if err != nil {
		return err
	}
	root := opts.root
	rep, err := detect.CollectFilesReport(root)
	if err != nil {
		return err
	}
	files := rep.Files
	warnWalkReport(rep)
	if len(files) == 0 {
		return fmt.Errorf("no supported source files found under %s", root)
	}
	results, newCache, newStat, _ := assemble(root, files, nil, nil)
	if opts.cargo {
		results, err = withCargo(root, results)
		if err != nil {
			return err
		}
	}
	if !opts.noManifests {
		results, err = withManifests(root, results)
		if err != nil {
			return err
		}
	}
	g, communities, err := writeOutputs(root, files, results, newCache, newStat, opts.semanticOpts(), opts.force, opts.noCluster)
	if err != nil {
		return err
	}
	fmt.Printf("built graph: %d files · %d nodes · %d edges · %d communities → %s\n",
		len(files), g.NumNodes(), g.NumEdges(), len(communities), filepath.Join(root, "graphify-out"))
	return nil
}

// buildOpts holds the parsed build/update arguments.
type buildOpts struct {
	root        string
	cargo       bool
	noManifests bool   // --no-manifests: skip the default package-manifest pass
	semantic    bool   // --semantic: run the opt-in LLM enrichment pass
	backend     string // --backend: which semantic backend (e.g. "bedrock")
	force       bool   // --force: overwrite graph.json even when the rebuild has fewer nodes
	noCluster   bool   // --no-cluster: skip community detection, write the raw extraction
}

// semanticOpts projects the build options onto the enrichment-stage options.
func (o buildOpts) semanticOpts() semanticOpts {
	return semanticOpts{enabled: o.semantic, backend: o.backend}
}

// parseBuildArgs splits build/update arguments into the target path (default "."),
// whether the opt-in --cargo crate-dependency pass was requested, whether
// --no-manifests disabled the default package-manifest pass, whether --force
// was given to bypass the anti-shrink guard, and whether --no-cluster skipped
// community detection. It exists for callers that do not support semantic
// enrichment; build uses parseBuildOpts.
func parseBuildArgs(args []string) (root string, cargo, noManifests, force, noCluster bool) {
	opts, _ := parseBuildOpts(args)
	return opts.root, opts.cargo, opts.noManifests, opts.force, opts.noCluster
}

// parseBuildOpts parses the full build flag set: the target path (default "."),
// --cargo, --semantic, and --backend <name> (or --backend=<name>). --semantic
// requires an explicit --backend; omitting it is an error so the costly LLM pass
// is never run against an unspecified provider.
func parseBuildOpts(args []string) (buildOpts, error) {
	opts := buildOpts{root: "."}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--cargo":
			opts.cargo = true
		case a == "--no-manifests":
			opts.noManifests = true
		case a == "--force":
			opts.force = true
		case a == "--no-cluster":
			opts.noCluster = true
		case a == "--semantic":
			opts.semantic = true
		case a == "--backend":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--backend requires a value (e.g. --backend bedrock)")
			}
			opts.backend = args[i+1]
			i++
		case strings.HasPrefix(a, "--backend="):
			opts.backend = strings.TrimPrefix(a, "--backend=")
		default:
			opts.root = a
		}
	}
	if opts.semantic && opts.backend == "" {
		return opts, fmt.Errorf("--semantic requires an explicit --backend (e.g. --backend bedrock)")
	}
	return opts, nil
}

// withCargo runs the Cargo manifest introspection pass and appends its crate
// nodes/edges to the per-file results so they flow through the normal resolve and
// graph-build path. It is opt-in (--cargo) so it never touches non-Rust corpora.
func withCargo(root string, results []extract.Result) ([]extract.Result, error) {
	res, err := extract.IntrospectCargo(root)
	if err != nil {
		return nil, err
	}
	return append(results, res), nil
}

// withManifests runs the deterministic package-manifest pass (pyproject.toml,
// go.mod, pom.xml) and appends its package nodes/depends_on edges to the per-file
// results. It runs by default (mirroring upstream manifest_ingest) because it is
// cheap and deterministic; --no-manifests disables it. It never errors on a
// malformed manifest — only on a walk failure — so it cannot break a build.
func withManifests(root string, results []extract.Result) ([]extract.Result, error) {
	res, err := extract.IntrospectManifests(root)
	if err != nil {
		return nil, err
	}
	return append(results, res), nil
}

// cmdUpdate rebuilds the graph incrementally: it re-parses only files whose
// content changed since the last build/update, reusing cached results for the
// rest, then resolves and writes the same outputs as build. With no existing
// cache it transparently degrades to a full build.
func cmdUpdate(args []string) error {
	root, cargo, noManifests, force, noCluster := parseBuildArgs(args)
	rep, err := detect.CollectFilesReport(root)
	if err != nil {
		return err
	}
	files := rep.Files
	warnWalkReport(rep)
	if len(files) == 0 {
		return fmt.Errorf("no supported source files found under %s", root)
	}
	prev := cache.Load(filepath.Join(root, "graphify-out", cache.FileName))
	prevStat := cache.LoadStat(filepath.Join(root, "graphify-out", cache.StatFileName))
	results, newCache, newStat, stats := assemble(root, files, prev, prevStat)
	if cargo {
		results, err = withCargo(root, results)
		if err != nil {
			return err
		}
	}
	if !noManifests {
		results, err = withManifests(root, results)
		if err != nil {
			return err
		}
	}
	g, communities, err := writeOutputs(root, files, results, newCache, newStat, semanticOpts{}, force, noCluster)
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

// cmdAsk answers a natural-language question against the graph using TF-IDF
// retrieval plus a bounded BFS/DFS traversal, printing the relevant subgraph as
// a token-budgeted text block. Unlike `query` (regex name match), this is the
// agent-native one-shot retrieval primitive.
func cmdAsk(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf(`usage: graphify ask "<question>" [--dfs] [--budget N] [--graph path]`)
	}
	question := args[0]
	dfs := false
	budget := 2000
	graphPath := defaultGraphPath
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch {
		case rest[i] == "--dfs":
			dfs = true
		case rest[i] == "--budget" && i+1 < len(rest):
			n, err := strconv.Atoi(rest[i+1])
			if err != nil {
				return fmt.Errorf("--budget must be an integer")
			}
			if n <= 0 {
				return fmt.Errorf("--budget must be a positive integer")
			}
			budget = n
			i++
		case strings.HasPrefix(rest[i], "--budget="):
			n, err := strconv.Atoi(strings.TrimPrefix(rest[i], "--budget="))
			if err != nil {
				return fmt.Errorf("--budget must be an integer")
			}
			if n <= 0 {
				return fmt.Errorf("--budget must be a positive integer")
			}
			budget = n
		case rest[i] == "--graph" && i+1 < len(rest):
			graphPath = rest[i+1]
			i++
		case strings.HasPrefix(rest[i], "--graph="):
			graphPath = strings.TrimPrefix(rest[i], "--graph=")
		}
	}
	if graphPath != defaultGraphPath {
		safe, err := safeGraphPath(graphPath)
		if err != nil {
			return err
		}
		graphPath = safe
	}
	g, err := query.Load(graphPath)
	if err != nil {
		return err
	}
	fmt.Println(query.Ask(g, question, dfs, 2, budget))
	return nil
}

// safeGraphPath contains a user-supplied --graph path: it must resolve inside a
// graphify-out directory under the current working directory, preventing path
// traversal that would read arbitrary on-disk JSON (config/credential files).
func safeGraphPath(path string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	base := filepath.Join(cwd, "graphify-out")
	return security.ValidateGraphPath(path, base)
}

func cmdExplain(args []string) error {
	positionals, graphPath := parseGraphFlag(args)
	if len(positionals) != 1 {
		return fmt.Errorf(`usage: graphify explain <node> [--graph path]`)
	}
	g, err := loadGraphAt(graphPath)
	if err != nil {
		return err
	}
	ex, err := query.Explain(g, positionals[0])
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

func cmdPath(args []string) error {
	positionals, graphPath := parseGraphFlag(args)
	if len(positionals) != 2 {
		return fmt.Errorf("usage: graphify path <from> <to> [--graph path]")
	}
	g, err := loadGraphAt(graphPath)
	if err != nil {
		return err
	}
	nodes, err := query.Path(g, positionals[0], positionals[1])
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

// parseGraphFlag splits args into positionals and an optional --graph <path>
// (or --graph=<path>) override, defaulting to defaultGraphPath. It lets explain
// and path accept an alternate graph.json the way ask and diff already do.
func parseGraphFlag(args []string) (positionals []string, graphPath string) {
	graphPath = defaultGraphPath
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--graph" && i+1 < len(args):
			graphPath = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--graph="):
			graphPath = strings.TrimPrefix(args[i], "--graph=")
		default:
			positionals = append(positionals, args[i])
		}
	}
	return positionals, graphPath
}

// loadGraphAt loads a graph from a user-supplied --graph path, applying the same
// containment guard as ask/diff when it differs from the default so an alternate
// path cannot escape graphify-out to read arbitrary on-disk JSON.
func loadGraphAt(graphPath string) (*query.Graph, error) {
	if graphPath != defaultGraphPath {
		safe, err := safeGraphPath(graphPath)
		if err != nil {
			return nil, err
		}
		graphPath = safe
	}
	return query.Load(graphPath)
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
	case "okf":
		bundle := filepath.Join(outDir, "okf")
		if err := export.OKFFromJSON(jsonPath, bundle); err != nil {
			return err
		}
		fmt.Println("wrote OKF bundle to " + bundle)
	default:
		return fmt.Errorf("unknown export format %q (want: graphml, dot, csv, okf)", format)
	}
	return nil
}

// cmdAffected prints the graph nodes defined in the given files and everything
// that transitively depends on them. With no files it derives them from the
// working tree's uncommitted changes (git diff against HEAD). --depth N bounds
// the reverse-dependency walk (default: unbounded); --relation R (repeatable)
// restricts which edge kinds count as "depends-on".
func cmdAffected(args []string) error {
	g, err := load()
	if err != nil {
		return err
	}
	var files, relations []string
	depth := 0 // unbounded — preserve the historical whole-closure behaviour
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--depth" && i+1 < len(args):
			depth, err = strconv.Atoi(args[i+1])
			if err != nil {
				return fmt.Errorf("--depth must be an integer")
			}
			i++
		case strings.HasPrefix(a, "--depth="):
			depth, err = strconv.Atoi(strings.TrimPrefix(a, "--depth="))
			if err != nil {
				return fmt.Errorf("--depth must be an integer")
			}
		case a == "--relation" && i+1 < len(args):
			relations = append(relations, args[i+1])
			i++
		case strings.HasPrefix(a, "--relation="):
			relations = append(relations, strings.TrimPrefix(a, "--relation="))
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		files = gitChangedFiles(".")
		if len(files) == 0 {
			return fmt.Errorf("no files given and no uncommitted changes detected (usage: graphify affected [file...] [--depth N] [--relation R])")
		}
		fmt.Printf("changed files (from git): %s\n", strings.Join(files, ", "))
	}
	res := query.Affected(g, files, query.AffectedOptions{Depth: depth, Relations: relations})
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
	for i := range paths {
		safe, err := safeGraphPath(paths[i])
		if err != nil {
			return err
		}
		paths[i] = safe
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

// cmdMergeDriver is the git merge driver for a committed graph.json. Wired via
// .gitattributes + a merge driver in .git/config as `graphify merge-driver %O
// %A %B`, it writes the node/edge union of the two sides back to %A (current).
// It ignores the base (%O) — a set union needs only the two branch tips — and
// returns an error on corrupt or oversized input so git surfaces a conflict
// instead of accepting a poisoned merge.
func cmdMergeDriver(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: graphify merge-driver <base> <current> <other>")
	}
	current, other := args[1], args[2]
	return query.Merge(current, other)
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
  graphify build [path] [--cargo] [--force] [--no-cluster]   build graph.json + report under <path>/graphify-out (--cargo adds Rust crate-dependency edges; --force overwrites even if the rebuild has fewer nodes, also GRAPHIFY_FORCE=1; --no-cluster skips community detection)
  graphify update [path] [--cargo] [--force] [--no-cluster]  rebuild incrementally, re-parsing only changed files
  graphify watch [path]        rebuild incrementally as files change (Ctrl-C to stop)
  graphify hook <install|uninstall|status> [path]  manage git hooks that update the graph after commits
  graphify query <pattern>     find nodes by name (regex, case-insensitive)
  graphify ask "<question>"    NL retrieval: relevant subgraph as text [--dfs --budget N --graph path]
  graphify explain <node>      show a node and its neighbours [--graph path]
  graphify path <from> <to>    shortest dependency path between two nodes [--graph path]
  graphify affected [file...]  nodes defined in changed files + their dependents [--depth N --relation R]
  graphify diff <old> <new>    node/edge delta between two graph.json snapshots
  graphify merge-driver <base> <current> <other>  git merge driver: union-merge two graph.json files
  graphify validate            check graph.json for structural problems
  graphify serve               MCP stdio server: load graph.json once, answer many queries
  graphify extract <file>      print one file's extracted nodes/edges (debug)
  graphify export <fmt> [path] convert graph.json to graphml, dot, csv, or okf
  graphify version             print version`)
}
