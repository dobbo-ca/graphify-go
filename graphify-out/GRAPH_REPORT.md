# Graph Report - .

## Summary
- 629 nodes · 1652 edges · 17 communities
- Extraction: 48% EXTRACTED · 51% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `84ae9841`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 78 edges
2. `MakeID()` - 49 edges
3. `builder.def()` - 43 edges
4. `walk()` - 32 edges
5. `fieldText()` - 29 edges
6. `Resolve()` - 28 edges
7. `FileFromBytes()` - 24 edges
8. `newBuilder()` - 21 edges
9. `parseRoot()` - 19 edges
10. `builder.addNode()` - 19 edges

## Surprising Connections (you probably didn't know these)
- `CallflowFromJSON()` --calls--> `readGraphJSON()`  [INFERRED]
  internal/export/callflow.go → internal/export/formats.go  _bridges separate communities_
- `buildMeta()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_
- `buildNodeLevel()` --calls--> `Graph.Degree()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_
- `buildNodeLevel()` --calls--> `Graph.Edges()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_
- `buildNodeLevel()` --calls--> `Graph.NodeIDs()`  [INFERRED]
  internal/export/html.go → internal/model/model.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (17 total)

### Community 0
Cohesion: 0.04
Nodes (141): builder.bashCommand(), builder.bashFunc(), builder.bashItems(), commandName(), extractBash(), firstArg(), builder.cppCalls(), builder.cppFunc() (+133 more)

### Community 1
Cohesion: 0.03
Nodes (116): bytes, Cache, Entry, HashBytes(), Load(), Save(), crypto/sha256, CollectFiles() (+108 more)

### Community 2
Cohesion: 0.05
Nodes (86): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+78 more)

### Community 3
Cohesion: 0.05
Nodes (28): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+20 more)

### Community 4
Cohesion: 0.08
Nodes (30): TestExtractBash(), TestExtractC(), TestExtractCpp(), TestExtractCSharp(), File(), FileFromBytes(), TestExtractAndResolve(), TestExtractJulia() (+22 more)

### Community 5
Cohesion: 0.15
Nodes (20): encoding/csv, encoding/xml, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge, gmlGraph (+12 more)

### Community 6
Cohesion: 0.18
Nodes (19): buildMeta(), buildNodeLevel(), colorFor(), communityName(), communityNames(), groupPrio(), legendHTML(), legendRow (+11 more)

### Community 7
Cohesion: 0.28
Nodes (15): builder.luaAssign(), builder.luaCall(), builder.luaCalls(), builder.luaEnsureType(), builder.luaFunc(), builder.luaMethod(), builder.luaStatement(), builder.luaTopCalls() (+7 more)

### Community 8
Cohesion: 0.35
Nodes (10): builder.cCalls(), builder.cFunc(), builder.cInclude(), builder.cType(), cDeclName(), cIsFuncPointer(), cStringContent(), extractC() (+2 more)

### Community 9
Cohesion: 0.38
Nodes (10): builder.rubyCalls(), builder.rubyMethod(), builder.rubyRecordCall(), builder.rubyRequire(), builder.rubyStatement(), builder.rubyType(), extractRuby(), rubyConstName() (+2 more)

### Community 10
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 11
Cohesion: 0.40
Nodes (9): dirScope(), extractTerraform(), firstIdent(), tfAttrString(), tfBlock(), tfChild(), tfChildNode(), tfRefAddress() (+1 more)

### Community 12
Cohesion: 0.44
Nodes (8): builder.ktCalls(), builder.ktFunc(), builder.ktImport(), builder.ktType(), extractKotlin(), ktBody(), ktNavName(), github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go

### Community 13
Cohesion: 0.29
Nodes (6): golang.org/x/text/cases, golang.org/x/text/unicode/norm, clean(), NormalizeID(), TestMakeID(), TestNormalizeIDMatchesMakeID()

### Community 14
Cohesion: 0.38
Nodes (6): defs.svh, add(), alu, compute(), Counter, Counter.step()

### Community 15
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region
