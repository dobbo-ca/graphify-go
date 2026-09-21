# Graph Report - /private/tmp/gfy-wt-graphify-go-2af.32

## Summary
- 1510 nodes · 3651 edges · 39 communities
- Extraction: 53% EXTRACTED · 46% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `1fe2440c`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 84 edges
2. `MakeID()` - 84 edges
3. `FileFromBytes()` - 62 edges
4. `Resolve()` - 54 edges
5. `github.com/dobbo-ca/graphify-go` - 52 edges
6. `Beads (from synthesis)` - 45 edges
7. `builder.def()` - 45 edges
8. `fieldText()` - 34 edges
9. `walk()` - 33 edges
10. `main()` - 28 edges

## Surprising Connections (you probably didn't know these)
- `tsCycleGraph()` --calls--> `FileFromBytes()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/extract/extract.go  _bridges separate communities_
- `tsCycleGraph()` --calls--> `Resolve()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/extract/resolve.go  _bridges separate communities_
- `TestCollectFilesGitignore()` --calls--> `CollectFiles()`  [INFERRED]
  internal/detect/gitignore_test.go → internal/detect/detect.go  _bridges separate communities_
- `TestCollectFilesGraphifyignore()` --calls--> `CollectFiles()`  [INFERRED]
  internal/detect/gitignore_test.go → internal/detect/detect.go  _bridges separate communities_
- `ToJSON()` --calls--> `WriteFileAtomic()`  [INFERRED]
  internal/export/export.go → internal/fsutil/fsutil.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (39 total)

### Community 0
Cohesion: 0.03
Nodes (221): bashHasExpansion(), builder.bashCommand(), builder.bashFunc(), builder.bashItems(), builder.scriptInvocationTarget(), collectBashFuncs(), commandName(), extractBash() (+213 more)

### Community 1
Cohesion: 0.02
Nodes (168): bytes, Cache, cacheFile, Entry, HashBytes(), HashFile(), Load(), LoadStat() (+160 more)

### Community 2
Cohesion: 0.04
Nodes (112): Cycle, entityLoc(), GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), isJSONKeyNode() (+104 more)

### Community 3
Cohesion: 0.03
Nodes (84): TestExtractBash(), TestExtractC(), TestExtractVueComponent(), dispatchTargets(), implementsEdge(), TestCSharpDispatchCrossLanguage(), TestCSharpDispatchSingleImplementer(), TestCSharpDispatchTwoImplementers() (+76 more)

### Community 4
Cohesion: 0.06
Nodes (78): bufio, github.com/dobbo-ca/graphify-go/internal/analyze, github.com/dobbo-ca/graphify-go/internal/query, argInt(), argString(), argStrings(), argTokenBudget(), budgetLines() (+70 more)

### Community 5
Cohesion: 0.06
Nodes (58): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+50 more)

### Community 6
Cohesion: 0.03
Nodes (60): Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs), Beads (from synthesis), Critic: misclassified beads, Critic: missed capabilities, [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges, [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line, [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction, [critic] p2 `build` `core-cli` — Cap and group explain's connection list instead of dumping every neighbour (+52 more)

### Community 7
Cohesion: 0.07
Nodes (52): encoding/csv, encoding/xml, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge, gmlGraph (+44 more)

### Community 8
Cohesion: 0.07
Nodes (52): crate, IntrospectCargo(), loadTOML(), memberManifestPaths(), packageName(), TestIntrospectCargoHonorsPackageRename(), TestIntrospectCargoRenameToExternalIsNoEdge(), hasEdge() (+44 more)

### Community 9
Cohesion: 0.04
Nodes (49): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+41 more)

### Community 10
Cohesion: 0.05
Nodes (28): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+20 more)

### Community 11
Cohesion: 0.07
Nodes (49): CollectFiles(), CollectFilesReport(), CollectManifests(), genericKeywordHit(), inMemoryDir(), isASCIIAlnum(), isASCIIAlpha(), IsSensitive() (+41 more)

### Community 12
Cohesion: 0.04
Nodes (53): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+45 more)

### Community 13
Cohesion: 0.07
Nodes (46): context, github.com/anthropics/anthropic-sdk-go, github.com/anthropics/anthropic-sdk-go/bedrock, github.com/dobbo-ca/graphify-go/internal/cache, github.com/dobbo-ca/graphify-go/internal/semantic, collectNotes(), enrich(), loadSemanticCache() (+38 more)

### Community 14
Cohesion: 0.12
Nodes (37): math, Ask(), bfsTraverse(), completeInducedEdges(), computeIDF(), dfsTraverse(), hubThreshold(), isSearchable() (+29 more)

### Community 15
Cohesion: 0.12
Nodes (29): Affected(), AffectedOptions, AffectedResult, Graph.collect(), normalizeSeed(), impacted(), loadJSON(), TestAffectedDepth() (+21 more)

### Community 16
Cohesion: 0.10
Nodes (30): edgeLoc(), Explain(), Explanation, Graph, Graph.bfsPath(), Graph.resolve(), GraphAttrs, Link (+22 more)

### Community 17
Cohesion: 0.11
Nodes (17): Agent Context Profiles, Agent Instructions, Beads Issue Tracker, Beads Issue Tracker, Non-Interactive Shell Commands, Quick Reference, Quick Reference, Quick Reference (+9 more)

### Community 18
Cohesion: 0.11
Nodes (18): Authoritative cloudposse `id` algorithm (to reimplement), Background — current Terraform extraction, Confidence storage, Data model — full change surface, Design decisions, Implementation stages, Objective, Out of scope (+10 more)

### Community 19
Cohesion: 0.17
Nodes (14): detectPackageFromArgs(), extractMCPConfig(), IsMCPConfigPath(), mcpServersMap(), mcpServerSpec, sortedKeys(), stripVersion(), TestDetectPackageFromArgs() (+6 more)

### Community 20
Cohesion: 0.35
Nodes (11): hasScriptCall(), resolveFiles(), TestBashScriptInvocation(), TestBashScriptInvocationFromFunction(), TestBashScriptInvocationPrunesMissing(), TestBashScriptInvocationSkipsDynamic(), TestBashScriptInvocationSkipsShadowed(), writeScript() (+3 more)

### Community 21
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 22
Cohesion: 0.22
Nodes (8): Beads - AI-Native Issue Tracking, Essential Commands, Get Started with Beads, Learn More, Quick Start, What is Beads?, Why Beads?, Working with Issues

### Community 23
Cohesion: 0.38
Nodes (6): defs.svh, add(), alu, compute(), Counter, Counter.step()

### Community 24
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 25
Cohesion: 0.40
Nodes (5): Business Glossary, Business Glossary, Revenue, Analytics Knowledge Bundle, Analytics Knowledge Bundle

### Community 26
Cohesion: 0.50
Nodes (3): 📚 Documentation, 🚀 Features, 🔧 Miscellaneous Tasks

### Community 27
Cohesion: 0.50
Nodes (3): Build / remove set, Deliberately skipped (agent-first), v0.5.0 — Agent-first delta

### Community 28
Cohesion: 0.50
Nodes (4): Tables, Tables, orders, orders

### Community 29
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **299 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+294 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **3 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
