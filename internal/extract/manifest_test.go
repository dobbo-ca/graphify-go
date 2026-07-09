package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

func writeManifestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertPackageGraph runs IntrospectManifests over a root holding a single
// manifest and asserts it produced one canonical package node for module, one
// package node per dependency in deps, and a depends_on edge module->dep for each.
func assertPackageGraph(t *testing.T, root, module string, deps []string) {
	t.Helper()
	res, err := IntrospectManifests(root)
	if err != nil {
		t.Fatalf("IntrospectManifests: %v", err)
	}

	nodesByID := map[string]model.Node{}
	for _, n := range res.Nodes {
		nodesByID[n.ID] = n
	}
	if want := 1 + len(deps); len(res.Nodes) != want {
		t.Fatalf("nodes = %d, want %d: %+v", len(res.Nodes), want, res.Nodes)
	}

	modID := idutil.MakeID("pkg", module)
	m, ok := nodesByID[modID]
	if !ok {
		t.Fatalf("missing module node %q; have %+v", modID, res.Nodes)
	}
	if m.FileType != "package" || m.Label != module {
		t.Errorf("module node = %+v, want file_type=package label=%q", m, module)
	}

	edges := map[string]bool{}
	for _, e := range res.Edges {
		if e.Relation != "depends_on" {
			t.Errorf("unexpected relation %q on edge %+v", e.Relation, e)
		}
		edges[e.Source+"->"+e.Target] = true
	}
	if len(res.Edges) != len(deps) {
		t.Errorf("edges = %d, want %d: %+v", len(res.Edges), len(deps), res.Edges)
	}
	for _, dep := range deps {
		depID := idutil.MakeID("pkg", dep)
		n, ok := nodesByID[depID]
		if !ok {
			t.Errorf("missing dependency node %q (%s); have %+v", depID, dep, res.Nodes)
			continue
		}
		if n.FileType != "package" || n.Label != dep {
			t.Errorf("dependency node = %+v, want file_type=package label=%q", n, dep)
		}
		if !edges[modID+"->"+depID] {
			t.Errorf("missing depends_on edge %s->%s; have %+v", modID, depID, res.Edges)
		}
	}
}

func TestIntrospectManifestsGoMod(t *testing.T) {
	root := t.TempDir()
	writeManifestFile(t, root, "go.mod", "module github.com/example/app\n\ngo 1.21\n\n"+
		"require (\n\tgithub.com/pkg/errors v0.9.1\n\tgolang.org/x/text v0.3.0 // indirect\n)\n")
	assertPackageGraph(t, root, "github.com/example/app",
		[]string{"github.com/pkg/errors", "golang.org/x/text"})
}

func TestIntrospectManifestsPyproject(t *testing.T) {
	root := t.TempDir()
	writeManifestFile(t, root, "pyproject.toml",
		"[project]\nname = \"myproj\"\nversion = \"1.0.0\"\ndependencies = [\"requests>=2.0\", \"flask\"]\n")
	assertPackageGraph(t, root, "myproj", []string{"requests", "flask"})
}

func TestIntrospectManifestsPom(t *testing.T) {
	root := t.TempDir()
	// A default xmlns exercises the namespace-agnostic (local-name) matching.
	writeManifestFile(t, root, "pom.xml", `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <groupId>com.example</groupId>
  <artifactId>myapp</artifactId>
  <version>1.0</version>
  <dependencies>
    <dependency>
      <groupId>org.apache.commons</groupId>
      <artifactId>commons-lang3</artifactId>
      <version>3.12.0</version>
    </dependency>
    <dependency>
      <groupId>com.google.guava</groupId>
      <artifactId>guava</artifactId>
      <version>31.0</version>
    </dependency>
  </dependencies>
</project>
`)
	assertPackageGraph(t, root, "com.example:myapp",
		[]string{"org.apache.commons:commons-lang3", "com.google.guava:guava"})
}
