# Graph Report - .

## Summary
- 234 nodes · 519 edges · 12 communities
- Extraction: 56% EXTRACTED · 43% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `1867212c`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 14 edges
2. `MakeID()` - 14 edges
3. `cmdBuild()` - 13 edges
4. `extractTerraform()` - 12 edges
5. `Generate()` - 12 edges
6. `builder.def()` - 11 edges
7. `Build()` - 11 edges
8. `ToHTML()` - 10 edges
9. `buildNodeLevel()` - 10 edges
10. `main()` - 9 edges

## Surprising Connections (you probably didn't know these)
- `GodNodes()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/analyze/analyze.go → internal/model/model.go  _bridges separate communities_
- `Cluster()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `louvain()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `ToJSON()` --calls--> `NodeCommunity()`  [INFERRED]
  internal/export/export.go → internal/cluster/cluster.go  _bridges separate communities_
- `ToJSON()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/export/export.go → internal/model/model.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (12 total)

### Community 0
Cohesion: 0.14
Nodes (36): builder, builder.addNode(), builder.call(), builder.contains(), builder.def(), builder.imp(), Call, Def (+28 more)

### Community 1
Cohesion: 0.10
Nodes (33): Cluster(), Cohesion(), less(), louvain(), reindexBySize(), Scores(), splitOversized(), render() (+25 more)

### Community 2
Cohesion: 0.08
Nodes (33): CollectFiles(), isSensitive(), fmt, io/fs, net, net/url, os, path/filepath (+25 more)

### Community 3
Cohesion: 0.14
Nodes (23): buildMeta(), buildNodeLevel(), colorFor(), communityName(), communityNames(), groupPrio(), legendHTML(), legendRow (+15 more)

### Community 4
Cohesion: 0.16
Nodes (20): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+12 more)

### Community 5
Cohesion: 0.18
Nodes (17): github.com/dobbo-ca/graphify-go/internal/detect, github.com/dobbo-ca/graphify-go/internal/export, github.com/dobbo-ca/graphify-go/internal/extract, github.com/dobbo-ca/graphify-go/internal/graph, github.com/dobbo-ca/graphify-go/internal/query, github.com/dobbo-ca/graphify-go/internal/report, arg(), cmdExplain() (+9 more)

### Community 6
Cohesion: 0.18
Nodes (12): encoding/json, jsonGraph, jsonLink, jsonNode, normLabel(), ToJSON(), golang.org/x/text/cases, golang.org/x/text/unicode/norm (+4 more)

### Community 7
Cohesion: 0.19
Nodes (9): File(), TestExtractAndResolve(), Resolve(), resolveRelImport(), TestExtractTerraform(), TestMakeID(), TestNormalizeIDMatchesMakeID(), path (+1 more)

### Community 8
Cohesion: 0.31
Nodes (10): parseRoot(), dirScope(), extractTerraform(), firstIdent(), tfBlock(), tfChild(), tfChildNode(), tfRefAddress() (+2 more)

### Community 9
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 10
Cohesion: 0.40
Nodes (5): express, ../util/math, boot(), Server, Server.start()

### Community 11
Cohesion: 0.67
Nodes (3): Add(), Calc, Calc.Sum()
