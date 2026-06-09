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

> **Note (2026-06-09):** This spec was first drafted against a stale local `main`.
> `origin/main` is several commits ahead (v0.4.0). The sections below reflect the
> **actual** current code in the worktree, where significant substrate already
> exists. What changed vs. the first draft: module-`source` capture and
> module→module-dir linking are already implemented; `dirScope` already uses the
> full directory path (basename-collision risk gone); `tfAttrString` already
> exists.

`internal/extract/terraform.go` already:

- Turns HCL blocks (`resource`/`data`/`module`/`variable`/`output`/`provider`/
  `locals`) into nodes, IDs scoped by the **full directory path**
  (`dirScope`, `terraform.go:261`) so two dirs sharing a base name don't collide.
- Emits edges `contains` (file→block), `references` (block→interpolated address),
  `depends_on`.
- **Captures each module's `source`** via `tfAttrString` (`terraform.go:160`) and
  records a `ModRef{FromID, Source, File, Loc}` (`terraform.go:103`,
  `extract.go:41`).
- **Links module invocations to what they instantiate** in `Resolve`
  (`resolve.go:103-149`): a *local* source resolves to a `tfmodule` **directory
  node** (`idutil.MakeID("tfmodule", dir)`) that gains `contains` edges to that
  dir's files, plus a `references` edge `module.<name> → dir`. A *registry/git*
  source (e.g. `cloudposse/label/null`) becomes an external `concept` node
  (`idutil.MakeID("tfmodule", source)`, Label = the source string) with a
  `references` edge `module.<name> → concept`.
- **Already links null-label usage partially**: a resource doing
  `bucket = module.this.id` already gets a `references` edge → `module.this`
  (`tfRefAddress` maps `module.<name>.<attr>` → `module.<name>`). Even
  `context = module.this.context` produces a (generic `references`) edge.

What is still missing:

1. **No null-label *marker*.** The `cloudposse/label/null` concept node and the
   `references` edge exist, but the `module.this` node itself is not visibly
   tagged, and the source-classification (`isLocalSource`) does not special-case
   null-label. Goal: a `[null-label]` marker on the module node.
2. **Computed name not reconstructed** — the deployed id exists nowhere.
3. **Context inheritance is a generic `references` edge**, not distinguished as
   label propagation. Goal: an `inherits_context` relation.
4. **No name reconstruction across the chain.** The module→dir `references` edges
   from step (resolve) give the *structural* substrate to walk local wrapper
   chains, but nothing accumulates label inputs along them.

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
  change** — search rides on `norm_label`).
- New edge relation: `inherits_context` — carried by the existing `relation`
  plumbing (no struct change). Module→module-dir linking already uses the existing
  `references` relation + `ModRef` channel; no new struct or relation needed there.

## Implementation stages

Three independently-shippable, verifiable stages. A+B are useful even if C slips.

### Stage A — Foundation: marker + context edge

Module-`source` capture and module→dir/concept linking **already exist** — Stage A
only adds the two missing pieces.

`internal/extract/terraform.go`:
- Add `isNullLabel(s string) bool` =
  `strings.Contains(s,"cloudposse/label/null") || strings.Contains(s,"terraform-null-label")`.
- Module arm (`:99-106`): `source` is already read for the `ModRef`. Lift that
  read so it also drives the Label: when `isNullLabel(source)`, pass
  `Label = "module.<name> [null-label]"` into `def` (the ID stays
  `module.<name>`, so every existing `references`/`depends_on` edge still
  resolves). `def` already takes Label separately from the address — no signature
  change for Stage A.
- `refsFrom` (`:55-58`): make the relation selection 3-way — replace the
  `depends_on` `if` with a `switch` on the attribute identifier, adding
  `case "context": rel2 = "inherits_context"`. This turns `context =
  module.this.context` into `module.<name> --inherits_context--> module.this`
  (and `context = var.context` into an `inherits_context` edge to `var.context`).

No `resolve.go` change in Stage A.

Verify: fixtures with `source="cloudposse/label/null"` + `context =
module.this.context` → assert `[null-label]` in the module node Label and an
`inherits_context` edge; existing `module.<name> --references--> <dir/concept>`
linking still holds.

### Stage B — Single-block literal name reconstruction

Scope is the null-label block's **own** literal inputs only — no cross-block/dir
resolution (that is Stage C). This is the bulk of the feature.

- Data-model change lands here (first use): add `ComputedName` to `model.Node`;
  `export.jsonNode` emits `computed_name` (`omitempty`) and folds it into
  `norm_label`; `query.Node` gains the field for display. To set `ComputedName`
  on the module node, generalize the `def` closure (`terraform.go:35`) into a
  `defNode(addr,label,loc,computed string)` and keep `def` as a thin wrapper
  passing `""` — so the other six block types are untouched.
- New file `internal/extract/nulllabel.go`:
  - `nullLabelInputs(body, src) labelInputs` — capture from the block body,
    **literals only**: scalars `namespace/tenant/environment/stage/name`
    (string_lit → KNOWN; `""`/null → EMPTY; absent → EMPTY *unless* a `context`
    attr is present, then UNKNOWN; non-literal expr → UNKNOWN), list `attributes`
    (tuple of string_lit → KNOWN list; non-literal → UNKNOWN), and knobs
    `delimiter`/`label_order`/`label_value_case` (literal → use; absent → assume
    documented default; present-but-non-literal → mark shape UNRESOLVED).
  - `composeID(in labelInputs) string` — the cloudposse subset (**no
    truncation**): per-segment normalize (regex strip `[^-a-zA-Z0-9]` → case);
    `attributes` → `join(delimiter)` into one slot; assemble per resolved
    `label_order`, dropping EMPTY; `{seg}` sentinel for UNKNOWN; return `""` when
    shape UNRESOLVED or nothing KNOWN. Confidence rides as a suffix
    (`" (partial)"` when any UNKNOWN segment fed the id).
- `label_order` + `tenant`: `tenant` is a captured scalar like the rest;
  `label_order` (literal) controls inclusion set + sequence, so a listed `tenant`
  emits — verbatim per the user's examples.
- Module arm: when `isNullLabel`, call `composeID(nullLabelInputs(bbody,src))` and
  pass it as the `computed` arg to `defNode`.

Verify: `name="asdf" attributes=["1","2"] delimiter="!"` → `asdf!1!2`;
`label_order=["attributes","name"]` (same inputs) → `1!2!asdf`;
`label_order=["tenant","namespace","name"] tenant="acme"` → reordered incl tenant;
a `var`-sourced segment with a `context` attr present → sentineled PARTIAL id (no
false full name); non-literal `label_order` → no id (UNRESOLVED). Searchable via
`norm_label` (assert through `export.normLabel`).

### Stage C — Context-chain reconstruction (whole-corpus)

Stage C fills the UNKNOWN segments Stage B left, by following context inheritance
across blocks/dirs. It runs as a whole-corpus post-pass (in/after `Resolve`,
where the `inherits_context` edges from Stage A and the existing module→dir/concept
`references` edges all exist).

- Substrate (already built): `inherits_context` edges (`module.X → module.this` /
  `var.context`) and module→`tfmodule`-dir `references` edges (caller invocation →
  wrapper dir → its files). Same-dir `module.this` is just the 1-hop case of the
  same walk — no separate code path.
- For each null-label module node with a non-EXACT `ComputedName`, walk its
  `inherits_context` target(s): a same-dir/same-corpus `module.this` whose own
  `labelInputs` are KNOWN contributes its scalars (child literal overrides;
  `attributes` merge parent-first); a `var.context` whose value originates from a
  caller's module invocation is followed via the module→dir `references` edge,
  mapping the caller's invocation attrs onto the wrapper's inputs.
- Accumulate **raw** inputs root→leaf per cloudposse merge rules
  (override-else-inherit for scalars + knobs; parent-first concat+dedup for
  `attributes`); **normalize once at the leaf** by re-running `composeID`.
- **Stop at the first opaque hop** (remote/registry/git source, `var.context` with
  no resolvable caller, dynamic expr) → leave remaining inherited segments
  UNKNOWN (sentinel), never fabricate.
- Confidence: `EXACT` only if the whole chain is local + all literal + knobs known;
  else `PARTIAL`.

Verify: 2–3-level local chain with literals + attribute accumulation +
`label_order` incl `tenant` → correct composed name; chain hitting `var.*` with no
resolvable caller → PARTIAL with sentinel; chain hitting a remote wrapper →
stops, PARTIAL.

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
  `inherits_context` edges are a relabel of edges that were already emitted as
  `references` (no topology change). Non-null-label repos are unaffected. The
  repo's own `graphify-out/graph.json` is git-tracked **but CI-regenerated**
  (`chore: regenerate knowledge graph [skip ci]`) — do **not** hand-edit it; let
  CI rebuild post-merge.
- **`inherits_context` relabel:** `context = module.this.context` previously
  emitted a `references` edge; it now emits `inherits_context` instead. Any
  consumer counting `references` edges to a label module sees one fewer. Acceptable
  — it is strictly more precise; no existing test asserts that specific edge.
- **Correctness of reconstruction:** mitigated by 3-state confidence, sentinels,
  never-fabricate, no-truncation-on-partial.
- **Perf:** Stage B adds a bounded per-null-label-block body walk; Stage C adds a
  whole-corpus pass over `inherits_context`/module-source edges (bounded by edge
  count). Negligible vs tree-sitter parse.

## Resolved since first draft (no longer concerns)

- Module-`source` capture (`ModRef`) and module→dir/concept linking already exist
  (`resolve.go:103-149`) — Stage A/C build on them, not rebuild.
- `dirScope` uses the full directory path (`terraform.go:261`) — the
  basename-collision risk is already fixed and covered by
  `TestExtractTerraformScopeByFullPath`.
- `tfAttrString` already exists (`terraform.go:160`).

## Out of scope

- Truncated/hashed id reproduction on partial data.
- Resolving values from tfvars / env / workspace / remote module bodies.
- A general HCL expression evaluator.
- Cross-repo (global multi-repo) module-source resolution.
