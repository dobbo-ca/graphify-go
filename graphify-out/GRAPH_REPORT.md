# Graph Report - .

## Summary
- 197 nodes · 430 edges · 9 communities
- Extraction: 56% EXTRACTED · 43% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `3e62a36f`
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
8. `ToHTML()` - 9 edges
9. `builder.jsStatement()` - 9 edges
10. `extractGo()` - 8 edges

## Surprising Connections (you probably didn't know these)
- `Cluster()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `louvain()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `ToHTML()` --calls--> `NodeCommunity()`  [INFERRED]
  internal/export/html.go → internal/cluster/cluster.go  _bridges separate communities_
- `ToHTML()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_
- `ToHTML()` --calls--> `Graph.NumNodes()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (9 total)

### Community 0
Cohesion: 0.13
Nodes (39): builder, builder.addNode(), builder.call(), builder.contains(), builder.def(), builder.imp(), Call, Def (+31 more)

### Community 1
Cohesion: 0.11
Nodes (29): Cluster(), Cohesion(), less(), louvain(), reindexBySize(), Scores(), splitOversized(), github.com/dobbo-ca/graphify-go/internal/model (+21 more)

### Community 2
Cohesion: 0.11
Nodes (28): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+20 more)

### Community 3
Cohesion: 0.13
Nodes (22): Resolve(), resolveRelImport(), github.com/dobbo-ca/graphify-go/internal/detect, github.com/dobbo-ca/graphify-go/internal/export, github.com/dobbo-ca/graphify-go/internal/extract, github.com/dobbo-ca/graphify-go/internal/graph, github.com/dobbo-ca/graphify-go/internal/idutil, github.com/dobbo-ca/graphify-go/internal/query (+14 more)

### Community 4
Cohesion: 0.15
Nodes (18): colorFor(), legendHTML(), ToHTML(), topNodes(), fmt, github.com/dobbo-ca/graphify-go/internal/security, net, net/url (+10 more)

### Community 5
Cohesion: 0.16
Nodes (17): Explain(), Explanation, Graph, Graph.resolve(), Link, Load(), loc(), Match (+9 more)

### Community 6
Cohesion: 0.14
Nodes (13): CollectFiles(), isSensitive(), TestExtractAndResolve(), golang.org/x/text/cases, golang.org/x/text/unicode/norm, clean(), NormalizeID(), TestMakeID() (+5 more)

### Community 7
Cohesion: 0.40
Nodes (5): express, ../util/math, boot(), Server, Server.start()

### Community 8
Cohesion: 0.67
Nodes (3): Add(), Calc, Calc.Sum()
