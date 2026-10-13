# Graph Report - .

## Summary
- 1928 nodes · 5575 edges · 38 communities
- Extraction: 52% EXTRACTED · 47% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `030df3b2`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `MakeID()` - 93 edges
2. `line()` - 92 edges
3. `FileFromBytes()` - 84 edges
4. `Node.ChildByFieldName()` - 61 edges
5. `Node.Child()` - 60 edges
6. `Node.ChildCount()` - 59 edges
7. `Resolve()` - 58 edges
8. `Relation.String()` - 53 edges
9. `builder.def()` - 46 edges
10. `Beads (from synthesis)` - 45 edges

## Surprising Connections (you probably didn't know these)
- `Surprising()` --calls--> `Confidence.String()`  [INFERRED]
  internal/analyze/analyze.go → internal/model/code.go  _bridges separate communities_
- `Surprising()` --calls--> `Relation.String()`  [INFERRED]
  internal/analyze/analyze.go → internal/model/code.go  _bridges separate communities_
- `fileHubNoteGraph()` --calls--> `Rel()`  [INFERRED]
  internal/analyze/analyze_test.go → internal/model/code.go  _bridges separate communities_
- `sampleGraph()` --calls--> `Conf()`  [INFERRED]
  internal/analyze/analyze_test.go → internal/model/code.go  _bridges separate communities_
- `TestGodNodesExcludeNoiseLabels()` --calls--> `Graph.AddNode()`  [INFERRED]
  internal/analyze/analyze_test.go → internal/model/model.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (38 total)

### Community 0
Cohesion: 0.03
Nodes (266): bytes, failWriter, failWriter.Write(), bashCodeFlag(), bashHasExpansion(), builder.bashCommand(), builder.bashFunc(), builder.bashItems() (+258 more)

### Community 1
Cohesion: 0.03
Nodes (161): Cycle, entityLoc(), GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), isJSONKeyNode() (+153 more)

### Community 2
Cohesion: 0.02
Nodes (127): bufio, crypto/rand, TestCollectFilesGitignore(), TestCollectFilesGraphifyignore(), embed, encoding/hex, errors, blankCppExportMacros() (+119 more)

### Community 3
Cohesion: 0.03
Nodes (161): fmt, TestExplainErrorKeepsAllCandidates(), TestExplainErrorSanitizesCandidates(), argInt(), argString(), argStrings(), argTokenBudget(), budgetLines() (+153 more)

### Community 4
Cohesion: 0.03
Nodes (133): Cache, cacheFile, dropBadCalls(), Entry, entryEnd(), HashBytes(), HashFile(), Load() (+125 more)

### Community 5
Cohesion: 0.04
Nodes (100): hasScriptCall(), resolveFiles(), TestBashPhpDashEKeepsEdge(), TestBashRunnerModuleFlagNoEdge(), TestBashRunnerNonShellTargets(), TestBashScriptInvocation(), TestBashScriptInvocationFromFunction(), TestBashScriptInvocationPrunesMissing() (+92 more)

### Community 6
Cohesion: 0.04
Nodes (93): extractBash(), TestBudgetSetting(), TestOverBudgetKeepsFileNode(), extractC(), componentLangPtr(), extractComponent(), lineOf(), maskComponentScript() (+85 more)

### Community 7
Cohesion: 0.06
Nodes (74): TestExtractBash(), TestExtractC(), TestExtractVueComponent(), TestExtractCpp(), TestExtractCSharp(), File(), TestExtractAndResolve(), TestFileFromBytesNonUTF8() (+66 more)

### Community 8
Cohesion: 0.05
Nodes (69): CollectFiles(), CollectFilesReport(), CollectManifests(), genericKeywordHit(), inMemoryDir(), isASCIIAlnum(), isASCIIAlpha(), isNestedWorktree() (+61 more)

### Community 9
Cohesion: 0.04
Nodes (70): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+62 more)

### Community 10
Cohesion: 0.03
Nodes (60): Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs), Beads (from synthesis), Critic: misclassified beads, Critic: missed capabilities, [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges, [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line, [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction, [critic] p2 `build` `core-cli` — Cap and group explain's connection list instead of dumping every neighbour (+52 more)

### Community 11
Cohesion: 0.04
Nodes (30): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+22 more)

### Community 12
Cohesion: 0.07
Nodes (43): TestGoImportTargetsMatchPerImportFilter(), TestResolveModulePath(), FuzzRecvTail(), TestRecvTailMatchesRegexp(), callSite, contains(), defOwners(), defQualifiers() (+35 more)

### Community 13
Cohesion: 0.07
Nodes (44): context, github.com/anthropics/anthropic-sdk-go, github.com/anthropics/anthropic-sdk-go/bedrock, collectNotes(), enrich(), loadSemanticCache(), newSemanticBackend(), saveSemanticCache() (+36 more)

### Community 14
Cohesion: 0.04
Nodes (45): defs.svh, Beads, [critic] p0 `build` `core-cli`: Skip nested git worktrees and dot-dir worktrees/ in detect, [critic] p1 `build` `core-cli`: Redact secrets in Terraform attribute values that fall through to raw source text, [critic] p1 `build` `core-cli`: Route .h headers containing C++ to the C++ extractor, [critic] p3 `build` `core-cli`: Record calls to plainly imported external Python modules, Critic verdict, Summary (+37 more)

### Community 15
Cohesion: 0.10
Nodes (36): encoding/csv, encoding/xml, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge, gmlGraph (+28 more)

### Community 16
Cohesion: 0.12
Nodes (37): Ask(), attributesText(), bfsTraverse(), completeInducedEdges(), computeIDF(), dfsTraverse(), hubThreshold(), isSearchable() (+29 more)

### Community 17
Cohesion: 0.06
Nodes (31): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+23 more)

### Community 18
Cohesion: 0.22
Nodes (18): nodeScan, nodeScan.fill(), nodeScan.key(), nodeScan.next(), nodeScan.nodes(), nodeScan.number(), nodeScan.space(), nodeScan.str() (+10 more)

### Community 19
Cohesion: 0.11
Nodes (17): Agent Context Profiles, Agent Instructions, Beads Issue Tracker, Beads Issue Tracker, Non-Interactive Shell Commands, Quick Reference, Quick Reference, Quick Reference (+9 more)

### Community 20
Cohesion: 0.15
Nodes (13): Effort / sequencing, Explicitly out of scope (don't pull these in), Gap table, Implementation plan (single follow-up session), Key references, Phase 1 — get the data onto the page, Phase 2 — sidebar UI (template), Phase 3 — selection behavior (+5 more)

### Community 21
Cohesion: 0.18
Nodes (10): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+2 more)

### Community 22
Cohesion: 0.18
Nodes (11): Correctness / coverage, Done, Follow-ups, GOALS, Language coverage, New commands (in-spirit parity push), Objective, Operational (release pipeline) (+3 more)

### Community 23
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 24
Cohesion: 0.20
Nodes (10): Files the parser gives up on, graphify-go, Install, Performance notes, Pipeline, Profiling a build, Scope, Usage (+2 more)

### Community 25
Cohesion: 0.22
Nodes (8): Beads - AI-Native Issue Tracking, Essential Commands, Get Started with Beads, Learn More, Quick Start, What is Beads?, Why Beads?, Working with Issues

### Community 26
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 27
Cohesion: 0.40
Nodes (5): Business Glossary, Business Glossary, Revenue, Analytics Knowledge Bundle, Analytics Knowledge Bundle

### Community 28
Cohesion: 0.50
Nodes (3): Build / remove set, Deliberately skipped (agent-first), v0.5.0 — Agent-first delta

### Community 29
Cohesion: 0.50
Nodes (4): Tables, Tables, orders, orders

### Community 30
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **343 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+338 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **1 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
