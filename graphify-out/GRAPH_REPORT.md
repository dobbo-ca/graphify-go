# Graph Report - .

## Summary
- 251 nodes · 564 edges · 12 communities
- Extraction: 55% EXTRACTED · 44% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `2c2e7464`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 14 edges
2. `MakeID()` - 14 edges
3. `cmdBuild()` - 13 edges
4. `layoutPositions()` - 12 edges
5. `extractTerraform()` - 12 edges
6. `Generate()` - 12 edges
7. `ToHTML()` - 11 edges
8. `builder.def()` - 11 edges
9. `Build()` - 11 edges
10. `buildNodeLevel()` - 10 edges

## Surprising Connections (you probably didn't know these)
- `Cluster()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `louvain()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `render()` --calls--> `ToHTML()`  [INFERRED]
  internal/export/html_test.go → internal/export/html.go  _bridges separate communities_
- `ToHTML()` --calls--> `layoutPositions()`  [INFERRED]
  internal/export/html.go → internal/export/layout.go  _bridges separate communities_
- `ToHTML()` --calls--> `Graph.NumEdges()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (12 total)

### Community 0
Cohesion: 0.07
Nodes (44): File(), TestExtractAndResolve(), Resolve(), resolveRelImport(), TestExtractTerraform(), github.com/dobbo-ca/graphify-go/internal/detect, github.com/dobbo-ca/graphify-go/internal/export, github.com/dobbo-ca/graphify-go/internal/extract (+36 more)

### Community 1
Cohesion: 0.13
Nodes (39): builder, builder.addNode(), builder.call(), builder.contains(), builder.def(), builder.imp(), Call, Def (+31 more)

### Community 2
Cohesion: 0.10
Nodes (35): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+27 more)

### Community 3
Cohesion: 0.11
Nodes (31): Cluster(), Cohesion(), less(), louvain(), reindexBySize(), Scores(), splitOversized(), render() (+23 more)

### Community 4
Cohesion: 0.09
Nodes (26): CollectFiles(), isSensitive(), encoding/json, jsonGraph, jsonLink, jsonNode, fmt, golang.org/x/text/cases (+18 more)

### Community 5
Cohesion: 0.19
Nodes (15): buildQuad(), layoutIters(), layoutPositions(), quad, quad.child(), quad.force(), quad.insert(), quad.subdivide() (+7 more)

### Community 6
Cohesion: 0.42
Nodes (8): dirScope(), extractTerraform(), firstIdent(), tfBlock(), tfChild(), tfChildNode(), tfRefAddress(), github.com/tree-sitter-grammars/tree-sitter-hcl/bindings/go

### Community 7
Cohesion: 0.43
Nodes (7): github.com/dobbo-ca/graphify-go/internal/analyze, base(), confidenceBreakdown(), Generate(), realLabels(), short(), sortedKeys()

### Community 8
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 9
Cohesion: 0.40
Nodes (5): express, ../util/math, boot(), Server, Server.start()

### Community 10
Cohesion: 0.33
Nodes (3): TestMakeID(), TestNormalizeIDMatchesMakeID(), testing

### Community 11
Cohesion: 0.67
Nodes (3): Add(), Calc, Calc.Sum()
