# Graph Report - .

## Summary
- 882 nodes · 2273 edges · 30 communities
- Extraction: 48% EXTRACTED · 51% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `223ed8fc`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 79 edges
2. `MakeID()` - 56 edges
3. `builder.def()` - 43 edges
4. `FileFromBytes()` - 41 edges
5. `Resolve()` - 39 edges
6. `walk()` - 32 edges
7. `fieldText()` - 29 edges
8. `newBuilder()` - 21 edges
9. `newServer()` - 20 edges
10. `builder.addNode()` - 20 edges

## Surprising Connections (you probably didn't know these)
- `conceptDir()` --calls--> `MakeID()`  [INFERRED]
  internal/export/okf.go → internal/idutil/idutil.go  _bridges separate communities_
- `conceptDoc()` --calls--> `SanitizeLabel()`  [INFERRED]
  internal/export/okf.go → internal/security/security.go  _bridges separate communities_
- `OKFFromJSON()` --calls--> `readGraphJSON()`  [INFERRED]
  internal/export/okf.go → internal/export/formats.go  _bridges separate communities_
- `OKFFromJSON()` --calls--> `MakeID()`  [INFERRED]
  internal/export/okf.go → internal/idutil/idutil.go  _bridges separate communities_
- `relationsByNode()` --calls--> `SanitizeLabel()`  [INFERRED]
  internal/export/okf.go → internal/security/security.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (30 total)

### Community 0
Cohesion: 0.04
Nodes (158): builder.bashCommand(), builder.bashFunc(), builder.bashItems(), commandName(), extractBash(), firstArg(), builder.cCalls(), builder.cFunc() (+150 more)

### Community 1
Cohesion: 0.03
Nodes (116): bytes, Cache, Entry, HashBytes(), HashFile(), Load(), LoadStat(), Save() (+108 more)

### Community 2
Cohesion: 0.04
Nodes (76): TestExtractBash(), TestExtractC(), File(), FileFromBytes(), fileStem(), TestExtractAndResolve(), TestExtractJava(), TestExtractJulia() (+68 more)

### Community 3
Cohesion: 0.05
Nodes (85): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+77 more)

### Community 4
Cohesion: 0.05
Nodes (28): boot(), crate::util::math, /etc/profile, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface (+20 more)

### Community 5
Cohesion: 0.10
Nodes (46): bufio, github.com/dobbo-ca/graphify-go/internal/query, argInt(), argString(), cmdServe(), communitiesOf(), labelOrID(), mcpServer (+38 more)

### Community 6
Cohesion: 0.09
Nodes (34): encoding/json, fmt, github.com/dobbo-ca/graphify-go/internal/security, linkKey, loadRawGraph(), Merge(), nodeKey, rawGraph (+26 more)

### Community 7
Cohesion: 0.12
Nodes (32): classifyList(), classifyScalar(), composeID(), exprRefAddress(), exprVarName(), labelInputs, normalizeSeg(), nullLabelInputs() (+24 more)

### Community 8
Cohesion: 0.12
Nodes (24): Affected(), AffectedResult, Graph.collect(), loadJSON(), TestAffectedInheritsContext(), TestAffectedNoMatch(), TestAffectedTransitive(), Diff() (+16 more)

### Community 9
Cohesion: 0.16
Nodes (26): math, Ask(), bfsTraverse(), computeIDF(), dfsTraverse(), Graph.neighbors(), hubThreshold(), isSearchable() (+18 more)

### Community 10
Cohesion: 0.19
Nodes (21): concept, concept.link(), conceptDescription(), conceptDir(), conceptDoc(), OKFFromJSON(), parentDir(), relation (+13 more)

### Community 11
Cohesion: 0.15
Nodes (19): encoding/csv, encoding/xml, CSVFromJSON(), DOTFromJSON(), dotQuote(), gmlData, gmlEdge, gmlGraph (+11 more)

### Community 12
Cohesion: 0.24
Nodes (16): crate, IntrospectCargo(), loadTOML(), memberManifestPaths(), packageName(), hasEdge(), hasNode(), nodeIDs() (+8 more)

### Community 13
Cohesion: 0.28
Nodes (15): builder.luaAssign(), builder.luaCall(), builder.luaCalls(), builder.luaEnsureType(), builder.luaFunc(), builder.luaMethod(), builder.luaStatement(), builder.luaTopCalls() (+7 more)

### Community 14
Cohesion: 0.13
Nodes (14): builder, builder.callMember(), Call, Def, Imp, ImportAlias, itoa(), ModInvoke (+6 more)

### Community 15
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 16
Cohesion: 0.29
Nodes (6): golang.org/x/text/cases, golang.org/x/text/unicode/norm, clean(), NormalizeID(), TestMakeID(), TestNormalizeIDMatchesMakeID()

### Community 17
Cohesion: 0.38
Nodes (6): defs.svh, add(), alu, compute(), Counter, Counter.step()

### Community 18
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 20
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null

## Knowledge Gaps
- **71 isolated node(s):** `assembleStats`, `mcpServer`, `rpcRequest`, `rpcResponse`, `rpcError` (+66 more)
  These have <=1 connection - possible missing edges or undocumented components.
- **3 thin communities (<3 nodes) omitted from report** - run `graphify query` to explore isolated nodes.
