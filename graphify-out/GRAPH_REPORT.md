# Graph Report - .

## Summary
- 300 nodes · 731 edges · 9 communities
- Extraction: 51% EXTRACTED · 48% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `7ed2480b`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 23 edges
2. `MakeID()` - 19 edges
3. `builder.def()` - 15 edges
4. `Cluster()` - 14 edges
5. `Generate()` - 14 edges
6. `cmdBuild()` - 13 edges
7. `fieldText()` - 13 edges
8. `layoutPositions()` - 12 edges
9. `File()` - 12 edges
10. `extractTerraform()` - 12 edges

## Surprising Connections (you probably didn't know these)
- `GodNodes()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/analyze/analyze.go → internal/model/model.go  _bridges separate communities_
- `Surprising()` --calls--> `NodeCommunity()`  [INFERRED]
  internal/analyze/analyze.go → internal/cluster/cluster.go  _bridges separate communities_
- `sampleGraph()` --calls--> `builder.contains()`  [INFERRED]
  internal/analyze/analyze_test.go → internal/extract/extract.go  _bridges separate communities_
- `TestGodNodesExcludeFileHubs()` --calls--> `GodNodes()`  [INFERRED]
  internal/analyze/analyze_test.go → internal/analyze/analyze.go  _bridges separate communities_
- `TestImportCyclesDetectsMutualImport()` --calls--> `ImportCycles()`  [INFERRED]
  internal/analyze/analyze_test.go → internal/analyze/analyze.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (9 total)

### Community 0
Cohesion: 0.10
Nodes (54): builder, builder.addNode(), builder.call(), builder.contains(), builder.def(), builder.imp(), Call, Def (+46 more)

### Community 1
Cohesion: 0.09
Nodes (44): sampleGraph(), TestGodNodesExcludeFileHubs(), TestImportCyclesDetectsMutualImport(), TestImportCyclesNoneWhenAcyclic(), TestSurprisingFindsCrossFileCall(), Cluster(), Cohesion(), less() (+36 more)

### Community 2
Cohesion: 0.08
Nodes (39): github.com/dobbo-ca/graphify-go/internal/detect, github.com/dobbo-ca/graphify-go/internal/export, github.com/dobbo-ca/graphify-go/internal/extract, github.com/dobbo-ca/graphify-go/internal/graph, github.com/dobbo-ca/graphify-go/internal/query, github.com/dobbo-ca/graphify-go/internal/report, github.com/dobbo-ca/graphify-go/internal/security, arg() (+31 more)

### Community 3
Cohesion: 0.10
Nodes (33): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+25 more)

### Community 4
Cohesion: 0.09
Nodes (33): buildMeta(), buildNodeLevel(), colorFor(), communityName(), communityNames(), groupPrio(), legendHTML(), legendRow (+25 more)

### Community 5
Cohesion: 0.11
Nodes (22): File(), TestExtractAndResolve(), TestExtractPython(), Resolve(), resolveRelImport(), TestExtractRust(), dirScope(), extractTerraform() (+14 more)

### Community 6
Cohesion: 0.11
Nodes (19): CollectFiles(), isSensitive(), encoding/json, jsonGraph, jsonLink, jsonNode, normLabel(), ToJSON() (+11 more)

### Community 7
Cohesion: 0.17
Nodes (9): crate::util::math, express, ../util/math, add(), Calc, Calc.Sum(), boot(), Server (+1 more)

### Community 8
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region
