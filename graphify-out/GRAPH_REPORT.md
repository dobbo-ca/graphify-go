# Graph Report - .

## Summary
- 2598 nodes · 8635 edges · 64 communities
- Extraction: 57% EXTRACTED · 42% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `daafb9d2`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `Node` - 299 edges
2. `line()` - 126 edges
3. `Node.ChildByFieldName()` - 101 edges
4. `FileFromBytes()` - 93 edges
5. `Node.Child()` - 79 edges
6. `Node.ChildCount()` - 77 edges
7. `fieldText()` - 72 edges
8. `Result` - 68 edges
9. `resolveSrc()` - 68 edges
10. `Resolve()` - 68 edges

## Surprising Connections (you probably didn't know these)
- `TestExplainErrorSanitizesCandidates()` --calls--> `Explain()`  [INFERRED]
  cmd/graphify/explain_error_test.go → internal/query/query.go  _bridges separate communities_
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

## Communities (64 total)

### Community 0
Cohesion: 0.03
Nodes (350): cmp, github.com/dobbo-ca/gositter/grammars/csharp, github.com/dobbo-ca/gositter/grammars/java, github.com/dobbo-ca/gositter/grammars/kotlin, github.com/dobbo-ca/gositter/grammars/lua, github.com/dobbo-ca/gositter/grammars/scala, bashHasExpansion(), builder.bashCommand() (+342 more)

### Community 1
Cohesion: 0.03
Nodes (186): bufio, bytes, testing.T, TestIsBrokenPipe(), TestQueryExitsZeroOnBrokenPipe(), cmdInstall(), skillPath(), testing.T (+178 more)

### Community 2
Cohesion: 0.02
Nodes (228): cmdExtract(), Entry, extractBash(), TestExtractBash(), testing.T, TestBudgetScalesWithSize(), TestBudgetSetting(), testing.T (+220 more)

### Community 3
Cohesion: 0.03
Nodes (146): 📚 Documentation, 🚀 Features, 🔧 Miscellaneous Tasks, Unreleased, modelOf(), golang.org/x/exp/rand, Cycle, entityLoc() (+138 more)

### Community 4
Cohesion: 0.03
Nodes (128): buildOpts.semanticOpts(), collectNotes(), context.Context, enrich(), loadSemanticCache(), newSemanticBackend(), saveSemanticCache(), semanticOpts (+120 more)

### Community 5
Cohesion: 0.05
Nodes (117): Affected(), AffectedOptions, AffectedResult, Graph.collect(), normalizeSeed(), impacted(), loadJSON(), TestAffectedDefaultFollowsInherits() (+109 more)

### Community 6
Cohesion: 0.05
Nodes (111): TestExplainErrorKeepsAllCandidates(), TestExplainErrorSanitizesCandidates(), testing.T, cmdValidate(), argInt(), argString(), argStrings(), argTokenBudget() (+103 more)

### Community 7
Cohesion: 0.04
Nodes (107): readGraph(), TestBuildCargoFlag(), testing.T, TestParseBuildArgs(), TestAssembleLargeFilesInFlight(), TestAssembleLargeFirst(), testing.T, assemble() (+99 more)

### Community 8
Cohesion: 0.02
Nodes (105): defs.svh, Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs), Beads (from synthesis), Critic: misclassified beads, Critic: missed capabilities, [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges, [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line, [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction (+97 more)

### Community 9
Cohesion: 0.05
Nodes (97): absPhys(), admitFile(), CollectFiles(), CollectFilesReport(), CollectManifests(), entryKind(), hasBuildOutputMarkers(), hasBuildOutputSuffix() (+89 more)

### Community 10
Cohesion: 0.04
Nodes (84): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+76 more)

### Community 11
Cohesion: 0.07
Nodes (78): callTargets(), testing.T, TestJSAliasedExports(), TestJSBareCallFreesOnlyExportMembers(), TestJSBareCallSkipsMethods(), TestJSClassMemberCollisions(), TestJSClassMemberForms(), TestJSConstCallTargets() (+70 more)

### Community 12
Cohesion: 0.07
Nodes (61): appendJSONLink(), jsonLink, toJSONLink(), IntrospectCargo(), packageName(), resolvePath(), testing.T, TestIntrospectCargoHonorsPackageRename() (+53 more)

### Community 13
Cohesion: 0.05
Nodes (60): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+52 more)

### Community 14
Cohesion: 0.07
Nodes (46): collections, idle(), one(), quiet(), Interleaved cold build + no-change update for three tools on the aligned corpora, Wait until other work leaves the CPUs (another session can run a job)., _demo(), _key() (+38 more)

### Community 15
Cohesion: 0.07
Nodes (53): testing.T, TestReadPathList(), TestCmdAskGraphTraversal(), TestCmdAskNegativeBudget(), testing.T, TestCmdExplainPathGraphOverride(), TestCmdExplainPathGraphTraversal(), TestCmdExplainPathSourceFile() (+45 more)

### Community 16
Cohesion: 0.11
Nodes (46): startManifests(), hasScriptCall(), resolveFiles(), TestBashPhpDashEKeepsEdge(), TestBashRunnerModuleFlagNoEdge(), TestBashRunnerNonShellTargets(), TestBashScriptInvocation(), TestBashScriptInvocationFromFunction() (+38 more)

### Community 17
Cohesion: 0.09
Nodes (44): runOut(), testing.T, TestParseBudgetEndToEnd(), TestParseBudgetSameAtAnyGOMAXPROCS(), writeTree(), gitRun(), hookInstalled(), TestHookInstallHonoursHooksPath() (+36 more)

### Community 18
Cohesion: 0.06
Nodes (31): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+23 more)

### Community 19
Cohesion: 0.08
Nodes (24): Benchmark: graphify-go against upstream graphify 0.9.88, curl/curl, curl/curl @ 2874cc6, Files, Graphify-Labs/graphify, Graphify-Labs/graphify @ 6478eb7, kubernetes/kubernetes, kubernetes/kubernetes @ ab4fb775 (+16 more)

### Community 20
Cohesion: 0.21
Nodes (23): extractAll(), TestCSharpBases(), testing.T, TestJavaBaseWithConstructor(), TestJavaGenericQualifiedBase(), TestJavaInheritsImplements(), TestPythonInherits(), TestUnresolvedBaseDrops() (+15 more)

### Community 21
Cohesion: 0.18
Nodes (22): io.Reader, nodeScan, nodeScan.fill(), nodeScan.key(), nodeScan.next(), nodeScan.nodes(), nodeScan.number(), nodeScan.space() (+14 more)

### Community 22
Cohesion: 0.10
Nodes (20): Benchmark re-run: graphify-go 9e292de against upstream graphify 0.9.88, Cells that got worse, Cold build, curl/curl, Files, Graphify-Labs/graphify, kubernetes/kubernetes, Limits (+12 more)

### Community 23
Cohesion: 0.11
Nodes (17): Agent Context Profiles, Agent Instructions, Beads Issue Tracker, Beads Issue Tracker, Non-Interactive Shell Commands, Quick Reference, Quick Reference, Quick Reference (+9 more)

### Community 24
Cohesion: 0.25
Nodes (16): callConfidence(), countCalls(), resolvePy(), TestImportGuidedNonAliased(), TestImportGuidedResolvesAmbiguousAlias(), TestImportGuidedSkipsMemberCall(), TestImportGuidedSkipsSelfEdge(), testing.T (+8 more)

### Community 25
Cohesion: 0.35
Nodes (14): encoding/json.RawMessage, loadRawGraph(), Merge(), rawGraph, loadMerged(), testing.T, TestMergeKeepsBothDirectionsAndRelations(), TestMergeMissingFile() (+6 more)

### Community 26
Cohesion: 0.15
Nodes (13): Effort / sequencing, Explicitly out of scope (don't pull these in), Gap table, Implementation plan (single follow-up session), Key references, Phase 1 — get the data onto the page, Phase 2 — sidebar UI (template), Phase 3 — selection behavior (+5 more)

### Community 27
Cohesion: 0.17
Nodes (10): express, Math, Math.add(), Server, Server.boot(), Server.start(), boot(), Server (+2 more)

### Community 28
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 29
Cohesion: 0.22
Nodes (8): Beads - AI-Native Issue Tracking, Essential Commands, Get Started with Beads, Learn More, Quick Start, What is Beads?, Why Beads?, Working with Issues

### Community 30
Cohesion: 0.32
Nodes (6): add(), double(), boot(), Server, Server.start(), std

### Community 31
Cohesion: 0.33
Nodes (6): helper.h, boot(), run(), Server, Server.start(), stdio.h

### Community 32
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 33
Cohesion: 0.47
Nodes (4): /etc/profile, boot(), add(), double()

### Community 34
Cohesion: 0.53
Nodes (4): add(), boot(), Server, Server.start()

### Community 35
Cohesion: 0.47
Nodes (5): boot(), Int, Server, Server.start(), scala.collection.mutable.Map

### Community 36
Cohesion: 0.50
Nodes (4): crate::util::math, boot(), Server, Server.start()

### Community 37
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), string

### Community 38
Cohesion: 0.50
Nodes (4): Boot(), Server, Server.Start(), System.Text

### Community 39
Cohesion: 0.50
Nodes (4): Server, Server.boot(), Server.start(), kotlin.math.sqrt

### Community 40
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), socket

### Community 41
Cohesion: 0.40
Nodes (5): Business Glossary, Business Glossary, Revenue, Analytics Knowledge Bundle, Analytics Knowledge Bundle

### Community 42
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), Psr\Log\LoggerInterface

### Community 43
Cohesion: 0.60
Nodes (4): add(), Int, MathUtils, MathUtils.double()

### Community 44
Cohesion: 0.50
Nodes (3): Build / remove set, Deliberately skipped (agent-first), v0.5.0 — Agent-first delta

### Community 45
Cohesion: 0.67
Nodes (3): add(), M, M.double()

### Community 46
Cohesion: 0.50
Nodes (4): Tables, Tables, orders, orders

### Community 47
Cohesion: 0.67
Nodes (3): Add(), Calc, Calc.Sum()

### Community 48
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **319 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+314 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **9 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
