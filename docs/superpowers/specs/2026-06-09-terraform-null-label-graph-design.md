# Terraform cloudposse null-label awareness in the graph

**Date:** 2026-06-09
**Status:** Approved design, pre-implementation
**Branch:** `worktree-tf-null-label-graph-4e7a`

## Objective

Make graphify's Terraform knowledge graph understand the
[cloudposse/terraform-null-label](https://github.com/cloudposse/terraform-null-label)
naming convention, which dobbo-ca terraform repos use heavily. Two outcomes the
user asked for ("Both"):

1. **Semantic tagging** — detect null-label modules, mark them distinctly, and
   make the label/context relationships legible in the graph.
2. **Searchable computed names** — reconstruct the deployed resource name
   (`namespace-environment-stage-name-attributes`) so `graphify query eg-prod-app`
   finds the label module and the resources named from it. Today these names are
   computed at apply time and appear nowhere in source, so they are unsearchable.

The user's repos are a **mix**: some label inputs are string literals in the
module call; many are inherited via `context = module.this.context` (or
`var.context`) **chained several wrapper-module levels deep**; `label_order` is
used regularly (and is the only way `tenant` enters an id).

## Background — current Terraform extraction

`internal/extract/terraform.go` already:

- Turns HCL blocks (`resource`/`data`/`module`/`variable`/`output`/`provider`/
  `locals`) into nodes, IDs scoped by **directory** (so cross-`.tf` refs resolve).
- Emits edges `contains` (file→block), `references` (block→interpolated address),
  `depends_on`.
- **Already links null-label usage partially**: a `module "this"` block becomes a
  node, and a resource doing `bucket = module.this.id` already gets a `references`
  edge → `module.this` (`tfRefAddress` maps `module.<name>.<attr>` →
  `module.<name>`). Even `context = module.this.context` produces a reference edge.

What is missing:

1. **No detection** that a module *is* null-label — `tfBlock` (`terraform.go:136`)
   parses only identifier/string_lit/body and **drops the `source` attribute**.
2. **Computed name not reconstructed** — exists nowhere in the graph.
3. **Context inheritance is a generic `references` edge**, not marked as label
   propagation.
4. **Module invocations are never linked to the module dir they source.**
   `Resolve` (`internal/extract/resolve.go`) wires calls/imports for code
   languages; Terraform contributes no `Defs`/`Calls`/`Imps`, and `source` is
   dropped, so there is no edge from `module "x" { source = "../foo" }` to the
   files in `../foo`. Tracing context across module boundaries needs this.

### Search mechanics (key enabler)

`norm_label` is the search key. It is derived **only** from `Label` at export
(`internal/export/export.go:69`, `normLabel(n.Label)`), and `query.Query`
(`internal/query/query.go:93`) matches a regex against `Label || ID || NormLabel`.
The HTML viewer search box also matches `label`/`norm_label`. Therefore anything
folded into `norm_label` becomes searchable in **both** the CLI and the viewer
with no `query.go` or viewer change.

## Authoritative cloudposse `id` algorithm (to reimplement)

graphify cannot run Terraform and the null-label module body is remote, so we
**reimplement** the id composition. Verified against the module source (main.tf /
context.tf, identical duplicate locals blocks), stable for v0.25.0+:

```
id = join(delimiter, [ formatted(label) for label in label_order if non-empty ])
```

- **Valid label keys:** `namespace, tenant, environment, stage, name, attributes`.
- **Default `label_order`:** `["namespace","environment","stage","name","attributes"]`
  — `tenant` is **excluded by default**; it only appears when `label_order` lists it.
- **Default `delimiter`:** `-` (`""` = no delimiter).
- **Scalar resolution** (`namespace/tenant/environment/stage/name`): child value
  overrides inherited context if non-null, else inherit.
- **`attributes` MERGE (not override):**
  `compact(distinct(concat(context.attributes, var.attributes)))` — parent-first,
  child appended, dedup preserving first occurrence. In the id the whole list is
  collapsed into **one** slot via `join(delimiter, attributes)`.
- **Normalization per segment:** `replace(value, regex_replace_chars, "")` then
  case. Default `regex_replace_chars = /[^-a-zA-Z0-9]/` (strip everything not
  hyphen / ASCII letter / digit). Default `label_value_case = "lower"`
  (`"none"` skips case but still regex-strips; `"title"` = Terraform `title()`
  semantics — split on whitespace only; `"upper"`).
- **Empty segments dropped** from the id.
- **Truncation:** only if `id_length_limit != 0` (default 0 = unlimited); uses an
  md5 hash. **We do not replicate truncation** — see below.
- **Context output is RAW:** `module.X.context` returns the merged-but-unnormalized
  inputs with nulls preserved; defaults/normalization apply only at the leaf at
  id-computation time. So a chain = accumulate raw inputs root→leaf, **normalize
  once at the leaf**.
- **Source strings identifying a null-label invocation:** registry shorthand
  `cloudposse/label/null`, fully-qualified `registry.terraform.io/cloudposse/label/null`,
  and git/github forms containing `terraform-null-label` (with optional
  `?ref=...`). Detection: `strings.Contains(src, "cloudposse/label/null") ||
  strings.Contains(src, "terraform-null-label")`.

## Reconstruction tractability & the never-fabricate rule

**Tractable** from parsed local HCL: literal values in-block; local `./`/`../`
module chains where every hop is literal/known; literal knobs
(`delimiter`/`label_order`/`label_value_case`).

**Intractable:** `var.*` without a literal default; tfvars / `*.auto.tfvars` /
`-var` / `TF_VAR_*` / `terraform.workspace`; remote module bodies (the null-label
module itself, registry/git wrappers); dynamic exprs (`"${var.name}-x"`,
`format()`, `coalesce()`, conditionals, for-exprs); `attributes` built
dynamically; per-instance `count`/`for_each` names.

**Rules (correctness stance: prefer false-UNKNOWN over false-KNOWN — a wrong
deployed name resolving to the wrong cloud resource is worse than a gap):**

- Resolve each segment to one of three states: **KNOWN** (literal/composed),
  **EMPTY** (literal `""`/null → dropped, a real null-label outcome), **UNKNOWN**
  (var-without-default, runtime, expr, opaque/remote hop).
- Build the display id by joining segments in `label_order` with the resolved
  delimiter. UNKNOWN segments → a syntactically distinct sentinel `{segname}`
  (e.g. `{namespace}-prod-app`) that can never collide with a real deployed name.
  Drop EMPTY segments.
- **Never apply truncation/hash on partial input.** Only compute the real
  truncated id when all segments **and** all shape knobs are KNOWN; otherwise
  leave untruncated and flag.
- **Knobs from a non-literal source** (`label_order`/`delimiter`/`case` from
  `var`/expr/remote) change the output *shape* → mark the whole name **UNRESOLVED**
  (do not fall back to default — we know it is overridden but cannot read it).
- Default knobs to documented upstream defaults **only** when the code does not
  override them, and record "assumed".
- Follow context inheritance **only** across local relative-path hops, until the
  first UNKNOWN/remote/opaque hop, then stop and mark remaining inherited segments
  UNKNOWN.
- **Confidence** rides with the name: `EXACT` (all segments + knobs KNOWN, no
  opaque hop) → real searchable id; `PARTIAL` (≥1 UNKNOWN, order known) →
  templated id usable for substring/prefix search; `UNRESOLVED` → store captured
  literals, emit no id.

## Design decisions

### Storage: split by purpose

| Thing | Where | Why |
|---|---|---|
| `[null-label]` marker | `Label` suffix | Must be visible in every surface (viewer renders `Label`; `explain`/`query` print it). A new field would not show without viewer changes. Tiny, deterministic; ID stays `module.<name>` so all existing edges still resolve. |
| Computed name (e.g. `eg-prod-app` / `{namespace}-prod-app`) | new `ComputedName` field on `model.Node` | Templated/partial strings would be ugly in `Label` and conflate node identity with derived name. It is metadata. |

**Search unification:** in `export`, set
`norm_label = normLabel(Label) + " " + normLabel(ComputedName)`. CLI `query` and
viewer search both hit it with zero `query.go`/viewer change. `ComputedName` is
also emitted as its own `computed_name` JSON key (`omitempty`) for clean
`explain` display.

### Confidence storage

`PARTIAL`/`EXACT`/`UNRESOLVED` is carried as a short suffix on `ComputedName`
(e.g. `eg-prod-app` for EXACT, `{namespace}-prod-app (partial)` for PARTIAL). No
separate field — keeps the data-model change to exactly one node field.
*(Implementation may revisit to a small enum field if display warrants; default is
the suffix.)*

### Data model — full change surface

- `model.Node`: add `ComputedName string \`json:"computed_name,omitempty"\``.
- `export.jsonNode`: add `computed_name` (omitempty); fold `ComputedName` into
  `norm_label`.
- `query.Node`: add `ComputedName` field (display only; **no `Query` logic
  change**).
- New edge relations: `inherits_context`, `sources` — additive, carried by the
  existing `relation` plumbing (no struct changes).
- Reuse the existing `Imp` channel to carry module `source` to `Resolve` (no new
  struct).

## Implementation stages

Three independently-shippable, verifiable stages. A+B are useful even if C slips.

### Stage A — Foundation: detect + tag + edges (no reconstruction)

`internal/extract/terraform.go`:
- Module arm (`:99`): read `source` via a small `tfAttrString(bbody,"source")`
  helper (modeled on the locals attr loop at `:115`). `isNullLabel(src)` =
  `strings.Contains` of `cloudposse/label/null` or `terraform-null-label`. If
  true → `Label = "module.<name> [null-label]"` (ID unchanged).
- For a **relative** source (`./`,`../`): append
  `Imp{FileID: <module node id>, File: rel, Spec: src, Loc: loc}` to carry it to
  `Resolve`.
- `refsFrom` (`:56`): make the relation selection 3-way — add
  `case "context": rel2 = "inherits_context"` alongside the existing `depends_on`
  branch.

`internal/extract/resolve.go` (Imps loop `:62`):
- New branch for `.tf` relative Imps: resolve `source` to a directory
  (`path.Clean(path.Join(path.Dir(im.File), spec))`), find a corpus file in that
  dir, emit a `sources` edge (module node → that file node, `Confidence:
  "INFERRED"`), then `continue`.
- **Guard:** bare/registry sources are non-relative → no dir → no edge, and must
  **not** fall through to the bare-import branch (which would mint a spurious
  external `concept` node).

Verify: fixtures with `source="cloudposse/label/null"` + `context =
module.this.context` + a relative-source module across two dirs → assert
`[null-label]` in a node Label, an `inherits_context` edge, and a `sources` edge.

### Stage B — Literal name reconstruction (in-block + same-dir `module.this`)

- Land the data-model change (`ComputedName` + export fold + `query.Node`
  display).
- Capture from the block body, **literals only**: scalars
  `namespace/tenant/environment/stage/name`, list `attributes`, knobs
  `delimiter/label_order/label_value_case`.
- Reconstruct via the cloudposse subset above (**no truncation**): per-segment
  KNOWN/EMPTY/UNKNOWN; normalize (regex strip → case); `attributes` →
  `join(delimiter)` into one slot; assemble per resolved `label_order` (literal,
  or default-assumed when absent, or UNRESOLVED when present-but-non-literal);
  drop EMPTY; `{seg}` sentinel for UNKNOWN.
- `label_order` + `tenant`: `tenant` is a captured scalar like the rest;
  `label_order` (when literal) controls inclusion set + sequence, so a listed
  `tenant` emits.
- Same-dir `module.this` overlay: if `context = module.this.context` and
  `module.this` is in the same dir with literal values, overlay (child literal
  overrides; `attributes` merge parent-first).
- Confidence suffix on `ComputedName` per the rules.

Verify: literal module → exact searchable name; `label_order =
["tenant","namespace","name"]` + `tenant="acme"` → reordered name including
tenant; attributes-merge; a `var`-sourced segment → sentineled PARTIAL id (no
false full name); non-literal `label_order` → UNRESOLVED.

### Stage C — Local context-chain reconstruction (multi-level)

- Reconstruction moves to a whole-corpus pass (in/after `Resolve`, where
  `sources` edges and child module variables exist).
- Walk the local chain root→leaf via `sources` edges, accumulating **raw** context
  per cloudposse merge rules: scalars + `label_order` override-else-inherit;
  `attributes` parent-first concat + dedup; **normalize once at the leaf**.
- Map each invocation's attrs (`namespace=…`, `name=…`, `context=…`) onto the
  child null-label inputs.
- **Stop at the first opaque hop** (remote/registry/git source, `var.context` from
  outside the tree, dynamic expr) → mark remaining inherited segments UNKNOWN,
  never fabricate.
- Confidence: `EXACT` only if the whole chain is local + all literal + knobs known;
  else `PARTIAL`; else `UNRESOLVED`.

Verify: 3-level local chain with literals + attribute accumulation + `label_order`
incl `tenant` → correct composed name; chain hitting `var.*` → PARTIAL with
sentinel; chain hitting a remote wrapper → stops, PARTIAL.

## Testing strategy

- Table-driven unit tests for the id composer against **hand-computed cloudposse
  goldens** (Terraform cannot run in CI): cover delimiter override, `label_order`
  reorder incl `tenant`, attributes merge+dedup, casing (`lower`/`title`/`none`),
  empty-segment drop, sentinel for UNKNOWN, UNRESOLVED for non-literal knobs.
- Fixture repos under `internal/extract/testdata/tf/` mirroring real patterns:
  literal, single-context same-dir, multi-level local chain, `label_order`+
  `tenant`, partial/`var`, remote-stop.
- Extend `terraform_test.go` for Stage A assertions; new test files for the
  composer (Stage B) and the chain walk (Stage C).
- `go test ./internal/...` green at each stage; existing `TestExtractTerraform`
  stays green (its fixtures have no module blocks).

## Risks

- **graph.json bytes:** folding the marker into `Label` changes `label`/
  `norm_label` for affected module nodes only (IDs unchanged → topology
  identical). New `computed_name` key is additive (`omitempty`). New
  `inherits_context`/`sources` edges are additive. Non-null-label repos are
  unaffected. Update any committed graph / golden snapshots.
- **Basename collision:** `dirScope`/module-dir resolution keys by
  `filepath.Base(dir)`; two sibling module dirs with the same basename could
  mislink. Acceptable for a single-repo first cut; flag, revisit if a global
  multi-repo graph lands (see MEMORY: terraform-graph-adoption).
- **`Imp` reuse:** Terraform contributes its first `Imp`, and `Imp.FileID` will
  hold a **module** node id, not a file id. `Resolve` must treat it generically as
  an edge Source; verify no other `Resolve` logic assumes `Imp.FileID` is a file.
- **Correctness of reconstruction:** mitigated by 3-state confidence, sentinels,
  never-fabricate, no-truncation-on-partial.
- **Perf:** extra per-module `bbody` walk + O(files) dir scan per Terraform `Imp`
  in `Resolve`; bounded by block/file counts, negligible vs tree-sitter parse.

## Out of scope

- Truncated/hashed id reproduction on partial data.
- Resolving values from tfvars / env / workspace / remote module bodies.
- A general HCL expression evaluator.
- Cross-repo (global multi-repo) module-source resolution.
