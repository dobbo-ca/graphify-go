# Graph Report - .

## Summary
- 281 nodes · 660 edges · 10 communities
- Extraction: 53% EXTRACTED · 46% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `02763849`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 23 edges
2. `MakeID()` - 19 edges
3. `builder.def()` - 15 edges
4. `cmdBuild()` - 13 edges
5. `fieldText()` - 13 edges
6. `layoutPositions()` - 12 edges
7. `File()` - 12 edges
8. `extractTerraform()` - 12 edges
9. `Generate()` - 12 edges
10. `ToHTML()` - 11 edges

## Surprising Connections (you probably didn't know these)
- `Cluster()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `louvain()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/cluster/cluster.go → internal/model/model.go  _bridges separate communities_
- `buildMeta()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_
- `buildNodeLevel()` --calls--> `Graph.Degree()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_
- `buildNodeLevel()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (10 total)

### Community 0
Cohesion: 0.10
Nodes (54): builder, builder.addNode(), builder.call(), builder.contains(), builder.def(), builder.imp(), Call, Def (+46 more)

### Community 1
Cohesion: 0.07
Nodes (43): encoding/json, jsonGraph, jsonLink, jsonNode, github.com/dobbo-ca/graphify-go/internal/detect, github.com/dobbo-ca/graphify-go/internal/export, github.com/dobbo-ca/graphify-go/internal/extract, github.com/dobbo-ca/graphify-go/internal/graph (+35 more)

### Community 2
Cohesion: 0.09
Nodes (38): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+30 more)

### Community 3
Cohesion: 0.12
Nodes (30): Cluster(), Cohesion(), less(), louvain(), reindexBySize(), Scores(), splitOversized(), render() (+22 more)

### Community 4
Cohesion: 0.09
Nodes (30): buildMeta(), buildNodeLevel(), colorFor(), communityName(), communityNames(), groupPrio(), legendHTML(), legendRow (+22 more)

### Community 5
Cohesion: 0.20
Nodes (13): dirScope(), extractTerraform(), firstIdent(), tfBlock(), tfChild(), tfChildNode(), tfRefAddress(), github.com/dobbo-ca/graphify-go/internal/idutil (+5 more)

### Community 6
Cohesion: 0.17
Nodes (9): crate::util::math, express, ../util/math, add(), Calc, Calc.Sum(), boot(), Server (+1 more)

### Community 7
Cohesion: 0.18
Nodes (11): CollectFiles(), isSensitive(), golang.org/x/text/cases, golang.org/x/text/unicode/norm, clean(), NormalizeID(), TestMakeID(), TestNormalizeIDMatchesMakeID() (+3 more)

### Community 8
Cohesion: 0.22
Nodes (10): File(), TestExtractAndResolve(), TestExtractPython(), Resolve(), resolveRelImport(), TestExtractRust(), TestExtractTerraform(), cmdExtract() (+2 more)

### Community 9
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region
