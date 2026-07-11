# Graph Report - .

## Summary
- 1271 nodes · 3066 edges · 35 communities
- Extraction: 53% EXTRACTED · 46% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `a55cf586`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 84 edges
2. `MakeID()` - 76 edges
3. `FileFromBytes()` - 59 edges
4. `github.com/dobbo-ca/graphify-go` - 52 edges
5. `Resolve()` - 47 edges
6. `builder.def()` - 45 edges
7. `fieldText()` - 33 edges
8. `walk()` - 32 edges
9. `main()` - 26 edges
10. `builder.addNode()` - 23 edges

## Surprising Connections (you probably didn't know these)
- `sampleGraph()` --calls--> `builder.contains()`  [INFERRED]
  internal/analyze/analyze_test.go → internal/extract/extract.go  _bridges separate communities_
- `CollectFilesReport()` --calls--> `ignorer.ignored()`  [INFERRED]
  internal/detect/detect.go → internal/detect/gitignore.go  _bridges separate communities_
- `CollectFilesReport()` --calls--> `newIgnorer()`  [INFERRED]
  internal/detect/detect.go → internal/detect/gitignore.go  _bridges separate communities_
- `conceptDir()` --calls--> `MakeID()`  [INFERRED]
  internal/export/okf.go → internal/idutil/idutil.go  _bridges separate communities_
- `conceptDoc()` --calls--> `SanitizeLabel()`  [INFERRED]
  internal/export/okf.go → internal/security/security.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (35 total)

### Community 0
Cohesion: 0.03
Nodes (186): builder.bashCommand(), builder.bashFunc(), builder.bashItems(), commandName(), extractBash(), firstArg(), builder.cCalls(), builder.cFunc() (+178 more)

### Community 1
Cohesion: 0.03
Nodes (127): bytes, Cache, Entry, HashBytes(), HashFile(), Load(), LoadStat(), Save() (+119 more)

### Community 2
Cohesion: 0.03
Nodes (102): TestExtractBash(), TestExtractC(), TestExtractVueComponent(), TestFileFromBytesMtsAsTypeScript(), TestFileFromBytesShebangBash(), File(), FileFromBytes(), fileStem() (+94 more)

### Community 3
Cohesion: 0.05
Nodes (98): Cycle, entityLoc(), GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey() (+90 more)

### Community 4
Cohesion: 0.05
Nodes (69): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+61 more)

### Community 5
Cohesion: 0.05
Nodes (61): context, fmt, github.com/anthropics/anthropic-sdk-go, github.com/anthropics/anthropic-sdk-go/bedrock, github.com/dobbo-ca/graphify-go/internal/cache, github.com/dobbo-ca/graphify-go/internal/semantic, collectNotes(), enrich() (+53 more)

### Community 6
Cohesion: 0.03
Nodes (56): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Agent Context Profiles, Beads Issue Tracker, Quick Reference, Rules (+48 more)

### Community 7
Cohesion: 0.05
Nodes (28): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+20 more)

### Community 8
Cohesion: 0.09
Nodes (51): bufio, encoding/json, github.com/dobbo-ca/graphify-go/internal/query, argInt(), argString(), cmdServe(), communitiesOf(), labelOrID() (+43 more)

### Community 9
Cohesion: 0.06
Nodes (46): CollectManifests(), ancestorDirs(), globToRegex(), ignoreFile, ignorer, ignorer.ignored(), ignorer.load(), ignoreRule (+38 more)

### Community 10
Cohesion: 0.04
Nodes (53): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+45 more)

### Community 11
Cohesion: 0.07
Nodes (40): net, net/url, Explanation, Graph, Graph.bfsPath(), Graph.resolve(), Link, Load() (+32 more)

### Community 12
Cohesion: 0.11
Nodes (28): Affected(), AffectedOptions, AffectedResult, Graph.collect(), impacted(), loadJSON(), TestAffectedDepth(), TestAffectedInheritsContext() (+20 more)

### Community 13
Cohesion: 0.16
Nodes (26): math, Ask(), bfsTraverse(), computeIDF(), dfsTraverse(), Graph.neighbors(), hubThreshold(), isSearchable() (+18 more)

### Community 14
Cohesion: 0.22
Nodes (19): concept, concept.link(), conceptDescription(), conceptDir(), conceptDoc(), OKFFromJSON(), parentDir(), relationsByNode() (+11 more)

### Community 15
Cohesion: 0.11
Nodes (17): Agent Context Profiles, Agent Instructions, Beads Issue Tracker, Beads Issue Tracker, Non-Interactive Shell Commands, Quick Reference, Quick Reference, Quick Reference (+9 more)

### Community 16
Cohesion: 0.22
Nodes (17): crate, IntrospectCargo(), loadTOML(), memberManifestPaths(), packageName(), hasEdge(), hasNode(), nodeIDs() (+9 more)

### Community 17
Cohesion: 0.11
Nodes (18): Authoritative cloudposse `id` algorithm (to reimplement), Background — current Terraform extraction, Confidence storage, Data model — full change surface, Design decisions, Implementation stages, Objective, Out of scope (+10 more)

### Community 18
Cohesion: 0.28
Nodes (15): builder.luaAssign(), builder.luaCall(), builder.luaCalls(), builder.luaEnsureType(), builder.luaFunc(), builder.luaMethod(), builder.luaStatement(), builder.luaTopCalls() (+7 more)

### Community 19
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 20
Cohesion: 0.22
Nodes (8): Beads - AI-Native Issue Tracking, Essential Commands, Get Started with Beads, Learn More, Quick Start, What is Beads?, Why Beads?, Working with Issues

### Community 21
Cohesion: 0.38
Nodes (6): defs.svh, add(), alu, compute(), Counter, Counter.step()

### Community 22
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 23
Cohesion: 0.40
Nodes (5): Business Glossary, Business Glossary, Revenue, Analytics Knowledge Bundle, Analytics Knowledge Bundle

### Community 24
Cohesion: 0.50
Nodes (3): 📚 Documentation, 🚀 Features, 🔧 Miscellaneous Tasks

### Community 25
Cohesion: 0.50
Nodes (3): Build / remove set, Deliberately skipped (agent-first), v0.5.0 — Agent-first delta

### Community 26
Cohesion: 0.50
Nodes (4): Tables, Tables, orders, orders

### Community 27
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **239 isolated node(s):** `First Step`, `Preferred Route`, `Core CLI Workflow`, `What Belongs In Beads`, `Rules` (+234 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **1 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
