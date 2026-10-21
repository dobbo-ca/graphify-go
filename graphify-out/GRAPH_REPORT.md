# Graph Report - .

## Summary
- 2544 nodes · 8403 edges · 59 communities
- Extraction: 57% EXTRACTED · 42% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `01226208`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `Node` - 283 edges
2. `line()` - 126 edges
3. `Node.ChildByFieldName()` - 95 edges
4. `FileFromBytes()` - 93 edges
5. `Node.Child()` - 77 edges
6. `Node.ChildCount()` - 75 edges
7. `fieldText()` - 68 edges
8. `resolveSrc()` - 68 edges
9. `Result` - 67 edges
10. `Resolve()` - 67 edges

## Surprising Connections (you probably didn't know these)
- `TestReadPathList()` --calls--> `readPathList()`  [INFERRED]
  cmd/graphify/affected_stdin_test.go → cmd/graphify/main.go  _bridges separate communities_
- `TestCmdPathAnnotatesHops()` --calls--> `captureStdout()`  [INFERRED]
  cmd/graphify/explain_path_test.go → cmd/graphify/maintenance_test.go  _bridges separate communities_
- `TestCmdPathDirectedContainsReverse()` --calls--> `captureStdout()`  [INFERRED]
  cmd/graphify/explain_path_test.go → cmd/graphify/maintenance_test.go  _bridges separate communities_
- `cmdAffected()` --calls--> `Affected()`  [INFERRED]
  cmd/graphify/main.go → internal/query/affected.go  _bridges separate communities_
- `cmdAsk()` --calls--> `Ask()`  [INFERRED]
  cmd/graphify/main.go → internal/query/ask.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (59 total)

### Community 0
Cohesion: 0.03
Nodes (356): cmp, github.com/dobbo-ca/gositter/grammars/bash, github.com/dobbo-ca/gositter/grammars/c, github.com/dobbo-ca/gositter/grammars/cpp, github.com/dobbo-ca/gositter/grammars/csharp, github.com/dobbo-ca/gositter/grammars/golang, github.com/dobbo-ca/gositter/grammars/hcl, github.com/dobbo-ca/gositter/grammars/java (+348 more)

### Community 1
Cohesion: 0.02
Nodes (249): 📚 Documentation, 🚀 Features, 🔧 Miscellaneous Tasks, Unreleased, testing.T, TestReadPathList(), encoding/json, github.com/dobbo-ca/gositter/grammars/python (+241 more)

### Community 2
Cohesion: 0.02
Nodes (221): anyNodeClustered(), TestBuildNoCluster(), testing.T, github.com/dobbo-ca/gositter, github.com/dobbo-ca/gositter/grammars/javascript, github.com/dobbo-ca/gositter/grammars/tsx, github.com/dobbo-ca/gositter/grammars/typescript, extractBash() (+213 more)

### Community 3
Cohesion: 0.02
Nodes (235): TestExplainErrorKeepsAllCandidates(), TestExplainErrorSanitizesCandidates(), testing.T, gitHead(), argInt(), argString(), argStrings(), argTokenBudget() (+227 more)

### Community 4
Cohesion: 0.03
Nodes (157): bufio, bytes, TestCmdAskGraphTraversal(), TestCmdAskNegativeBudget(), testing.T, readGraph(), TestBuildCargoFlag(), testing.T (+149 more)

### Community 5
Cohesion: 0.03
Nodes (126): testing.T, TestIsBrokenPipe(), TestQueryExitsZeroOnBrokenPipe(), runOut(), testing.T, TestParseBudgetEndToEnd(), TestParseBudgetSameAtAnyGOMAXPROCS(), writeTree() (+118 more)

### Community 6
Cohesion: 0.04
Nodes (118): golang.org/x/text/cases, golang.org/x/text/unicode/norm, hasScriptCall(), resolveFiles(), TestBashPhpDashEKeepsEdge(), TestBashRunnerModuleFlagNoEdge(), TestBashRunnerNonShellTargets(), TestBashScriptInvocation() (+110 more)

### Community 7
Cohesion: 0.03
Nodes (105): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+97 more)

### Community 8
Cohesion: 0.05
Nodes (108): absPhys(), admitFile(), CollectFiles(), CollectFilesReport(), CollectManifests(), entryKind(), genericKeywordHit(), hasBuildOutputMarkers() (+100 more)

### Community 9
Cohesion: 0.07
Nodes (78): callTargets(), testing.T, TestJSAliasedExports(), TestJSBareCallFreesOnlyExportMembers(), TestJSBareCallSkipsMethods(), TestJSClassMemberCollisions(), TestJSClassMemberForms(), TestJSConstCallTargets() (+70 more)

### Community 10
Cohesion: 0.05
Nodes (74): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+66 more)

### Community 11
Cohesion: 0.07
Nodes (66): buildOpts.semanticOpts(), collectNotes(), context.Context, enrich(), loadSemanticCache(), newSemanticBackend(), saveSemanticCache(), semanticOpts (+58 more)

### Community 12
Cohesion: 0.03
Nodes (60): Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs), Beads (from synthesis), Critic: misclassified beads, Critic: missed capabilities, [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges, [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line, [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction, [critic] p2 `build` `core-cli` — Cap and group explain's connection list instead of dumping every neighbour (+52 more)

### Community 13
Cohesion: 0.04
Nodes (45): defs.svh, Beads, [critic] p0 `build` `core-cli`: Skip nested git worktrees and dot-dir worktrees/ in detect, [critic] p1 `build` `core-cli`: Redact secrets in Terraform attribute values that fall through to raw source text, [critic] p1 `build` `core-cli`: Route .h headers containing C++ to the C++ extractor, [critic] p3 `build` `core-cli`: Record calls to plainly imported external Python modules, Critic verdict, Summary (+37 more)

### Community 14
Cohesion: 0.15
Nodes (33): github.com/BurntSushi/toml, crate, IntrospectCargo(), memberManifestPaths(), packageName(), resolvePath(), shallowestCargo(), testing.T (+25 more)

### Community 15
Cohesion: 0.06
Nodes (31): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+23 more)

### Community 16
Cohesion: 0.18
Nodes (23): io.Reader, nodeScan, nodeScan.fill(), nodeScan.key(), nodeScan.next(), nodeScan.nodes(), nodeScan.number(), nodeScan.space() (+15 more)

### Community 17
Cohesion: 0.21
Nodes (23): extractAll(), TestCSharpBases(), testing.T, TestJavaBaseWithConstructor(), TestJavaGenericQualifiedBase(), TestJavaInheritsImplements(), TestPythonInherits(), TestUnresolvedBaseDrops() (+15 more)

### Community 18
Cohesion: 0.10
Nodes (20): Benchmark re-run: graphify-go 9e292de against upstream graphify 0.9.88, Cells that got worse, Cold build, curl/curl, Files, Graphify-Labs/graphify, kubernetes/kubernetes, Limits (+12 more)

### Community 19
Cohesion: 0.11
Nodes (17): Agent Context Profiles, Agent Instructions, Beads Issue Tracker, Beads Issue Tracker, Non-Interactive Shell Commands, Quick Reference, Quick Reference, Quick Reference (+9 more)

### Community 20
Cohesion: 0.15
Nodes (13): Effort / sequencing, Explicitly out of scope (don't pull these in), Gap table, Implementation plan (single follow-up session), Key references, Phase 1 — get the data onto the page, Phase 2 — sidebar UI (template), Phase 3 — selection behavior (+5 more)

### Community 21
Cohesion: 0.17
Nodes (10): express, Math, Math.add(), Server, Server.boot(), Server.start(), boot(), Server (+2 more)

### Community 22
Cohesion: 0.52
Nodes (11): Merge(), loadMerged(), testing.T, TestMergeKeepsBothDirectionsAndRelations(), TestMergeMissingFile(), TestMergePreservesNodeAttributes(), TestMergePreservesProvenanceAndHyperedges(), TestMergeRejectsCorruptInput() (+3 more)

### Community 23
Cohesion: 0.29
Nodes (10): Graph.UnclassifiedSummary(), testing.T, TestUnclassifiedSummaryEmptyWhenNoneSkipped(), TestUnclassifiedSummaryRanksAndCaps(), TestUnclassifiedSummarySanitizesExtensions(), Validate(), testing.T, TestValidateClean() (+2 more)

### Community 24
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 25
Cohesion: 0.22
Nodes (8): Beads - AI-Native Issue Tracking, Essential Commands, Get Started with Beads, Learn More, Quick Start, What is Beads?, Why Beads?, Working with Issues

### Community 26
Cohesion: 0.32
Nodes (6): add(), double(), boot(), Server, Server.start(), std

### Community 27
Cohesion: 0.33
Nodes (6): helper.h, boot(), run(), Server, Server.start(), stdio.h

### Community 28
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 29
Cohesion: 0.47
Nodes (4): /etc/profile, boot(), add(), double()

### Community 30
Cohesion: 0.47
Nodes (5): boot(), Int, Server, Server.start(), scala.collection.mutable.Map

### Community 31
Cohesion: 0.50
Nodes (4): crate::util::math, boot(), Server, Server.start()

### Community 32
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), string

### Community 33
Cohesion: 0.50
Nodes (4): Boot(), Server, Server.Start(), System.Text

### Community 34
Cohesion: 0.50
Nodes (4): Server, Server.boot(), Server.start(), kotlin.math.sqrt

### Community 35
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), socket

### Community 36
Cohesion: 0.40
Nodes (5): Business Glossary, Business Glossary, Revenue, Analytics Knowledge Bundle, Analytics Knowledge Bundle

### Community 37
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), Psr\Log\LoggerInterface

### Community 38
Cohesion: 0.60
Nodes (4): add(), Int, MathUtils, MathUtils.double()

### Community 39
Cohesion: 0.50
Nodes (3): Build / remove set, Deliberately skipped (agent-first), v0.5.0 — Agent-first delta

### Community 40
Cohesion: 0.67
Nodes (3): add(), M, M.double()

### Community 41
Cohesion: 0.50
Nodes (4): Tables, Tables, orders, orders

### Community 42
Cohesion: 0.67
Nodes (3): Add(), Calc, Calc.Sum()

### Community 43
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **316 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+311 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **9 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
