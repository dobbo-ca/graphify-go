# Graph Report - .

## Summary
- 678 nodes · 1772 edges · 18 communities
- Extraction: 48% EXTRACTED · 51% INFERRED · 0% AMBIGUOUS

## Graph Freshness
- Built from commit: `c4d1bb99`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify build .` after code changes to rebuild.

## God Nodes (most connected - your core abstractions)
1. `line()` - 78 edges
2. `MakeID()` - 51 edges
3. `builder.def()` - 43 edges
4. `Resolve()` - 35 edges
5. `FileFromBytes()` - 33 edges
6. `walk()` - 32 edges
7. `fieldText()` - 29 edges
8. `newBuilder()` - 21 edges
9. `parseRoot()` - 19 edges
10. `builder.addNode()` - 19 edges

## Surprising Connections (you probably didn't know these)
- `CollectFiles()` --calls--> `ignorer.ignored()`  [INFERRED]
  internal/detect/detect.go → internal/detect/gitignore.go  _bridges separate communities_
- `CollectFiles()` --calls--> `newIgnorer()`  [INFERRED]
  internal/detect/detect.go → internal/detect/gitignore.go  _bridges separate communities_
- `TestCollectFilesSkipsTerraform()` --calls--> `CollectFiles()`  [INFERRED]
  internal/detect/detect_test.go → internal/detect/detect.go  _bridges separate communities_
- `builder.bashCommand()` --calls--> `builder.call()`  [INFERRED]
  internal/extract/bash.go → internal/extract/extract.go  _bridges separate communities_
- `builder.bashCommand()` --calls--> `line()`  [INFERRED]
  internal/extract/bash.go → internal/extract/extract.go  _bridges separate communities_

## Import Cycles
- None detected.

## Communities (18 total)

### Community 0
Cohesion: 0.04
Nodes (108): Cycle, GodNode, GodNodes(), ImportCycles(), isConceptNode(), isFileNode(), rotateKey(), Surprise (+100 more)

### Community 1
Cohesion: 0.04
Nodes (85): bytes, mustWrite(), TestCollectFilesSkipsTerraform(), encoding/csv, encoding/json, encoding/xml, CallflowFromJSON(), communityLabel() (+77 more)

### Community 2
Cohesion: 0.07
Nodes (91): builder.bashFunc(), builder.cCalls(), builder.cFunc(), builder.cInclude(), builder.cType(), cDeclName(), cIsFuncPointer(), cStringContent() (+83 more)

### Community 3
Cohesion: 0.05
Nodes (66): builder.bashCommand(), builder.bashItems(), commandName(), extractBash(), firstArg(), builder.cppInclude(), builder.cppItems(), builder.cppUsing() (+58 more)

### Community 4
Cohesion: 0.08
Nodes (35): TestExtractBash(), TestExtractC(), File(), FileFromBytes(), TestExtractAndResolve(), TestExtractJulia(), TestExtractKotlin(), composeFromHCL() (+27 more)

### Community 5
Cohesion: 0.08
Nodes (42): Cache, Entry, HashBytes(), Load(), Save(), crypto/sha256, CollectFiles(), isSensitive() (+34 more)

### Community 6
Cohesion: 0.12
Nodes (32): classifyList(), classifyScalar(), composeID(), exprRefAddress(), exprVarName(), labelInputs, normalizeSeg(), nullLabelInputs() (+24 more)

### Community 7
Cohesion: 0.10
Nodes (18): crate::util::math, express, helper.h, json, kotlin.math.sqrt, Psr\Log\LoggerInterface, scala.collection.mutable.Map, socket (+10 more)

### Community 8
Cohesion: 0.10
Nodes (10): boot(), /etc/profile, add(), Calc, Calc.Sum(), double(), M, M.double() (+2 more)

### Community 9
Cohesion: 0.14
Nodes (15): ancestorDirs(), globToRegex(), ignoreFile, ignorer, ignorer.ignored(), ignorer.load(), ignoreRule, newIgnorer() (+7 more)

### Community 10
Cohesion: 0.27
Nodes (16): builder.verilogCalls(), builder.verilogClass(), builder.verilogInclude(), builder.verilogItems(), builder.verilogMethod(), builder.verilogModule(), builder.verilogPackage(), builder.verilogSubroutine() (+8 more)

### Community 11
Cohesion: 0.29
Nodes (12): builder.luaAssign(), builder.luaCall(), builder.luaStatement(), builder.luaTopCalls(), childByKind(), extractLua(), firstNamed(), luaCalleeName() (+4 more)

### Community 12
Cohesion: 0.38
Nodes (10): builder.rubyCalls(), builder.rubyMethod(), builder.rubyRecordCall(), builder.rubyRequire(), builder.rubyStatement(), builder.rubyType(), extractRuby(), rubyConstName() (+2 more)

### Community 13
Cohesion: 0.20
Nodes (9): Circle, Shapes, Shapes.area(), Shapes.scale(), cube(), MathUtils, MathUtils.square(), LinearAlgebra (+1 more)

### Community 14
Cohesion: 0.38
Nodes (6): defs.svh, add(), alu, compute(), Counter, Counter.step()

### Community 15
Cohesion: 0.43
Nodes (5): aws_instance.web, aws_vpc.main, data.aws_ami.ubuntu, output.instance_id, var.region

### Community 16
Cohesion: 0.67
Nodes (3): aws_s3_bucket.default, module.this [null-label], cloudposse/label/null
