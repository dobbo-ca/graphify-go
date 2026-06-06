# Graph Report - .

## Summary
- 222 nodes · 482 edges · 12 communities
- Extraction: 56% EXTRACTED · 43% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `1d387d95`
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
8. `main()` - 9 edges
9. `builder.jsStatement()` - 9 edges
10. `buildNodeLevel()` - 8 edges

## Surprising Connections (you probably didn't know these)
- `GodNodes()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/analyze/analyze.go → internal/model/model.go  _bridges separate communities_
- `louvain()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `ToJSON()` --calls--> `NodeCommunity()`  [INFERRED]
  internal/export/export.go → internal/cluster/cluster.go  _bridges separate communities_
- `ToJSON()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/export/export.go → internal/model/model.go  _bridges separate communities_
- `buildMeta()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (12 total)

### Community 0
Cohesion: 0.08
Nodes (37): Cluster(), Cohesion(), less(), louvain(), reindexBySize(), Scores(), splitOversized(), jsonGraph (+29 more)

### Community 1
Cohesion: 0.13
Nodes (38): builder, builder.addNode(), builder.call(), builder.contains(), builder.def(), builder.imp(), Call, Def (+30 more)

### Community 2
Cohesion: 0.15
Nodes (21): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+13 more)

### Community 3
Cohesion: 0.13
Nodes (18): CollectFiles(), isSensitive(), github.com/dobbo-ca/graphify-go/internal/idutil, io/fs, net, net/url, os, path (+10 more)

### Community 4
Cohesion: 0.16
Nodes (17): Explain(), Explanation, Graph, Graph.resolve(), Link, Load(), loc(), Match (+9 more)

### Community 5
Cohesion: 0.19
Nodes (16): github.com/dobbo-ca/graphify-go/internal/detect, github.com/dobbo-ca/graphify-go/internal/export, github.com/dobbo-ca/graphify-go/internal/extract, github.com/dobbo-ca/graphify-go/internal/graph, github.com/dobbo-ca/graphify-go/internal/query, github.com/dobbo-ca/graphify-go/internal/report, arg(), cmdExplain() (+8 more)

### Community 6
Cohesion: 0.15
Nodes (13): File(), TestExtractAndResolve(), Resolve(), resolveRelImport(), TestExtractTerraform(), golang.org/x/text/cases, golang.org/x/text/unicode/norm, cmdExtract() (+5 more)

### Community 7
Cohesion: 0.18
Nodes (14): encoding/json, buildMeta(), buildNodeLevel(), colorFor(), legendHTML(), legendRow, legendRows(), vedge (+6 more)

### Community 8
Cohesion: 0.42
Nodes (8): dirScope(), extractTerraform(), firstIdent(), tfBlock(), tfChild(), tfChildNode(), tfRefAddress(), github.com/tree-sitter-grammars/tree-sitter-hcl/bindings/go

### Community 9
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 10
Cohesion: 0.40
Nodes (5): express, ../util/math, boot(), Server, Server.start()

### Community 11
Cohesion: 0.67
Nodes (3): Add(), Calc, Calc.Sum()
