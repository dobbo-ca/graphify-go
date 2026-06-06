# Graph Report - .

## Summary
- 195 nodes · 425 edges · 9 communities
- Extraction: 56% EXTRACTED · 43% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `14cf96de`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `cmdBuild()` - 13 edges
2. `line()` - 13 edges
3. `MakeID()` - 13 edges
4. `Generate()` - 12 edges
5. `Build()` - 11 edges
6. `builder.def()` - 10 edges
7. `main()` - 9 edges
8. `builder.jsStatement()` - 9 edges
9. `extractGo()` - 8 edges
10. `builder.goMethod()` - 8 edges

## Surprising Connections (you probably didn't know these)
- `Cluster()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `louvain()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `ToJSON()` --calls--> `NodeCommunity()`  [INFERRED]
  internal/export/export.go → internal/cluster/cluster.go  _bridges separate communities_
- `ToJSON()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/export/export.go → internal/model/model.go  _bridges separate communities_
- `ToJSON()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/export/export.go → internal/model/model.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (9 total)

### Community 0
Cohesion: 0.08
Nodes (37): encoding/json, fmt, github.com/dobbo-ca/graphify-go/internal/cluster, github.com/dobbo-ca/graphify-go/internal/detect, github.com/dobbo-ca/graphify-go/internal/export, github.com/dobbo-ca/graphify-go/internal/extract, github.com/dobbo-ca/graphify-go/internal/graph, github.com/dobbo-ca/graphify-go/internal/query (+29 more)

### Community 1
Cohesion: 0.13
Nodes (38): builder, builder.addNode(), builder.call(), builder.contains(), builder.def(), builder.imp(), Call, Def (+30 more)

### Community 2
Cohesion: 0.11
Nodes (29): Cluster(), Cohesion(), less(), louvain(), reindexBySize(), Scores(), splitOversized(), github.com/dobbo-ca/graphify-go/internal/model (+21 more)

### Community 3
Cohesion: 0.15
Nodes (22): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+14 more)

### Community 4
Cohesion: 0.14
Nodes (17): CollectFiles(), isSensitive(), io/fs, net, net/url, os, path/filepath, regexp (+9 more)

### Community 5
Cohesion: 0.19
Nodes (11): jsonGraph, jsonLink, jsonNode, normLabel(), ToJSON(), golang.org/x/text/cases, golang.org/x/text/unicode/norm, clean() (+3 more)

### Community 6
Cohesion: 0.18
Nodes (10): File(), TestExtractAndResolve(), Resolve(), resolveRelImport(), github.com/dobbo-ca/graphify-go/internal/idutil, cmdExtract(), TestMakeID(), TestNormalizeIDMatchesMakeID() (+2 more)

### Community 7
Cohesion: 0.40
Nodes (5): express, ../util/math, boot(), Server, Server.start()

### Community 8
Cohesion: 0.67
Nodes (3): Add(), Calc, Calc.Sum()
