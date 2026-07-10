# Graph Report - .

## Summary
- 1220 nodes · 2944 edges · 30 communities
- Extraction: 51% EXTRACTED · 48% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `3a17a243`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 84 edges
2. `MakeID()` - 74 edges
3. `FileFromBytes()` - 57 edges
4. `github.com/dobbo-ca/graphify-go` - 52 edges
5. `builder.def()` - 43 edges
6. `Resolve()` - 43 edges
7. `fieldText()` - 33 edges
8. `walk()` - 32 edges
9. `builder.addNode()` - 23 edges
10. `newServer()` - 22 edges

## Surprising Connections (you probably didn't know these)
- `conceptDir()` --calls--> `MakeID()`  [INFERRED]
  internal/export/okf.go → internal/idutil/idutil.go  _bridges separate communities_
- `conceptDoc()` --calls--> `SanitizeLabel()`  [INFERRED]
  internal/export/okf.go → internal/security/security.go  _bridges separate communities_
- `OKFFromJSON()` --calls--> `readGraphJSON()`  [INFERRED]
  internal/export/okf.go → internal/export/formats.go  _bridges separate communities_
- `OKFFromJSON()` --calls--> `MakeID()`  [INFERRED]
  internal/export/okf.go → internal/idutil/idutil.go  _bridges separate communities_
- `relationsByNode()` --calls--> `SanitizeLabel()`  [INFERRED]
  internal/export/okf.go → internal/security/security.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (30 total)

### Community 0
Cohesion: 0.04
Nodes (163): builder.bashCommand(), builder.bashFunc(), builder.bashItems(), commandName(), extractBash(), firstArg(), builder.cCalls(), builder.cFunc() (+155 more)

### Community 1
Cohesion: 0.03
Nodes (114): bytes, Cache, Entry, HashBytes(), HashFile(), Load(), LoadStat(), Save() (+106 more)

### Community 2
Cohesion: 0.04
Nodes (108): Cycle, entityLoc(), GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey() (+100 more)

### Community 3
Cohesion: 0.03
Nodes (96): TestExtractBash(), TestExtractC(), TestExtractVueComponent(), TestFileFromBytesMtsAsTypeScript(), TestFileFromBytesShebangBash(), File(), FileFromBytes(), fileStem() (+88 more)

### Community 4
Cohesion: 0.05
Nodes (76): bufio, encoding/json, github.com/dobbo-ca/graphify-go/internal/query, argInt(), argString(), cmdServe(), communitiesOf(), labelOrID() (+68 more)

### Community 5
Cohesion: 0.05
Nodes (61): context, fmt, github.com/anthropics/anthropic-sdk-go, github.com/anthropics/anthropic-sdk-go/bedrock, github.com/dobbo-ca/graphify-go/internal/cache, github.com/dobbo-ca/graphify-go/internal/semantic, collectNotes(), enrich() (+53 more)

### Community 6
Cohesion: 0.05
Nodes (28): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+20 more)

### Community 7
Cohesion: 0.04
Nodes (53): github.com/anthropics/anthropic-sdk-go, github.com/aws/aws-sdk-go-v2, github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream, github.com/aws/aws-sdk-go-v2/config, github.com/aws/aws-sdk-go-v2/credentials, github.com/aws/aws-sdk-go-v2/feature/ec2/imds, github.com/aws/aws-sdk-go-v2/internal/configsources, github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 (+45 more)

### Community 8
Cohesion: 0.08
Nodes (48): builder.luaAssign(), builder.luaCall(), builder.luaCalls(), builder.luaEnsureType(), builder.luaFunc(), builder.luaMethod(), builder.luaStatement(), builder.luaTopCalls() (+40 more)

### Community 9
Cohesion: 0.07
Nodes (42): CollectFiles(), CollectFilesReport(), CollectManifests(), genericKeywordHit(), isASCIIAlnum(), isASCIIAlpha(), IsSensitive(), ShebangExt() (+34 more)

### Community 10
Cohesion: 0.08
Nodes (33): encoding/csv, encoding/xml, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge, gmlGraph (+25 more)

### Community 11
Cohesion: 0.09
Nodes (34): componentLangPtr(), extractComponent(), lineOf(), maskComponentScript(), builder, builder.addRationale(), builder.callMember(), Call (+26 more)

### Community 12
Cohesion: 0.06
Nodes (28): 1. Think Before Coding, 2. Simplicity First, 3. Surgical Changes, 4. Goal-Driven Execution, Use the knowledge graph, not grep, Correctness / coverage, Done, Follow-ups (+20 more)

### Community 13
Cohesion: 0.12
Nodes (27): Affected(), AffectedOptions, AffectedResult, Graph.collect(), impacted(), loadJSON(), TestAffectedDepth(), TestAffectedInheritsContext() (+19 more)

### Community 14
Cohesion: 0.16
Nodes (26): math, Ask(), bfsTraverse(), computeIDF(), dfsTraverse(), Graph.neighbors(), hubThreshold(), isSearchable() (+18 more)

### Community 15
Cohesion: 0.21
Nodes (20): concept, concept.link(), conceptDescription(), conceptDir(), conceptDoc(), OKFFromJSON(), parentDir(), relation (+12 more)

### Community 16
Cohesion: 0.11
Nodes (18): Authoritative cloudposse `id` algorithm (to reimplement), Background — current Terraform extraction, Confidence storage, Data model — full change surface, Design decisions, Implementation stages, Objective, Out of scope (+10 more)

### Community 17
Cohesion: 0.11
Nodes (17): Final verification (run before opening a PR), Reference: cloudposse `id` algorithm (what composeID reimplements), Reference: tree-sitter-hcl v1.2.0 node shapes (verified empirically), Self-review notes (author), Stage A — Foundation: null-label marker + inherits_context edge, Stage B — Single-block literal name reconstruction, Stage C — Whole-corpus context-chain reconstruction, Task A1: `isNullLabel` helper + `[null-label]` marker on module nodes (+9 more)

### Community 18
Cohesion: 0.24
Nodes (16): crate, IntrospectCargo(), loadTOML(), memberManifestPaths(), packageName(), hasEdge(), hasNode(), nodeIDs() (+8 more)

### Community 19
Cohesion: 0.14
Nodes (13): Effort / sequencing, Explicitly out of scope (don't pull these in), Gap table, Implementation plan (single follow-up session), Key references, Phase 1 — get the data onto the page, Phase 2 — sidebar UI (template), Phase 3 — selection behavior (+5 more)

### Community 20
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

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
- **220 isolated node(s):** `📚 Documentation`, `🔧 Miscellaneous Tasks`, `🚀 Features`, `1. Think Before Coding`, `2. Simplicity First` (+215 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **2 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
