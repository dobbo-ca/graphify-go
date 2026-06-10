package extract

import (
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// maxChainDepth bounds how many local wrapper hops a var-ref or context is
// followed, guarding against cyclic or pathological module graphs.
const maxChainDepth = 8

// resolveNullLabels completes partial cloudposse null-label ids by following
// LOCAL wrapper-module chains. The single-block reconstructor (Stage B) already
// fills a null-label module's id from its own literals; when a field comes from
// `var.X` or a `context =`, the id is partial. This pass fills those gaps by
// looking at the single local caller that invoked the wrapper directory.
//
// It only acts on the tractable subset: a wrapper dir with exactly one caller,
// direct `var.X -> arg` mappings, and same-corpus `module.Y.context` inheritance.
// For multi-caller dirs, remote/registry hops, or anything unresolvable it
// leaves the Stage B (partial) id untouched — never fabricating an exact id.
func resolveNullLabels(results []Result, out *model.Extraction) {
	// Index invocations by their module node id.
	invByNode := map[string]ModInvoke{}
	for _, r := range results {
		for _, m := range r.ModInvokes {
			invByNode[m.NodeID] = m
		}
	}

	// nodeLabel maps a node id to its label (one pass), used both to spot
	// tfmodule dir nodes and to recover a null-label's module address.
	nodeLabel := map[string]string{}
	for _, n := range out.Nodes {
		nodeLabel[n.ID] = n.Label
	}

	// callersByDir maps a wrapper directory (the dir a local `source` resolved
	// to) to the invocations that called it. Built from the module-source pass's
	// `references` edges into tfmodule dir nodes (whose Label is the dir path).
	dirLabel := map[string]string{} // dir-node id -> dir path label
	for id, label := range nodeLabel {
		if idutil.MakeID("tfmodule", label) == id {
			dirLabel[id] = label
		}
	}
	callersByDir := map[string][]ModInvoke{}
	for _, e := range out.Edges {
		if e.Relation != "references" {
			continue
		}
		dir, ok := dirLabel[e.Target]
		if !ok {
			continue
		}
		if m, ok := invByNode[e.Source]; ok {
			callersByDir[dir] = append(callersByDir[dir], m)
		}
	}

	// Index null-label refs (by node) so a `module.Y.context` reference can
	// inherit from another null-label module in the same corpus.
	nlByNode := map[string]NullLabelRef{}
	for _, r := range results {
		for _, ref := range r.NullLabels {
			nlByNode[ref.NodeID] = ref
		}
	}
	// nlByAddr lets a `module.<y>` context ref find the sibling null-label in the
	// same directory: key is "<dir>\x00module.<y>".
	nlByAddr := map[string]NullLabelRef{}
	for _, ref := range nlByNode {
		addr := strings.TrimSuffix(nodeLabel[ref.NodeID], " [null-label]")
		nlByAddr[ref.Scope+"\x00"+addr] = ref
	}

	for _, r := range results {
		for _, ref := range r.NullLabels {
			before := composeID(ref.Inputs)
			if before != "" && !strings.Contains(before, "{") && !strings.HasSuffix(before, "(partial)") {
				continue // already EXACT
			}
			resolved := resolveOneNullLabel(ref, callersByDir, nlByAddr, 0)
			after := composeID(resolved)
			if after == "" || after == before {
				continue
			}
			beforeExact := before != "" && !strings.Contains(before, "{") && !strings.HasSuffix(before, "(partial)")
			afterExact := !strings.Contains(after, "{") && !strings.HasSuffix(after, "(partial)")
			// Replace only when the new id is a strict improvement: it became
			// fully exact, or it carries more concrete segments than before.
			if !(afterExact && !beforeExact) && exactCount(after) <= exactCount(before) {
				continue
			}
			setComputed(out, ref.NodeID, after)
		}
	}
}

// resolveOneNullLabel returns a copy of ref.Inputs with var-ref and context
// fields filled in where possible. When the wrapper dir has exactly one local
// caller, `var.X` fields are filled from that caller's args; a `context =`
// module ref inherits from its sibling. When the dir has zero or multiple
// callers, var-ref fields are left unknown (so the id stays partial) rather than
// guessed — the top-level improvement gate then leaves Stage B's id untouched.
func resolveOneNullLabel(ref NullLabelRef, callersByDir map[string][]ModInvoke, nlByAddr map[string]NullLabelRef, depth int) labelInputs {
	callers := callersByDir[ref.Scope]
	var M *ModInvoke
	if len(callers) == 1 {
		M = &callers[0]
	}

	res := cloneInputs(ref.Inputs)

	// Fill scalar fields.
	for _, k := range labelScalars {
		if res.scalars[k].state == segKnown {
			continue
		}
		if vname, ok := ref.Inputs.varRefs[k]; ok && M != nil {
			if v, st := resolveArg(*M, vname, callersByDir, depth); st == segKnown && v != "" {
				res.scalars[k] = segVal{val: v, state: segKnown}
			}
		}
	}

	// Fill attributes from a var-ref to a list argument.
	if res.attrState != segKnown {
		if vname, ok := ref.Inputs.varRefs["attributes"]; ok && M != nil {
			if v, st := resolveArg(*M, vname, callersByDir, depth); st == segKnown {
				if v == "" {
					res.attrs, res.attrState = nil, segKnown
				} else {
					res.attrs, res.attrState = strings.Split(v, listSep), segKnown
				}
			}
		}
	}

	// Context inheritance: a `context = module.<y>` (sibling null-label in the
	// same dir) contributes fields the child did not set (override-else-inherit).
	if ref.Inputs.contextRef != "" && depth < maxChainDepth {
		if parent, ok := nlByAddr[ref.Scope+"\x00"+ref.Inputs.contextRef]; ok {
			pin := resolveOneNullLabel(parent, callersByDir, nlByAddr, depth+1)
			inheritContext(&res, pin)
		}
	}

	return res
}

// resolveArg resolves a wrapper argument to a literal: a literal Args value, or
// a transitive pass-through via the wrapper's own single caller. Returns
// (value, segKnown) on success, else ("", segUnknown). Bounded by maxChainDepth.
func resolveArg(M ModInvoke, vname string, callersByDir map[string][]ModInvoke, depth int) (string, segState) {
	if sv, ok := M.Args[vname]; ok {
		return sv.val, sv.state
	}
	if outerVar, ok := M.ArgVarRefs[vname]; ok && depth < maxChainDepth {
		callers := callersByDir[M.Dir]
		if len(callers) == 1 {
			return resolveArg(callers[0], outerVar, callersByDir, depth+1)
		}
	}
	return "", segUnknown
}

// inheritContext copies parent fields into child where the child left them
// unset (segUnknown/segEmpty), mirroring cloudposse override-else-inherit.
// Attributes merge parent-first, then dedup in composeID.
func inheritContext(child *labelInputs, parent labelInputs) {
	for _, k := range labelScalars {
		if child.scalars[k].state == segKnown {
			continue // child overrides the inherited value
		}
		switch parent.scalars[k].state {
		case segKnown:
			child.scalars[k] = parent.scalars[k]
		case segUnknown:
			// The parent contributes this field but its value is unknown; the
			// child's effective value is therefore unknown too, so it renders as
			// a {seg} placeholder (partial) rather than being silently dropped.
			child.scalars[k] = segVal{state: segUnknown}
		}
	}
	if parent.attrState == segKnown {
		merged := append([]string{}, parent.attrs...)
		if child.attrState == segKnown {
			merged = append(merged, child.attrs...)
		}
		child.attrs, child.attrState = merged, segKnown
	}
	// Clear the child's "pending inheritance" markers ONLY when the resolved
	// parent is itself fully exact — a non-empty id with no {seg} placeholder,
	// no " (partial)" suffix, and no residual contextRef/varRefs of its own. If
	// the parent is still partial, the child must stay partial too: dropping the
	// markers here would emit a wrong EXACT id (the parent's unknown segment is
	// silently lost). Never fabricate; a gap is safer than a wrong name.
	pid := composeID(parent)
	parentExact := pid != "" && !strings.Contains(pid, "{") &&
		!strings.HasSuffix(pid, " (partial)") &&
		parent.contextRef == "" && len(parent.varRefs) == 0
	if parentExact {
		child.hasContext = false
		child.contextRef = ""
	}
}

// cloneInputs deep-copies the mutable maps/slices of a labelInputs so a resolved
// copy never mutates the captured original.
func cloneInputs(in labelInputs) labelInputs {
	out := in
	out.scalars = make(map[string]segVal, len(in.scalars))
	for k, v := range in.scalars {
		out.scalars[k] = v
	}
	out.attrs = append([]string(nil), in.attrs...)
	out.labelOrder = append([]string(nil), in.labelOrder...)
	out.varRefs = make(map[string]string, len(in.varRefs))
	for k, v := range in.varRefs {
		out.varRefs[k] = v
	}
	return out
}

// exactCount counts the concrete (non-"{seg}") segments in a composed id, used
// to decide whether a re-resolved id is strictly more complete than Stage B's.
func exactCount(id string) int {
	id = strings.TrimSuffix(id, " (partial)")
	if id == "" {
		return 0
	}
	n := 0
	for _, seg := range strings.FieldsFunc(id, func(r rune) bool { return r == '-' }) {
		if !strings.HasPrefix(seg, "{") {
			n++
		}
	}
	return n
}

// setComputed updates a node's ComputedName in out.Nodes by id.
func setComputed(out *model.Extraction, id, name string) {
	for i := range out.Nodes {
		if out.Nodes[i].ID == id {
			out.Nodes[i].ComputedName = name
			return
		}
	}
}
