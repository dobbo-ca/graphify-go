# Graph Report - /private/tmp/gfy-wt-graphify-go-2af.11

## Summary
- 1421 nodes · 3409 edges · 41 communities
- Extraction: 53% EXTRACTED · 46% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `2077bbee`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 84 edges
2. `MakeID()` - 81 edges
3. `FileFromBytes()` - 61 edges
4. `github.com/dobbo-ca/graphify-go` - 52 edges
5. `Resolve()` - 51 edges
6. `Beads (from synthesis)` - 45 edges
7. `builder.def()` - 45 edges
8. `fieldText()` - 34 edges
9. `walk()` - 33 edges
10. `main()` - 26 edges

## Surprising Connections (you probably didn't know these)
- `tsCycleGraph()` --calls--> `FileFromBytes()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/extract/extract.go  _bridges separate communities_
- `tsCycleGraph()` --calls--> `Resolve()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/extract/resolve.go  _bridges separate communities_
- `conceptDir()` --calls--> `MakeID()`  [INFERRED]
  internal/export/okf.go → internal/idutil/idutil.go  _bridges separate communities_
- `OKFFromJSON()` --calls--> `MakeID()`  [INFERRED]
  internal/export/okf.go → internal/idutil/idutil.go  _bridges separate communities_
- `resolveFiles()` --calls--> `File()`  [INFERRED]
  internal/extract/bash_scriptinvocation_test.go → internal/extract/extract.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (41 total)

### Community 0
Cohesion: 0.03
Nodes (193): bashHasExpansion(), builder.bashCommand(), builder.bashFunc(), builder.bashItems(), builder.scriptInvocationTarget(), collectBashFuncs(), commandName(), extractBash() (+185 more)

### Community 1
Cohesion: 0.03
Nodes (143): bytes, Cache, cacheFile, Entry, HashBytes(), HashFile(), Load(), LoadStat() (+135 more)

### Community 2
Cohesion: 0.03
Nodes (115): TestExtractBash(), TestExtractC(), TestExtractVueComponent(), TestExtractJSONSkipsCommentKey(), TestExtractJSSkipsDegenerateName(), TestFileFromBytesMtsAsTypeScript(), TestFileFromBytesShebangBash(), File() (+107 more)

### Community 3
Cohesion: 0.05
Nodes (102): Cycle, entityLoc(), GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey() (+94 more)

### Community 4
Cohesion: 0.06
Nodes (56): CollectFiles(), CollectFilesReport(), CollectManifests(), genericKeywordHit(), isASCIIAlnum(), isASCIIAlpha(), IsSensitive(), ShebangExt() (+48 more)

### Community 5
Cohesion: 0.03
Nodes (60): Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs), Beads (from synthesis), Critic: misclassified beads, Critic: missed capabilities, [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges, [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line, [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction, [critic] p2 `build` `core-cli` — Cap and group explain's connection list instead of dumping every neighbour (+52 more)

### Community 6
Cohesion: 0.06
Nodes (53): encoding/csv, encoding/xml, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge, gmlGraph (+45 more)

### Community 7
Cohesion: 0.07
Nodes (53): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+45 more)

### Community 8
Cohesion: 0.08
Nodes (54): bufio, github.com/dobbo-ca/graphify-go/internal/analyze, github.com/dobbo-ca/graphify-go/internal/query, argInt(), argString(), argTokenBudget(), budgetLines(), cmdServe() (+46 more)

### Community 9
Cohesion: 0.05
Nodes (28): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+20 more)

### Community 10
Cohesion: 0.04
Nodes (53): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+45 more)

### Community 11
Cohesion: 0.07
Nodes (46): context, github.com/anthropics/anthropic-sdk-go, github.com/anthropics/anthropic-sdk-go/bedrock, github.com/dobbo-ca/graphify-go/internal/detect, github.com/dobbo-ca/graphify-go/internal/semantic, collectNotes(), enrich(), loadSemanticCache() (+38 more)

### Community 12
Cohesion: 0.08
Nodes (47): Effort / sequencing, Explicitly out of scope (don't pull these in), Gap table, Implementation plan (single follow-up session), Key references, Phase 1 — get the data onto the page, Phase 2 — sidebar UI (template), Phase 3 — selection behavior (+39 more)

### Community 13
Cohesion: 0.11
Nodes (34): crate, IntrospectCargo(), loadTOML(), memberManifestPaths(), packageName(), TestIntrospectCargoHonorsPackageRename(), TestIntrospectCargoRenameToExternalIsNoEdge(), hasEdge() (+26 more)

### Community 14
Cohesion: 0.12
Nodes (29): Affected(), AffectedOptions, AffectedResult, Graph.collect(), normalizeSeed(), impacted(), loadJSON(), TestAffectedDepth() (+21 more)

### Community 15
Cohesion: 0.11
Nodes (29): edgeLoc(), Explain(), Explanation, Graph, Graph.bfsPath(), Graph.resolve(), Link, Load() (+21 more)

### Community 16
Cohesion: 0.11
Nodes (17): Agent Context Profiles, Agent Instructions, Beads Issue Tracker, Beads Issue Tracker, Non-Interactive Shell Commands, Quick Reference, Quick Reference, Quick Reference (+9 more)

### Community 17
Cohesion: 0.11
Nodes (18): Authoritative cloudposse `id` algorithm (to reimplement), Background — current Terraform extraction, Confidence storage, Data model — full change surface, Design decisions, Implementation stages, Objective, Out of scope (+10 more)

### Community 18
Cohesion: 0.28
Nodes (15): builder.luaAssign(), builder.luaCall(), builder.luaCalls(), builder.luaEnsureType(), builder.luaFunc(), builder.luaMethod(), builder.luaStatement(), builder.luaTopCalls() (+7 more)

### Community 19
Cohesion: 0.35
Nodes (11): hasScriptCall(), resolveFiles(), TestBashScriptInvocation(), TestBashScriptInvocationFromFunction(), TestBashScriptInvocationPrunesMissing(), TestBashScriptInvocationSkipsDynamic(), TestBashScriptInvocationSkipsShadowed(), writeScript() (+3 more)

### Community 20
Cohesion: 0.18
Nodes (10): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+2 more)

### Community 21
Cohesion: 0.18
Nodes (11): Correctness / coverage, Done, Follow-ups, GOALS, Language coverage, New commands (in-spirit parity push), Objective, Operational (release pipeline) (+3 more)

### Community 22
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 23
Cohesion: 0.22
Nodes (8): Beads - AI-Native Issue Tracking, Essential Commands, Get Started with Beads, Learn More, Quick Start, What is Beads?, Why Beads?, Working with Issues

### Community 24
Cohesion: 0.54
Nodes (7): callConfidence(), countCalls(), resolvePy(), TestImportGuidedNonAliased(), TestImportGuidedResolvesAmbiguousAlias(), TestImportGuidedSkipsMemberCall(), TestImportGuidedSkipsSelfEdge()

### Community 25
Cohesion: 0.38
Nodes (6): defs.svh, add(), alu, compute(), Counter, Counter.step()

### Community 26
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 27
Cohesion: 0.29
Nodes (7): graphify-go, Install, Pipeline, Scope, Usage, Use it in your own repo, Why

### Community 28
Cohesion: 0.40
Nodes (5): Business Glossary, Business Glossary, Revenue, Analytics Knowledge Bundle, Analytics Knowledge Bundle

### Community 29
Cohesion: 0.50
Nodes (3): 📚 Documentation, 🚀 Features, 🔧 Miscellaneous Tasks

### Community 30
Cohesion: 0.50
Nodes (3): Build / remove set, Deliberately skipped (agent-first), v0.5.0 — Agent-first delta

### Community 31
Cohesion: 0.50
Nodes (4): Tables, Tables, orders, orders

### Community 32
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **296 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+291 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **2 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
