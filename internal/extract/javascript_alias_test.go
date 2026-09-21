package extract

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
)

// `@/lib/x` is the project-root alias convention: it must resolve to the corpus
// file lib/x.ts (imports_from), not become an external dependency node.
func TestResolveAtSlashAliasImport(t *testing.T) {
	files := []string{"app/page.tsx", "lib/x.ts"}
	results := []Result{
		FileFromBytes("app/page.tsx", []byte("import { x } from '@/lib/x';\nimport React from 'react';\n")),
		FileFromBytes("lib/x.ts", []byte("export const x = 1;\n")),
	}
	ext := Resolve(results, files)

	fromID, toID := idutil.MakeID("app/page.tsx"), idutil.MakeID("lib/x.ts")
	found := false
	for _, e := range ext.Edges {
		if e.Relation == "imports_from" && e.Source == fromID && e.Target == toID {
			found = true
		}
		if e.Relation == "imports" && e.Source == fromID && e.Target == idutil.MakeID("@/lib/x") {
			t.Error("@/lib/x became an external import edge")
		}
	}
	if !found {
		t.Error("expected app/page.tsx --imports_from--> lib/x.ts")
	}

	// A genuinely bare specifier still becomes an external node.
	bare := false
	for _, n := range ext.Nodes {
		if n.Label == "react" && n.FileType == "concept" {
			bare = true
		}
	}
	if !bare {
		t.Error("expected external concept node for react")
	}
}
