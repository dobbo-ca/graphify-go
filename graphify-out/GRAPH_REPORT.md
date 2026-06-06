# Graph Report - .

## Summary
- 204 nodes · 442 edges · 10 communities
- Extraction: 56% EXTRACTED · 43% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `8dca625f`
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
9. `buildNodeLevel()` - 8 edges
10. `extractGo()` - 8 edges

## Surprising Connections (you probably didn't know these)
- `GodNodes()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/analyze/analyze.go → internal/model/model.go  _bridges separate communities_
- `ImportCycles()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/analyze/analyze.go → internal/model/model.go  _bridges separate communities_
- `Surprising()` --calls--> `NodeCommunity()`  [INFERRED]
  internal/analyze/analyze.go → internal/cluster/cluster.go  _bridges separate communities_
- `Surprising()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/analyze/analyze.go → internal/model/model.go  _bridges separate communities_
- `buildNodeLevel()` --calls--> `Graph.Degree()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (10 total)

### Community 0
Cohesion: 0.13
Nodes (38): builder, builder.addNode(), builder.call(), builder.contains(), builder.def(), builder.imp(), Call, Def (+30 more)

### Community 1
Cohesion: 0.10
Nodes (35): Cluster(), Cohesion(), less(), louvain(), NodeCommunity(), reindexBySize(), Scores(), splitOversized() (+27 more)

### Community 2
Cohesion: 0.09
Nodes (34): github.com/dobbo-ca/graphify-go/internal/detect, github.com/dobbo-ca/graphify-go/internal/export, github.com/dobbo-ca/graphify-go/internal/extract, github.com/dobbo-ca/graphify-go/internal/graph, github.com/dobbo-ca/graphify-go/internal/query, github.com/dobbo-ca/graphify-go/internal/report, arg(), cmdExplain() (+26 more)

### Community 3
Cohesion: 0.18
Nodes (17): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+9 more)

### Community 4
Cohesion: 0.17
Nodes (12): File(), TestExtractAndResolve(), Resolve(), resolveRelImport(), github.com/dobbo-ca/graphify-go/internal/idutil, github.com/dobbo-ca/graphify-go/internal/model, crossLanguage(), cmdExtract() (+4 more)

### Community 5
Cohesion: 0.14
Nodes (13): encoding/json, jsonGraph, jsonLink, jsonNode, normLabel(), legendRow, vedge, vfont (+5 more)

### Community 6
Cohesion: 0.18
Nodes (13): fmt, net, net/url, os, isBlockedIP(), mustCIDR(), SanitizeLabel(), TestSanitizeLabel() (+5 more)

### Community 7
Cohesion: 0.18
Nodes (11): CollectFiles(), isSensitive(), golang.org/x/text/cases, golang.org/x/text/unicode/norm, clean(), NormalizeID(), TestMakeID(), TestNormalizeIDMatchesMakeID() (+3 more)

### Community 8
Cohesion: 0.40
Nodes (5): express, ../util/math, boot(), Server, Server.start()

### Community 9
Cohesion: 0.67
Nodes (3): Add(), Calc, Calc.Sum()
