# Graph Report - .

## Summary
- 1934 nodes · 5602 edges · 36 communities
- Extraction: 52% EXTRACTED · 47% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `1976a0d4`
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
8. `Relation.String()` - 54 edges
9. `builder.def()` - 46 edges
10. `Beads (from synthesis)` - 45 edges

## Surprising Connections (you probably didn't know these)
- `Surprising()` --calls--> `Relation.String()`  [INFERRED]
  internal/analyze/analyze.go → internal/model/code.go  _bridges separate communities_
- `tsCycleGraph()` --calls--> `FileFromBytes()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/extract/extract.go  _bridges separate communities_
- `tsCycleGraph()` --calls--> `Resolve()`  [INFERRED]
  internal/analyze/typeonly_cycle_test.go → internal/extract/resolve.go  _bridges separate communities_
- `writeCache()` --calls--> `Result.AppendJSON()`  [INFERRED]
  internal/cache/cache.go → internal/extract/resultjson.go  _bridges separate communities_
- `writeCache()` --calls--> `AppendJSONString()`  [INFERRED]
  internal/cache/cache.go → internal/model/jsonenc.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (36 total)

### Community 0
Cohesion: 0.02
Nodes (269): Cycle, entityLoc(), GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), isJSONKeyNode() (+261 more)

### Community 1
Cohesion: 0.03
Nodes (268): failWriter, failWriter.Write(), bashCodeFlag(), bashHasExpansion(), builder.bashCommand(), builder.bashFunc(), builder.bashItems(), builder.scriptInvocationTarget() (+260 more)

### Community 2
Cohesion: 0.02
Nodes (175): extractBash(), TestExtractBash(), TestBudgetSetting(), TestOverBudgetKeepsFileNode(), extractC(), TestExtractC(), TestExtractVueComponent(), blankCppExportMacros() (+167 more)

### Community 3
Cohesion: 0.03
Nodes (135): crypto/rand, ancestorDirs(), globToRegex(), ignoreFile, ignorer, ignorer.ignored(), ignorer.infoExcludePath(), ignorer.load() (+127 more)

### Community 4
Cohesion: 0.04
Nodes (128): bufio, bytes, Cache, cacheFile, dropBadCalls(), Entry, entryEnd(), HashBytes() (+120 more)

### Community 5
Cohesion: 0.03
Nodes (84): defs.svh, Beads, [critic] p0 `build` `core-cli`: Skip nested git worktrees and dot-dir worktrees/ in detect, [critic] p1 `build` `core-cli`: Redact secrets in Terraform attribute values that fall through to raw source text, [critic] p1 `build` `core-cli`: Route .h headers containing C++ to the C++ extractor, [critic] p3 `build` `core-cli`: Record calls to plainly imported external Python modules, Critic verdict, Summary (+76 more)

### Community 6
Cohesion: 0.03
Nodes (76): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+68 more)

### Community 7
Cohesion: 0.06
Nodes (73): gitHead(), argInt(), argString(), argStrings(), argTokenBudget(), budgetLines(), commitLine(), communitiesOf() (+65 more)

### Community 8
Cohesion: 0.04
Nodes (70): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+62 more)

### Community 9
Cohesion: 0.03
Nodes (60): Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs), Beads (from synthesis), Critic: misclassified beads, Critic: missed capabilities, [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges, [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line, [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction, [critic] p2 `build` `core-cli` — Cap and group explain's connection list instead of dumping every neighbour (+52 more)

### Community 10
Cohesion: 0.04
Nodes (30): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+22 more)

### Community 11
Cohesion: 0.07
Nodes (44): context, github.com/anthropics/anthropic-sdk-go, github.com/anthropics/anthropic-sdk-go/bedrock, collectNotes(), enrich(), loadSemanticCache(), newSemanticBackend(), saveSemanticCache() (+36 more)

### Community 12
Cohesion: 0.07
Nodes (44): normalizeSeed(), ambiguous(), AmbiguousError, edgeLoc(), Explain(), Explanation, fileNodes(), Graph (+36 more)

### Community 13
Cohesion: 0.10
Nodes (39): CollectFiles(), CollectFilesReport(), CollectManifests(), genericKeywordHit(), inMemoryDir(), isASCIIAlnum(), isASCIIAlpha(), isNestedWorktree() (+31 more)

### Community 14
Cohesion: 0.12
Nodes (37): Ask(), attributesText(), bfsTraverse(), completeInducedEdges(), computeIDF(), dfsTraverse(), hubThreshold(), isSearchable() (+29 more)

### Community 15
Cohesion: 0.06
Nodes (31): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+23 more)

### Community 16
Cohesion: 0.15
Nodes (24): Affected(), impacted(), loadJSON(), TestAffectedDepth(), TestAffectedInheritsContext(), TestAffectedNoMatch(), TestAffectedRelation(), TestAffectedSeedPathForms() (+16 more)

### Community 17
Cohesion: 0.20
Nodes (20): hasScriptCall(), resolveFiles(), TestBashPhpDashEKeepsEdge(), TestBashRunnerModuleFlagNoEdge(), TestBashRunnerNonShellTargets(), TestBashScriptInvocation(), TestBashScriptInvocationFromFunction(), TestBashScriptInvocationPrunesMissing() (+12 more)

### Community 18
Cohesion: 0.19
Nodes (19): cargoDepNames(), IntrospectManifests(), manifestInfo, parseCargo(), parseGoMod(), parseManifest(), parsePom(), parsePyproject() (+11 more)

### Community 19
Cohesion: 0.11
Nodes (17): Agent Context Profiles, Agent Instructions, Beads Issue Tracker, Beads Issue Tracker, Non-Interactive Shell Commands, Quick Reference, Quick Reference, Quick Reference (+9 more)

### Community 20
Cohesion: 0.15
Nodes (13): Effort / sequencing, Explicitly out of scope (don't pull these in), Gap table, Implementation plan (single follow-up session), Key references, Phase 1 — get the data onto the page, Phase 2 — sidebar UI (template), Phase 3 — selection behavior (+5 more)

### Community 21
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 22
Cohesion: 0.31
Nodes (9): clean(), cleanASCII(), cleanUnicode(), NormalizeID(), FuzzCleanASCII(), TestCleanASCIIMatchesUnicode(), TestCleanIsIdempotentAndCaselessStable(), TestMakeID() (+1 more)

### Community 23
Cohesion: 0.22
Nodes (8): Beads - AI-Native Issue Tracking, Essential Commands, Get Started with Beads, Learn More, Quick Start, What is Beads?, Why Beads?, Working with Issues

### Community 24
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 25
Cohesion: 0.40
Nodes (5): Business Glossary, Business Glossary, Revenue, Analytics Knowledge Bundle, Analytics Knowledge Bundle

### Community 26
Cohesion: 0.50
Nodes (3): Build / remove set, Deliberately skipped (agent-first), v0.5.0 — Agent-first delta

### Community 27
Cohesion: 0.50
Nodes (4): Tables, Tables, orders, orders

### Community 28
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **344 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+339 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **1 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
