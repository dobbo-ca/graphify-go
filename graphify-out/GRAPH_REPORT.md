# Graph Report - .

## Summary
- 1576 nodes · 3873 edges · 40 communities
- Extraction: 52% EXTRACTED · 47% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `9be99b9f`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 87 edges
2. `MakeID()` - 85 edges
3. `FileFromBytes()` - 66 edges
4. `Resolve()` - 57 edges
5. `github.com/dobbo-ca/graphify-go` - 52 edges
6. `Beads (from synthesis)` - 45 edges
7. `builder.def()` - 45 edges
8. `fieldText()` - 35 edges
9. `walk()` - 34 edges
10. `main()` - 29 edges

## Surprising Connections (you probably didn't know these)
- `tsCycleGraph()` --calls--> `FileFromBytes()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/extract/extract.go  _bridges separate communities_
- `tsCycleGraph()` --calls--> `Resolve()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/extract/resolve.go  _bridges separate communities_
- `Save()` --calls--> `WriteFileAtomic()`  [INFERRED]
  internal/cache/cache.go → internal/fsutil/fsutil.go  _bridges separate communities_
- `SaveStat()` --calls--> `WriteFileAtomic()`  [INFERRED]
  internal/cache/cache.go → internal/fsutil/fsutil.go  _bridges separate communities_
- `ToJSON()` --calls--> `WriteFileAtomic()`  [INFERRED]
  internal/export/export.go → internal/fsutil/fsutil.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (40 total)

### Community 0
Cohesion: 0.03
Nodes (222): bashHasExpansion(), builder.bashCommand(), builder.bashFunc(), builder.bashItems(), builder.scriptInvocationTarget(), collectBashFuncs(), commandName(), extractBash() (+214 more)

### Community 1
Cohesion: 0.03
Nodes (112): crypto/rand, encoding/csv, errors, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge (+104 more)

### Community 2
Cohesion: 0.03
Nodes (109): TestExtractBash(), TestExtractC(), TestExtractVueComponent(), TestExtractJSONSkipsCommentKey(), TestExtractJSSkipsDegenerateName(), TestFileFromBytesMtsAsTypeScript(), TestFileFromBytesShebangBash(), File() (+101 more)

### Community 3
Cohesion: 0.04
Nodes (105): Cache, cacheFile, Entry, HashBytes(), HashFile(), Load(), LoadStat(), mtimeGranularity() (+97 more)

### Community 4
Cohesion: 0.05
Nodes (101): Cycle, entityLoc(), GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), isJSONKeyNode() (+93 more)

### Community 5
Cohesion: 0.05
Nodes (83): bufio, encoding/json, github.com/dobbo-ca/graphify-go/internal/query, TestExplainErrorKeepsAllCandidates(), TestExplainErrorSanitizesCandidates(), argInt(), argString(), argStrings() (+75 more)

### Community 6
Cohesion: 0.05
Nodes (77): math, Ask(), attributesText(), bfsTraverse(), completeInducedEdges(), computeIDF(), dfsTraverse(), hubThreshold() (+69 more)

### Community 7
Cohesion: 0.05
Nodes (64): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+56 more)

### Community 8
Cohesion: 0.03
Nodes (60): Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs), Beads (from synthesis), Critic: misclassified beads, Critic: missed capabilities, [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges, [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line, [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction, [critic] p2 `build` `core-cli` — Cap and group explain's connection list instead of dumping every neighbour (+52 more)

### Community 9
Cohesion: 0.06
Nodes (52): CollectFiles(), CollectFilesReport(), CollectManifests(), genericKeywordHit(), inMemoryDir(), isASCIIAlnum(), isASCIIAlpha(), IsSensitive() (+44 more)

### Community 10
Cohesion: 0.05
Nodes (28): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+20 more)

### Community 11
Cohesion: 0.04
Nodes (53): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+45 more)

### Community 12
Cohesion: 0.05
Nodes (44): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+36 more)

### Community 13
Cohesion: 0.07
Nodes (45): context, github.com/anthropics/anthropic-sdk-go, github.com/anthropics/anthropic-sdk-go/bedrock, github.com/dobbo-ca/graphify-go/internal/semantic, collectNotes(), enrich(), loadSemanticCache(), newSemanticBackend() (+37 more)

### Community 14
Cohesion: 0.09
Nodes (41): encoding/xml, crate, IntrospectCargo(), loadTOML(), memberManifestPaths(), packageName(), TestIntrospectCargoHonorsPackageRename(), TestIntrospectCargoRenameToExternalIsNoEdge() (+33 more)

### Community 15
Cohesion: 0.11
Nodes (30): TestReadPathList(), Affected(), AffectedOptions, AffectedResult, Graph.collect(), normalizeSeed(), impacted(), loadJSON() (+22 more)

### Community 16
Cohesion: 0.17
Nodes (21): hasScriptCall(), resolveFiles(), TestBashScriptInvocation(), TestBashScriptInvocationFromFunction(), TestBashScriptInvocationPrunesMissing(), TestBashScriptInvocationSkipsDynamic(), TestBashScriptInvocationSkipsShadowed(), writeScript() (+13 more)

### Community 17
Cohesion: 0.22
Nodes (19): concept, concept.link(), conceptDescription(), conceptDir(), conceptDoc(), OKFFromJSON(), parentDir(), relationsByNode() (+11 more)

### Community 18
Cohesion: 0.11
Nodes (17): Agent Context Profiles, Agent Instructions, Beads Issue Tracker, Beads Issue Tracker, Non-Interactive Shell Commands, Quick Reference, Quick Reference, Quick Reference (+9 more)

### Community 19
Cohesion: 0.11
Nodes (18): Authoritative cloudposse `id` algorithm (to reimplement), Background — current Terraform extraction, Confidence storage, Data model — full change surface, Design decisions, Implementation stages, Objective, Out of scope (+10 more)

### Community 20
Cohesion: 0.26
Nodes (13): dispatchTargets(), implementsEdge(), TestCSharpDispatchCrossLanguage(), TestCSharpDispatchSingleImplementer(), TestCSharpDispatchTwoImplementers(), extractCSharp(), fileStem(), extractJSON() (+5 more)

### Community 21
Cohesion: 0.15
Nodes (13): Effort / sequencing, Explicitly out of scope (don't pull these in), Gap table, Implementation plan (single follow-up session), Key references, Phase 1 — get the data onto the page, Phase 2 — sidebar UI (template), Phase 3 — selection behavior (+5 more)

### Community 22
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 23
Cohesion: 0.22
Nodes (8): Beads - AI-Native Issue Tracking, Essential Commands, Get Started with Beads, Learn More, Quick Start, What is Beads?, Why Beads?, Working with Issues

### Community 24
Cohesion: 0.38
Nodes (6): defs.svh, add(), alu, compute(), Counter, Counter.step()

### Community 25
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 26
Cohesion: 0.40
Nodes (5): Business Glossary, Business Glossary, Revenue, Analytics Knowledge Bundle, Analytics Knowledge Bundle

### Community 27
Cohesion: 0.50
Nodes (3): 📚 Documentation, 🚀 Features, 🔧 Miscellaneous Tasks

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
- **298 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+293 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **3 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
