# Graph Report - .

## Summary
- 900 nodes · 2329 edges · 29 communities
- Extraction: 48% EXTRACTED · 51% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `427d8d34`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 80 edges
2. `MakeID()` - 61 edges
3. `FileFromBytes()` - 50 edges
4. `builder.def()` - 43 edges
5. `Resolve()` - 39 edges
6. `walk()` - 32 edges
7. `fieldText()` - 29 edges
8. `newBuilder()` - 21 edges
9. `builder.addNode()` - 21 edges
10. `newServer()` - 20 edges

## Surprising Connections (you probably didn't know these)
- `CollectFiles()` --calls--> `ignorer.ignored()`  [INFERRED]
  internal/detect/detect.go → internal/detect/gitignore.go  _bridges separate communities_
- `CollectFiles()` --calls--> `newIgnorer()`  [INFERRED]
  internal/detect/detect.go → internal/detect/gitignore.go  _bridges separate communities_
- `TestCollectFilesIncludesMCPConfigs()` --calls--> `CollectFiles()`  [INFERRED]
  internal/detect/detect_test.go → internal/detect/detect.go  _bridges separate communities_
- `TestCollectFilesSkipsTerraform()` --calls--> `CollectFiles()`  [INFERRED]
  internal/detect/detect_test.go → internal/detect/detect.go  _bridges separate communities_
- `TestCollectFilesGitignore()` --calls--> `CollectFiles()`  [INFERRED]
  internal/detect/gitignore_test.go → internal/detect/detect.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (29 total)

### Community 0
Cohesion: 0.04
Nodes (163): builder.bashCommand(), builder.bashFunc(), builder.bashItems(), commandName(), extractBash(), firstArg(), builder.cCalls(), builder.cFunc() (+155 more)

### Community 1
Cohesion: 0.03
Nodes (79): isSensitive(), mustWrite(), TestCollectFilesIncludesMCPConfigs(), TestCollectFilesSkipsTerraform(), ancestorDirs(), globToRegex(), ignoreFile, ignorer (+71 more)

### Community 2
Cohesion: 0.04
Nodes (76): TestExtractBash(), TestExtractC(), File(), FileFromBytes(), TestExtractAndResolve(), jsonHasEdge(), jsonNodes(), TestExtractJSONExtendsArray() (+68 more)

### Community 3
Cohesion: 0.05
Nodes (83): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+75 more)

### Community 4
Cohesion: 0.05
Nodes (68): bytes, Cache, Entry, HashBytes(), HashFile(), Load(), LoadStat(), Save() (+60 more)

### Community 5
Cohesion: 0.06
Nodes (66): bufio, encoding/json, github.com/dobbo-ca/graphify-go/internal/query, cmdPath(), argInt(), argString(), cmdServe(), communitiesOf() (+58 more)

### Community 6
Cohesion: 0.05
Nodes (28): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+20 more)

### Community 7
Cohesion: 0.09
Nodes (40): encoding/csv, encoding/xml, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge, gmlGraph (+32 more)

### Community 8
Cohesion: 0.12
Nodes (32): classifyList(), classifyScalar(), composeID(), exprRefAddress(), exprVarName(), labelInputs, normalizeSeg(), nullLabelInputs() (+24 more)

### Community 9
Cohesion: 0.12
Nodes (24): Affected(), AffectedResult, Graph.collect(), loadJSON(), TestAffectedInheritsContext(), TestAffectedNoMatch(), TestAffectedTransitive(), Diff() (+16 more)

### Community 10
Cohesion: 0.16
Nodes (26): math, Ask(), bfsTraverse(), computeIDF(), dfsTraverse(), Graph.neighbors(), hubThreshold(), isSearchable() (+18 more)

### Community 11
Cohesion: 0.24
Nodes (16): crate, IntrospectCargo(), loadTOML(), memberManifestPaths(), packageName(), hasEdge(), hasNode(), nodeIDs() (+8 more)

### Community 12
Cohesion: 0.24
Nodes (15): fmt, linkKey, loadRawGraph(), Merge(), nodeKey, rawGraph, loadMerged(), TestMergeKeepsBothDirectionsAndRelations() (+7 more)

### Community 13
Cohesion: 0.28
Nodes (15): builder.luaAssign(), builder.luaCall(), builder.luaCalls(), builder.luaEnsureType(), builder.luaFunc(), builder.luaMethod(), builder.luaStatement(), builder.luaTopCalls() (+7 more)

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
- **71 isolated node(s):** `assembleStats`, `mcpServer`, `rpcRequest`, `rpcResponse`, `rpcError` (+66 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **3 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
