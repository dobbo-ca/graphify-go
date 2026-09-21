package extract

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

const ifaceSrc = "interface IReport {\n  string Build();\n}\n"

const reportSrc = "class Report {\n  public string Build() { return \"x\"; }\n}\n"

const auditSrc = "class Audit {\n  public string Build() { return \"y\"; }\n}\n"

// implementsEdge stands in for the inherits/implements pass: `implements` edges
// are not emitted yet, so the dispatch pass is fed the edge it will walk.
func implementsEdge(implFile, implType string) model.Edge {
	return model.Edge{
		Source: idutil.MakeID(fileStem(implFile), implType), Target: idutil.MakeID("ireport", "IReport"),
		Relation: "implements", Confidence: "EXTRACTED", SourceFile: implFile,
	}
}

func dispatchTargets(t *testing.T, results []Result, files []string) map[string]string {
	t.Helper()
	ext := Resolve(results, files)
	label := map[string]string{}
	for _, n := range ext.Nodes {
		label[n.ID] = n.Label
	}
	got := map[string]string{}
	for _, e := range ext.Edges {
		if e.Relation == "dispatches_to" {
			got[label[e.Source]] = label[e.Target]
		}
	}
	return got
}

// One implementer owning one same-named method: the interface method dispatches
// to the implementation.
func TestCSharpDispatchSingleImplementer(t *testing.T) {
	files := []string{"IReport.cs", "Report.cs"}
	results := []Result{
		extractCSharp("IReport.cs", []byte(ifaceSrc)),
		extractCSharp("Report.cs", []byte(reportSrc)),
	}
	results[1].Edges = append(results[1].Edges, implementsEdge("Report.cs", "Report"))

	got := dispatchTargets(t, results, files)
	if got["IReport.Build()"] != "Report.Build()" {
		t.Errorf("expected IReport.Build --dispatches_to--> Report.Build, got %v", got)
	}
}

// Two implementers: the runtime target is ambiguous, so nothing is emitted.
func TestCSharpDispatchTwoImplementers(t *testing.T) {
	files := []string{"IReport.cs", "Report.cs", "Audit.cs"}
	results := []Result{
		extractCSharp("IReport.cs", []byte(ifaceSrc)),
		extractCSharp("Report.cs", []byte(reportSrc)),
		extractCSharp("Audit.cs", []byte(auditSrc)),
	}
	results[1].Edges = append(results[1].Edges, implementsEdge("Report.cs", "Report"))
	results[2].Edges = append(results[2].Edges, implementsEdge("Audit.cs", "Audit"))

	if got := dispatchTargets(t, results, files); len(got) != 0 {
		t.Errorf("expected no dispatches_to with two implementers, got %v", got)
	}
}

// A non-C# implementer is a name collision, not an implementation.
func TestCSharpDispatchCrossLanguage(t *testing.T) {
	files := []string{"IReport.cs", "Report.java"}
	results := []Result{
		extractCSharp("IReport.cs", []byte(ifaceSrc)),
		extractJava("Report.java", []byte("class Report {\n  public String Build() { return \"x\"; }\n}\n")),
	}
	results[1].Edges = append(results[1].Edges, implementsEdge("Report.java", "Report"))

	if got := dispatchTargets(t, results, files); len(got) != 0 {
		t.Errorf("expected no dispatches_to across a language boundary, got %v", got)
	}
}
