package extract

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// resolvePy extracts each in-memory Python file and stitches them with Resolve.
func resolvePy(srcs map[string]string) model.Extraction {
	var results []Result
	var files []string
	for f, s := range srcs {
		results = append(results, FileFromBytes(f, []byte(s)))
		files = append(files, f)
	}
	return Resolve(results, files)
}

// callConfidence returns the confidence of a src->tgt calls edge, by node label.
func callConfidence(ext model.Extraction, srcLabel, tgtLabel string) (string, bool) {
	id2label := map[string]string{}
	for _, n := range ext.Nodes {
		id2label[n.ID] = n.Label
	}
	for _, e := range ext.Edges {
		if e.Relation == "calls" && id2label[e.Source] == srcLabel && id2label[e.Target] == tgtLabel {
			return e.Confidence, true
		}
	}
	return "", false
}

// countCalls returns the number of calls edges in an extraction.
func countCalls(ext model.Extraction) int {
	n := 0
	for _, e := range ext.Edges {
		if e.Relation == "calls" {
			n++
		}
	}
	return n
}

// TestImportGuidedResolvesAmbiguousAlias checks that `from M import N as L`
// evidence resolves an otherwise-ambiguous bare call L() to the unique
// (module_stem, N) definition with EXTRACTED confidence — a case the generic
// name pass cannot resolve (the name N is defined in two files).
func TestImportGuidedResolvesAmbiguousAlias(t *testing.T) {
	ext := resolvePy(map[string]string{
		"helper.py": "def transform(x):\n    return x\n",
		"a/dup.py":  "def transform(y):\n    return y\n", // decoy: same name, different module
		"main.py":   "from helper import transform as tx\n\n\ndef run():\n    return tx(1)\n",
	})

	conf, ok := callConfidence(ext, "run()", "transform()")
	if !ok {
		t.Fatal("expected run --calls--> transform via import evidence")
	}
	if conf != "EXTRACTED" {
		t.Errorf("import-guided edge confidence = %q, want EXTRACTED", conf)
	}
	// The decoy a/dup.py:transform shares the bare name, so the generic name
	// pass could not have resolved this; exactly one calls edge must exist.
	if got := countCalls(ext); got != 1 {
		t.Errorf("calls edge count = %d, want 1 (ambiguous name resolved only by import)", got)
	}
}

// TestImportGuidedSkipsMemberCall checks that a member call obj.tx() never
// resolves via import evidence even though tx is an imported alias.
func TestImportGuidedSkipsMemberCall(t *testing.T) {
	ext := resolvePy(map[string]string{
		"helper.py": "def transform(x):\n    return x\n",
		"a/dup.py":  "def transform(y):\n    return y\n",
		"main.py":   "from helper import transform as tx\n\n\ndef run(obj):\n    return obj.tx(1)\n",
	})
	if _, ok := callConfidence(ext, "run()", "transform()"); ok {
		t.Error("member call obj.tx() must not resolve via import evidence")
	}
}

// TestImportGuidedSkipsSelfEdge checks that import evidence never emits a
// self-edge when the imported symbol resolves back to the caller.
func TestImportGuidedSkipsSelfEdge(t *testing.T) {
	// run imports itself by name from its own module; the unique (main, run)
	// def is run itself, so no edge may be emitted.
	ext := resolvePy(map[string]string{
		"main.py": "from main import run\n\n\ndef run():\n    return run()\n",
	})
	for _, e := range ext.Edges {
		if e.Relation == "calls" && e.Source == e.Target {
			t.Errorf("unexpected self calls edge %s->%s", e.Source, e.Target)
		}
	}
}

// TestImportGuidedNonAliased checks the plain `from M import N` form (local name
// equals imported name) resolves with EXTRACTED confidence.
func TestImportGuidedNonAliased(t *testing.T) {
	ext := resolvePy(map[string]string{
		"helper.py": "def transform(x):\n    return x\n",
		"a/dup.py":  "def transform(y):\n    return y\n",
		"main.py":   "from helper import transform\n\n\ndef run():\n    return transform(1)\n",
	})
	conf, ok := callConfidence(ext, "run()", "transform()")
	if !ok || conf != "EXTRACTED" {
		t.Errorf("plain `from helper import transform` should resolve EXTRACTED, got %q ok=%v", conf, ok)
	}
}
