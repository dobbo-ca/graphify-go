# Graph Report - .

## Summary
- 2343 nodes · 7815 edges · 64 communities
- Extraction: 57% EXTRACTED · 42% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `e4b99d17`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `Node` - 272 edges
2. `line()` - 123 edges
3. `Node.ChildByFieldName()` - 91 edges
4. `FileFromBytes()` - 89 edges
5. `Node.Child()` - 75 edges
6. `Node.ChildCount()` - 73 edges
7. `Result` - 66 edges
8. `Resolve()` - 64 edges
9. `fieldText()` - 63 edges
10. `resolveSrc()` - 60 edges

## Surprising Connections (you probably didn't know these)
- `TestCmdPathAnnotatesHops()` --calls--> `captureStdout()`  [INFERRED]
  cmd/graphify/explain_path_test.go → cmd/graphify/maintenance_test.go  _bridges separate communities_
- `TestCmdPathDirectedContainsReverse()` --calls--> `captureStdout()`  [INFERRED]
  cmd/graphify/explain_path_test.go → cmd/graphify/maintenance_test.go  _bridges separate communities_
- `cmdAsk()` --calls--> `Ask()`  [INFERRED]
  cmd/graphify/main.go → internal/query/ask.go  _bridges separate communities_
- `cmdAsk()` --calls--> `Load()`  [INFERRED]
  cmd/graphify/main.go → internal/query/query.go  _bridges separate communities_
- `cmdBuild()` --calls--> `CollectFilesReport()`  [INFERRED]
  cmd/graphify/main.go → internal/detect/detect.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (64 total)

### Community 0
Cohesion: 0.03
Nodes (338): github.com/dobbo-ca/gositter, github.com/dobbo-ca/gositter/grammars/c, github.com/dobbo-ca/gositter/grammars/cpp, github.com/dobbo-ca/gositter/grammars/golang, github.com/dobbo-ca/gositter/grammars/java, github.com/dobbo-ca/gositter/grammars/javascript, github.com/dobbo-ca/gositter/grammars/julia, github.com/dobbo-ca/gositter/grammars/kotlin (+330 more)

### Community 1
Cohesion: 0.03
Nodes (154): bufio, bytes, testing.T, TestIsBrokenPipe(), TestQueryExitsZeroOnBrokenPipe(), cmdInstall(), skillPath(), testing.T (+146 more)

### Community 2
Cohesion: 0.02
Nodes (211): cmdExtract(), Entry, extractBash(), TestExtractBash(), testing.T, TestBudgetScalesWithSize(), TestBudgetSetting(), testing.T (+203 more)

### Community 3
Cohesion: 0.04
Nodes (166): TestExplainErrorKeepsAllCandidates(), TestExplainErrorSanitizesCandidates(), testing.T, cmdQuery(), cmdValidate(), load(), argInt(), argString() (+158 more)

### Community 4
Cohesion: 0.03
Nodes (144): 📚 Documentation, 🚀 Features, 🔧 Miscellaneous Tasks, Unreleased, golang.org/x/exp/rand, Cycle, entityLoc(), GodNode (+136 more)

### Community 5
Cohesion: 0.03
Nodes (111): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+103 more)

### Community 6
Cohesion: 0.04
Nodes (103): readGraph(), TestBuildCargoFlag(), testing.T, TestParseBuildArgs(), TestAssembleLargeFilesInFlight(), TestAssembleLargeFirst(), testing.T, assemble() (+95 more)

### Community 7
Cohesion: 0.05
Nodes (101): appendJSONLink(), jsonLink, toJSONLink(), streamGraph(), IntrospectCargo(), memberManifestPaths(), packageName(), resolvePath() (+93 more)

### Community 8
Cohesion: 0.06
Nodes (71): hasScriptCall(), resolveFiles(), TestBashPhpDashEKeepsEdge(), TestBashRunnerModuleFlagNoEdge(), TestBashRunnerNonShellTargets(), TestBashScriptInvocation(), TestBashScriptInvocationFromFunction(), TestBashScriptInvocationPrunesMissing() (+63 more)

### Community 9
Cohesion: 0.06
Nodes (67): Imp, ImportAlias, TypeRef, TestGoImportTargetsMatchPerImportFilter(), testing.T, jsLike(), jsMods, jsMods.aliases() (+59 more)

### Community 10
Cohesion: 0.05
Nodes (69): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+61 more)

### Community 11
Cohesion: 0.08
Nodes (69): callTargets(), testing.T, TestJSAliasedExports(), TestJSBareCallSkipsMethods(), TestJSConstCallTargets(), TestJSConstClosureCalls(), TestJSConstInitializerNoRecursion(), TestJSConstKeepsCallBinding() (+61 more)

### Community 12
Cohesion: 0.03
Nodes (60): Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs), Beads (from synthesis), Critic: misclassified beads, Critic: missed capabilities, [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges, [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line, [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction, [critic] p2 `build` `core-cli` — Cap and group explain's connection list instead of dumping every neighbour (+52 more)

### Community 13
Cohesion: 0.09
Nodes (46): cmdExport(), encoding/csv, encoding/xml, graphAttrs, jsonGraph, jsonNode, CSVFromJSON(), DOTFromJSON() (+38 more)

### Community 14
Cohesion: 0.09
Nodes (43): runOut(), testing.T, TestParseBudgetEndToEnd(), TestParseBudgetSameAtAnyGOMAXPROCS(), writeTree(), gitRun(), hookInstalled(), TestHookInstallHonoursHooksPath() (+35 more)

### Community 15
Cohesion: 0.10
Nodes (43): CollectFiles(), CollectFilesReport(), CollectManifests(), inMemoryDir(), io/fs.DirEntry, isNestedWorktree(), IsSensitive(), ShebangExt() (+35 more)

### Community 16
Cohesion: 0.05
Nodes (38): Beads, [critic] p0 `build` `core-cli`: Skip nested git worktrees and dot-dir worktrees/ in detect, [critic] p1 `build` `core-cli`: Redact secrets in Terraform attribute values that fall through to raw source text, [critic] p1 `build` `core-cli`: Route .h headers containing C++ to the C++ extractor, [critic] p3 `build` `core-cli`: Record calls to plainly imported external Python modules, Critic verdict, Summary, [synth] p0 `build` `core-cli`: Redirect `update <subdir>` and `watch` to the recorded scan root (+30 more)

### Community 17
Cohesion: 0.06
Nodes (31): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+23 more)

### Community 18
Cohesion: 0.15
Nodes (25): oldNode, ReadShrinkGuard(), ShrinkGuard, io.Reader, nodeScan, nodeScan.fill(), nodeScan.key(), nodeScan.next() (+17 more)

### Community 19
Cohesion: 0.13
Nodes (21): Graph.UnclassifiedSummary(), testing.T, TestUnclassifiedSummaryEmptyWhenNoneSkipped(), TestUnclassifiedSummaryRanksAndCaps(), TestUnclassifiedSummarySanitizesExtensions(), Validate(), testing.T, TestValidateClean() (+13 more)

### Community 20
Cohesion: 0.11
Nodes (17): Agent Context Profiles, Agent Instructions, Beads Issue Tracker, Beads Issue Tracker, Non-Interactive Shell Commands, Quick Reference, Quick Reference, Quick Reference (+9 more)

### Community 21
Cohesion: 0.35
Nodes (14): encoding/json.RawMessage, loadRawGraph(), Merge(), rawGraph, loadMerged(), testing.T, TestMergeKeepsBothDirectionsAndRelations(), TestMergeMissingFile() (+6 more)

### Community 22
Cohesion: 0.15
Nodes (13): Effort / sequencing, Explicitly out of scope (don't pull these in), Gap table, Implementation plan (single follow-up session), Key references, Phase 1 — get the data onto the page, Phase 2 — sidebar UI (template), Phase 3 — selection behavior (+5 more)

### Community 23
Cohesion: 0.17
Nodes (10): express, Math, Math.add(), Server, Server.boot(), Server.start(), boot(), Server (+2 more)

### Community 24
Cohesion: 0.18
Nodes (11): Correctness / coverage, Done, Follow-ups, GOALS, Language coverage, New commands (in-spirit parity push), Objective, Operational (release pipeline) (+3 more)

### Community 25
Cohesion: 0.47
Nodes (10): gapEdges(), TestGapAbstractDoesNotBlockSingleImpl(), TestGapAbstractWithSameFileOverride(), TestGapCSharpNullConditionalCall(), TestGapJSMemberAssignedFunction(), TestGapRubyBareSelfSend(), TestGapRubyLocalsNotSelfSends(), TestGapScalaDeferredDef() (+2 more)

### Community 26
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 27
Cohesion: 0.22
Nodes (8): Beads - AI-Native Issue Tracking, Essential Commands, Get Started with Beads, Learn More, Quick Start, What is Beads?, Why Beads?, Working with Issues

### Community 28
Cohesion: 0.32
Nodes (6): add(), double(), boot(), Server, Server.start(), std

### Community 29
Cohesion: 0.38
Nodes (6): defs.svh, add(), alu, compute(), Counter, Counter.step()

### Community 30
Cohesion: 0.33
Nodes (6): helper.h, boot(), run(), Server, Server.start(), stdio.h

### Community 31
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 32
Cohesion: 0.47
Nodes (4): /etc/profile, boot(), add(), double()

### Community 33
Cohesion: 0.53
Nodes (4): add(), boot(), Server, Server.start()

### Community 34
Cohesion: 0.47
Nodes (5): boot(), Int, Server, Server.start(), scala.collection.mutable.Map

### Community 35
Cohesion: 0.33
Nodes (5): github.com/dobbo-ca/gositter.Language, limit(), Parse(), testing.T, TestLimit()

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
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), json

### Community 44
Cohesion: 0.60
Nodes (4): add(), Int, MathUtils, MathUtils.double()

### Community 45
Cohesion: 0.50
Nodes (3): Build / remove set, Deliberately skipped (agent-first), v0.5.0 — Agent-first delta

### Community 46
Cohesion: 0.67
Nodes (3): add(), M, M.double()

### Community 47
Cohesion: 0.50
Nodes (4): Tables, Tables, orders, orders

### Community 48
Cohesion: 0.67
Nodes (3): Add(), Calc, Calc.Sum()

### Community 49
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **266 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+261 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **8 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
