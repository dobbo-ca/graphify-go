package extract

import (
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// resolveCSharpDispatch links a C# interface's method to the implementing
// method when the interface has exactly one implementer owning exactly one
// same-named method.
//
// A call through a constructor-injected dependency (`_report.Build()` where
// `_report` is an `IReport`) resolves to the interface's method node, because
// that is what the call site names. Nothing joins it to `Report.Build()`, so a
// directed walk stops at the interface — on a DI-heavy service, that cuts most
// chains. One `dispatches_to` edge per method reconnects every call that
// reaches it.
//
// Single-owner only: walking `implements` to the one type that can serve the
// call mirrors the runtime, guessing among several implementers would not.
// Purely additive — the call edge to the interface method stays, the call site
// really does name the interface. Confidence is INFERRED: the target is forced
// once there is a single implementer, but the source text never names it.
func resolveCSharpDispatch(out *model.Extraction) {
	node := map[string]*model.Node{}
	for i := range out.Nodes {
		node[out.Nodes[i].ID] = &out.Nodes[i]
	}

	implementers := map[string]map[string]bool{} // interface id -> implementer ids
	methods := map[string]map[string][]string{}  // owner id -> bare name -> method ids
	for _, e := range out.Edges {
		switch e.Relation {
		case "implements":
			if implementers[e.Target] == nil {
				implementers[e.Target] = map[string]bool{}
			}
			implementers[e.Target][e.Source] = true
		case "contains":
			m := node[e.Target]
			if m == nil || !isCSharpNode(m) {
				continue
			}
			name := bareMethodName(m.Label)
			if name == "" {
				continue
			}
			if methods[e.Source] == nil {
				methods[e.Source] = map[string][]string{}
			}
			if !contains(methods[e.Source][name], e.Target) {
				methods[e.Source][name] = append(methods[e.Source][name], e.Target)
			}
		}
	}

	for ifaceID, impls := range implementers {
		if len(impls) != 1 {
			continue
		}
		var implID string
		for id := range impls {
			implID = id
		}
		// Both ends must be C# declarations: `implements` is resolved by name,
		// so a cross-language pair is a name collision, not an implementation.
		if !isCSharpNode(node[ifaceID]) || !isCSharpNode(node[implID]) {
			continue
		}
		for name, declared := range methods[ifaceID] {
			candidates := methods[implID][name]
			if len(declared) != 1 || len(candidates) != 1 || declared[0] == candidates[0] {
				continue
			}
			impl := node[candidates[0]]
			out.Edges = append(out.Edges, model.Edge{
				Source: declared[0], Target: candidates[0], Relation: "dispatches_to",
				Confidence: "INFERRED", SourceFile: impl.SourceFile, SourceLocation: impl.SourceLocation,
			})
		}
	}
}

func isCSharpNode(n *model.Node) bool {
	return n != nil && strings.HasSuffix(n.SourceFile, ".cs")
}

// bareMethodName returns a method node's own name: `Report.Build()` -> `Build`.
// Case is kept — C# is case sensitive and an implementing member must spell the
// interface member exactly. The name is cut at the first parenthesis rather
// than by stripping a trailing `()`, so a label that ever carried a signature
// still matches.
func bareMethodName(label string) string {
	if i := strings.Index(label, "("); i >= 0 {
		label = label[:i]
	}
	if i := strings.LastIndex(label, "."); i >= 0 {
		label = label[i+1:]
	}
	return label
}
