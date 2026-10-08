# Graph Report - .

## Summary
- 1695 nodes · 4340 edges · 39 communities
- Extraction: 54% EXTRACTED · 45% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `d2bee405`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `MakeID()` - 92 edges
2. `line()` - 91 edges
3. `FileFromBytes()` - 76 edges
4. `Resolve()` - 68 edges
5. `github.com/dobbo-ca/graphify-go` - 52 edges
6. `builder.def()` - 46 edges
7. `Beads (from synthesis)` - 45 edges
8. `fieldText()` - 43 edges
9. `walk()` - 39 edges
10. `Beads` - 35 edges

## Surprising Connections (you probably didn't know these)
- `TestGodNodesExcludeNoiseLabels()` --calls--> `Graph.AddNode()`  [INFERRED]
  internal/analyze/analyze_test.go → internal/model/model.go  _bridges separate communities_
- `TestImportCyclesSkipsTypeOnlyImports()` --calls--> `ImportCycles()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/analyze/analyze.go  _bridges separate communities_
- `TestImportCyclesTypeAndValueImportSameModule()` --calls--> `ImportCycles()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/analyze/analyze.go  _bridges separate communities_
- `tsCycleGraph()` --calls--> `FileFromBytes()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/extract/extract.go  _bridges separate communities_
- `tsCycleGraph()` --calls--> `Resolve()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/extract/resolve.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (39 total)

### Community 0
Cohesion: 0.03
Nodes (241): TestImportCyclesSkipsTypeOnlyImports(), TestImportCyclesTypeAndValueImportSameModule(), tsCycleGraph(), Cache, cacheFile, Entry, mtimeGranularity(), StatEntry (+233 more)

### Community 1
Cohesion: 0.03
Nodes (124): bytes, crypto/rand, TestCollectFilesGitignore(), TestCollectFilesGraphifyignore(), embed, encoding/hex, encoding/json, errors (+116 more)

### Community 2
Cohesion: 0.03
Nodes (116): TestExtractBash(), TestExtractC(), TestExtractVueComponent(), TestExtractJSONSkipsCommentKey(), TestExtractJSSkipsDegenerateName(), bareMethodName(), isCSharpNode(), resolveCSharpDispatch() (+108 more)

### Community 3
Cohesion: 0.04
Nodes (115): bufio, argInt(), argString(), argStrings(), argTokenBudget(), budgetLines(), cmdServe(), commitLine() (+107 more)

### Community 4
Cohesion: 0.04
Nodes (105): HashBytes(), HashFile(), Load(), LoadStat(), Save(), SaveStat(), Stamp(), TestHashFileFastpath() (+97 more)

### Community 5
Cohesion: 0.05
Nodes (91): Cycle, entityLoc(), GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), isJSONKeyNode() (+83 more)

### Community 6
Cohesion: 0.04
Nodes (83): extractBash(), extractC(), blankCppExportMacros(), extractCpp(), TestCppExportMacroClass(), TestCppHeaderRoutesToCppExtractor(), dispatchTargets(), implementsEdge() (+75 more)

### Community 7
Cohesion: 0.06
Nodes (58): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+50 more)

### Community 8
Cohesion: 0.07
Nodes (54): ancestorDirs(), globToRegex(), ignoreFile, ignorer, ignorer.ignored(), ignorer.infoExcludePath(), ignorer.load(), ignoreRule (+46 more)

### Community 9
Cohesion: 0.03
Nodes (60): Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs), Beads (from synthesis), Critic: misclassified beads, Critic: missed capabilities, [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges, [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line, [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction, [critic] p2 `build` `core-cli` — Cap and group explain's connection list instead of dumping every neighbour (+52 more)

### Community 10
Cohesion: 0.04
Nodes (30): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+22 more)

### Community 11
Cohesion: 0.04
Nodes (53): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+45 more)

### Community 12
Cohesion: 0.07
Nodes (44): context, github.com/anthropics/anthropic-sdk-go, github.com/anthropics/anthropic-sdk-go/bedrock, collectNotes(), enrich(), loadSemanticCache(), newSemanticBackend(), saveSemanticCache() (+36 more)

### Community 13
Cohesion: 0.08
Nodes (43): encoding/csv, encoding/xml, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge, gmlGraph (+35 more)

### Community 14
Cohesion: 0.05
Nodes (44): defs.svh, Beads, [critic] p0 `build` `core-cli`: Skip nested git worktrees and dot-dir worktrees/ in detect, [critic] p1 `build` `core-cli`: Redact secrets in Terraform attribute values that fall through to raw source text, [critic] p1 `build` `core-cli`: Route .h headers containing C++ to the C++ extractor, [critic] p3 `build` `core-cli`: Record calls to plainly imported external Python modules, Critic verdict, Summary (+36 more)

### Community 15
Cohesion: 0.12
Nodes (38): math, Ask(), attributesText(), bfsTraverse(), completeInducedEdges(), computeIDF(), dfsTraverse(), hubThreshold() (+30 more)

### Community 16
Cohesion: 0.11
Nodes (36): CollectFiles(), CollectFilesReport(), CollectManifests(), genericKeywordHit(), inMemoryDir(), isASCIIAlnum(), isASCIIAlpha(), isNestedWorktree() (+28 more)

### Community 17
Cohesion: 0.14
Nodes (25): cmdDiff(), Affected(), impacted(), loadJSON(), TestAffectedDepth(), TestAffectedInheritsContext(), TestAffectedNoMatch(), TestAffectedRelation() (+17 more)

### Community 18
Cohesion: 0.18
Nodes (22): hasScriptCall(), resolveFiles(), TestBashPhpDashEKeepsEdge(), TestBashRunnerModuleFlagNoEdge(), TestBashRunnerNonShellTargets(), TestBashScriptInvocation(), TestBashScriptInvocationFromFunction(), TestBashScriptInvocationPrunesMissing() (+14 more)

### Community 19
Cohesion: 0.11
Nodes (17): Agent Context Profiles, Agent Instructions, Beads Issue Tracker, Beads Issue Tracker, Non-Interactive Shell Commands, Quick Reference, Quick Reference, Quick Reference (+9 more)

### Community 20
Cohesion: 0.11
Nodes (18): Authoritative cloudposse `id` algorithm (to reimplement), Background — current Terraform extraction, Confidence storage, Data model — full change surface, Design decisions, Implementation stages, Objective, Out of scope (+10 more)

### Community 21
Cohesion: 0.15
Nodes (13): Effort / sequencing, Explicitly out of scope (don't pull these in), Gap table, Implementation plan (single follow-up session), Key references, Phase 1 — get the data onto the page, Phase 2 — sidebar UI (template), Phase 3 — selection behavior (+5 more)

### Community 22
Cohesion: 0.18
Nodes (10): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+2 more)

### Community 23
Cohesion: 0.18
Nodes (11): Correctness / coverage, Done, Follow-ups, GOALS, Language coverage, New commands (in-spirit parity push), Objective, Operational (release pipeline) (+3 more)

### Community 24
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

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
Nodes (3): 📚 Documentation, 🚀 Features, 🔧 Miscellaneous Tasks

### Community 29
Cohesion: 0.50
Nodes (3): Build / remove set, Deliberately skipped (agent-first), v0.5.0 — Agent-first delta

### Community 30
Cohesion: 0.50
Nodes (4): Tables, Tables, orders, orders

### Community 31
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **336 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+331 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **1 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
