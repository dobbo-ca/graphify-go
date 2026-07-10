# Graph Report - /Users/cdobbyn/work/dobbo-ca/graphify-go-wt-parity-p0-a0fea0

## Summary
- 975 nodes · 2567 edges · 29 communities
- Extraction: 47% EXTRACTED · 52% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `3b52c2e6`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 80 edges
2. `MakeID()` - 62 edges
3. `errContext.Error()` - 58 edges
4. `FileFromBytes()` - 50 edges
5. `builder.def()` - 43 edges
6. `Resolve()` - 39 edges
7. `walk()` - 32 edges
8. `fieldText()` - 29 edges
9. `newBuilder()` - 21 edges
10. `builder.addNode()` - 21 edges

## Surprising Connections (you probably didn't know these)
- `TestSurprisingFindsCrossFileCall()` --calls--> `errContext.Error()`  [INFERRED]
  internal/analyze/analyze_test.go → internal/semantic/semantic_test.go  _bridges separate communities_
- `TestHashFileFastpath()` --calls--> `errContext.Error()`  [INFERRED]
  internal/cache/cache_test.go → internal/semantic/semantic_test.go  _bridges separate communities_
- `TestHashFileMissing()` --calls--> `errContext.Error()`  [INFERRED]
  internal/cache/cache_test.go → internal/semantic/semantic_test.go  _bridges separate communities_
- `TestStatIndexRoundTrip()` --calls--> `errContext.Error()`  [INFERRED]
  internal/cache/cache_test.go → internal/semantic/semantic_test.go  _bridges separate communities_
- `Cluster()` --calls--> `Graph.NumNodes()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (29 total)

### Community 0
Cohesion: 0.04
Nodes (177): builder.bashCommand(), builder.bashFunc(), builder.bashItems(), commandName(), extractBash(), firstArg(), builder.cCalls(), builder.cFunc() (+169 more)

### Community 1
Cohesion: 0.02
Nodes (148): bytes, Cache, Entry, HashBytes(), HashFile(), Load(), LoadStat(), Save() (+140 more)

### Community 2
Cohesion: 0.04
Nodes (84): TestExtractBash(), TestExtractC(), File(), FileFromBytes(), TestExtractAndResolve(), jsonHasEdge(), jsonNodes(), TestExtractJSONExtendsArray() (+76 more)

### Community 3
Cohesion: 0.05
Nodes (92): Cycle, entityLoc(), GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey() (+84 more)

### Community 4
Cohesion: 0.05
Nodes (28): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+20 more)

### Community 5
Cohesion: 0.07
Nodes (44): context, github.com/anthropics/anthropic-sdk-go, github.com/anthropics/anthropic-sdk-go/bedrock, github.com/dobbo-ca/graphify-go/internal/semantic, collectNotes(), enrich(), loadSemanticCache(), newSemanticBackend() (+36 more)

### Community 6
Cohesion: 0.10
Nodes (46): bufio, argInt(), argString(), cmdServe(), communitiesOf(), labelOrID(), mcpServer, mcpServer.callTool() (+38 more)

### Community 7
Cohesion: 0.09
Nodes (41): encoding/csv, encoding/xml, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge, gmlGraph (+33 more)

### Community 8
Cohesion: 0.11
Nodes (33): classifyList(), classifyScalar(), composeID(), exprRefAddress(), exprVarName(), labelInputs, normalizeSeg(), nullLabelInputs() (+25 more)

### Community 9
Cohesion: 0.12
Nodes (24): Affected(), AffectedResult, Graph.collect(), loadJSON(), TestAffectedInheritsContext(), TestAffectedNoMatch(), TestAffectedTransitive(), Diff() (+16 more)

### Community 10
Cohesion: 0.16
Nodes (26): math, Ask(), bfsTraverse(), computeIDF(), dfsTraverse(), Graph.neighbors(), hubThreshold(), isSearchable() (+18 more)

### Community 11
Cohesion: 0.11
Nodes (21): ancestorDirs(), globToRegex(), ignoreFile, ignorer, ignorer.ignored(), ignorer.load(), ignoreRule, newIgnorer() (+13 more)

### Community 12
Cohesion: 0.11
Nodes (20): builder, builder.callMember(), Call, Def, fileStem(), Imp, ImportAlias, itoa() (+12 more)

### Community 13
Cohesion: 0.24
Nodes (16): crate, IntrospectCargo(), loadTOML(), memberManifestPaths(), packageName(), hasEdge(), hasNode(), nodeIDs() (+8 more)

### Community 14
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 15
Cohesion: 0.38
Nodes (6): defs.svh, add(), alu, compute(), Counter, Counter.step()

### Community 16
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 18
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **84 isolated node(s):** `assembleStats`, `buildOpts`, `semanticOpts`, `mcpServer`, `rpcRequest` (+79 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **3 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
