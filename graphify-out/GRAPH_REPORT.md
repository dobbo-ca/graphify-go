# Graph Report - .

## Summary
- 2075 nodes · 5968 edges · 62 communities
- Extraction: 51% EXTRACTED · 48% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `14449ccf`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 98 edges
2. `FileFromBytes()` - 88 edges
3. `Node.Child()` - 67 edges
4. `Node.ChildCount()` - 66 edges
5. `Node.ChildByFieldName()` - 65 edges
6. `Resolve()` - 60 edges
7. `Relation.String()` - 56 edges
8. `builder.id()` - 55 edges
9. `MakeID()` - 54 edges
10. `fieldText()` - 49 edges

## Surprising Connections (you probably didn't know these)
- `TestBuildCargoFlag()` --calls--> `cmdBuild()`  [INFERRED]
  cmd/graphify/cargo_test.go → cmd/graphify/main.go  _bridges separate communities_
- `TestParseBuildArgs()` --calls--> `parseBuildArgs()`  [INFERRED]
  cmd/graphify/cargo_test.go → cmd/graphify/main.go  _bridges separate communities_
- `TestExplainErrorSanitizesCandidates()` --calls--> `Explain()`  [INFERRED]
  cmd/graphify/explain_error_test.go → internal/query/query.go  _bridges separate communities_
- `TestExplainErrorSanitizesCandidates()` --calls--> `Load()`  [INFERRED]
  cmd/graphify/explain_error_test.go → internal/query/query.go  _bridges separate communities_
- `TestCmdPathAnnotatesHops()` --calls--> `captureStdout()`  [INFERRED]
  cmd/graphify/explain_path_test.go → cmd/graphify/maintenance_test.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (62 total)

### Community 0
Cohesion: 0.03
Nodes (271): github.com/dobbo-ca/gositter, github.com/dobbo-ca/gositter/grammars/bash, github.com/dobbo-ca/gositter/grammars/c, github.com/dobbo-ca/gositter/grammars/cpp, github.com/dobbo-ca/gositter/grammars/csharp, github.com/dobbo-ca/gositter/grammars/golang, github.com/dobbo-ca/gositter/grammars/java, github.com/dobbo-ca/gositter/grammars/javascript (+263 more)

### Community 1
Cohesion: 0.02
Nodes (213): 📚 Documentation, 🚀 Features, 🔧 Miscellaneous Tasks, Unreleased, golang.org/x/exp/rand, Cycle, entityLoc(), GodNode (+205 more)

### Community 2
Cohesion: 0.02
Nodes (160): bytes, TestIsBrokenPipe(), TestQueryExitsZeroOnBrokenPipe(), runOut(), TestParseBudgetEndToEnd(), TestParseBudgetSameAtAnyGOMAXPROCS(), writeTree(), readGraph() (+152 more)

### Community 3
Cohesion: 0.03
Nodes (163): cmdExtract(), extractBash(), TestExtractBash(), TestBudgetScalesWithSize(), TestBudgetSetting(), TestOverBudgetKeepsFileNode(), extractC(), TestExtractC() (+155 more)

### Community 4
Cohesion: 0.03
Nodes (136): TestReadPathList(), TestCmdAskGraphTraversal(), TestCmdAskNegativeBudget(), TestCmdExplainPathGraphOverride(), TestCmdExplainPathGraphTraversal(), TestCmdExplainPathSourceFile(), TestCmdPathAnnotatesHops(), TestCmdPathDirectedContainsReverse() (+128 more)

### Community 5
Cohesion: 0.04
Nodes (120): bufio, argInt(), argString(), argStrings(), argTokenBudget(), budgetLines(), cmdServe(), commitLine() (+112 more)

### Community 6
Cohesion: 0.05
Nodes (66): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+58 more)

### Community 7
Cohesion: 0.06
Nodes (57): TestGoImportTargetsMatchPerImportFilter(), jsLike(), jsMods, jsMods.aliases(), jsMods.bound(), jsMods.def(), jsMods.each(), jsMods.edges() (+49 more)

### Community 8
Cohesion: 0.07
Nodes (57): golang.org/x/text/cases, golang.org/x/text/unicode/norm, hasScriptCall(), resolveFiles(), TestBashPhpDashEKeepsEdge(), TestBashRunnerModuleFlagNoEdge(), TestBashRunnerNonShellTargets(), TestBashScriptInvocation() (+49 more)

### Community 9
Cohesion: 0.03
Nodes (60): Add PR graph-impact MCP tools (list_prs, get_pr_impact, triage_prs), Beads (from synthesis), Critic: misclassified beads, Critic: missed capabilities, [critic] p0 `build` `core-cli` — Render the induced subgraph in ask/query_graph, not just traversal-tree edges, [critic] p1 `build` `core-cli` — Cite the traversed edge's call-site line in explain, not the neighbour's definition line, [critic] p2 `build` `core-cli` — Annotate the `path` CLI route with relation, confidence and direction, [critic] p2 `build` `core-cli` — Cap and group explain's connection list instead of dumping every neighbour (+52 more)

### Community 10
Cohesion: 0.07
Nodes (45): collectNotes(), enrich(), loadSemanticCache(), newSemanticBackend(), saveSemanticCache(), semanticOpts, context, github.com/anthropics/anthropic-sdk-go (+37 more)

### Community 11
Cohesion: 0.04
Nodes (45): defs.svh, Beads, [critic] p0 `build` `core-cli`: Skip nested git worktrees and dot-dir worktrees/ in detect, [critic] p1 `build` `core-cli`: Redact secrets in Terraform attribute values that fall through to raw source text, [critic] p1 `build` `core-cli`: Route .h headers containing C++ to the C++ extractor, [critic] p3 `build` `core-cli`: Record calls to plainly imported external Python modules, Critic verdict, Summary (+37 more)

### Community 12
Cohesion: 0.10
Nodes (44): TestJSAliasedExports(), TestJSBareCallSkipsMethods(), TestJSConstCallTargets(), TestJSConstClosureCalls(), TestJSConstInitializerNoRecursion(), TestJSConstKeepsCallBinding(), TestJSConstNoMarkdownReference(), TestJSExportedGeneratorConst() (+36 more)

### Community 13
Cohesion: 0.10
Nodes (40): CollectFiles(), CollectFilesReport(), CollectManifests(), genericKeywordHit(), inMemoryDir(), isASCIIAlnum(), isASCIIAlpha(), isNestedWorktree() (+32 more)

### Community 14
Cohesion: 0.10
Nodes (38): github.com/BurntSushi/toml, ancestorDirs(), globToRegex(), ignoreFile, ignorer, ignorer.ignored(), ignorer.infoExcludePath(), ignorer.load() (+30 more)

### Community 15
Cohesion: 0.10
Nodes (37): cmdExport(), encoding/csv, encoding/xml, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge (+29 more)

### Community 16
Cohesion: 0.12
Nodes (37): Ask(), attributesText(), bfsTraverse(), completeInducedEdges(), computeIDF(), dfsTraverse(), hubThreshold(), isSearchable() (+29 more)

### Community 17
Cohesion: 0.06
Nodes (31): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+23 more)

### Community 18
Cohesion: 0.15
Nodes (24): Affected(), impacted(), loadJSON(), TestAffectedDepth(), TestAffectedInheritsContext(), TestAffectedNoMatch(), TestAffectedRelation(), TestAffectedSeedPathForms() (+16 more)

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
Cohesion: 0.17
Nodes (10): express, Math, Math.add(), Server, Server.boot(), Server.start(), boot(), Server (+2 more)

### Community 23
Cohesion: 0.18
Nodes (10): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+2 more)

### Community 24
Cohesion: 0.18
Nodes (11): Correctness / coverage, Done, Follow-ups, GOALS, Language coverage, New commands (in-spirit parity push), Objective, Operational (release pipeline) (+3 more)

### Community 25
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 26
Cohesion: 0.20
Nodes (10): Files the parser gives up on, graphify-go, Install, Performance notes, Pipeline, Profiling a build, Scope, Usage (+2 more)

### Community 27
Cohesion: 0.22
Nodes (8): Beads - AI-Native Issue Tracking, Essential Commands, Get Started with Beads, Learn More, Quick Start, What is Beads?, Why Beads?, Working with Issues

### Community 28
Cohesion: 0.32
Nodes (6): add(), double(), boot(), Server, Server.start(), std

### Community 29
Cohesion: 0.33
Nodes (6): helper.h, boot(), run(), Server, Server.start(), stdio.h

### Community 30
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 31
Cohesion: 0.47
Nodes (4): /etc/profile, boot(), add(), double()

### Community 32
Cohesion: 0.47
Nodes (4): add(), boot(), Server, Server.start()

### Community 33
Cohesion: 0.50
Nodes (4): crate::util::math, boot(), Server, Server.start()

### Community 34
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), string

### Community 35
Cohesion: 0.50
Nodes (4): Boot(), Server, Server.Start(), System.Text

### Community 36
Cohesion: 0.50
Nodes (4): Server, Server.boot(), Server.start(), kotlin.math.sqrt

### Community 37
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), socket

### Community 38
Cohesion: 0.40
Nodes (5): Business Glossary, Business Glossary, Revenue, Analytics Knowledge Bundle, Analytics Knowledge Bundle

### Community 39
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), Psr\Log\LoggerInterface

### Community 40
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), json

### Community 41
Cohesion: 0.50
Nodes (4): boot(), Server, Server.start(), scala.collection.mutable.Map

### Community 42
Cohesion: 0.50
Nodes (3): Build / remove set, Deliberately skipped (agent-first), v0.5.0 — Agent-first delta

### Community 43
Cohesion: 0.67
Nodes (3): add(), M, M.double()

### Community 44
Cohesion: 0.50
Nodes (4): Tables, Tables, orders, orders

### Community 45
Cohesion: 0.67
Nodes (3): Add(), Calc, Calc.Sum()

### Community 46
Cohesion: 0.67
Nodes (3): add(), MathUtils, MathUtils.double()

### Community 47
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **352 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+347 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **8 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
