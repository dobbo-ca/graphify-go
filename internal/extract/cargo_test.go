package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// writeManifest creates dir (if needed) and writes a Cargo.toml with the given
// body, trimming the leading newline so test literals can stay indented.
func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(strings.TrimPrefix(body, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", dir, err)
	}
}

func nodeIDs(res Result) map[string]bool {
	ids := map[string]bool{}
	for _, n := range res.Nodes {
		ids[n.ID] = true
	}
	return ids
}

func hasNode(res Result, want model.Node) bool {
	for _, n := range res.Nodes {
		if n == want {
			return true
		}
	}
	return false
}

func hasEdge(res Result, want model.Edge) bool {
	for _, e := range res.Edges {
		if e == want {
			return true
		}
	}
	return false
}

func TestIntrospectCargoWorkspaceInternalDependencyOnly(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `
[workspace]
members = ["app", "core"]
`)
	writeManifest(t, filepath.Join(root, "app"), `
[package]
name = "app"
version = "0.1.0"
edition = "2021"

[dependencies]
core = { path = "../core" }
serde = "1"
`)
	writeManifest(t, filepath.Join(root, "core"), `
[package]
name = "core"
version = "0.1.0"
edition = "2021"
`)

	res, err := IntrospectCargo(root)
	if err != nil {
		t.Fatalf("IntrospectCargo: %v", err)
	}

	ids := nodeIDs(res)
	if !ids["crate:app"] || !ids["crate:core"] || len(ids) != 2 {
		t.Fatalf("node ids = %v, want only crate:app and crate:core", ids)
	}
	if ids["crate:serde"] {
		t.Error("registry dep serde must not be a node")
	}
	if !hasNode(res, model.Node{ID: "crate:app", Label: "app", FileType: "code", SourceFile: "app/Cargo.toml", SourceLocation: "L1"}) {
		t.Error("missing crate:app node with expected fields")
	}
	wantEdge := model.Edge{
		Source: "crate:app", Target: "crate:core", Relation: "crate_depends_on",
		Confidence: "EXTRACTED", Weight: 1.0, SourceFile: "app/Cargo.toml", SourceLocation: "L1",
	}
	if !hasEdge(res, wantEdge) {
		t.Errorf("missing internal edge %+v; got %+v", wantEdge, res.Edges)
	}
	for _, e := range res.Edges {
		if e.Target == "crate:serde" {
			t.Error("must not emit an edge to registry dep serde")
		}
	}
}

func TestIntrospectCargoMalformedTOMLReportsError(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `
[package
name = "broken"
`)

	if _, err := IntrospectCargo(root); err == nil {
		t.Fatal("expected a TOML parse error for a malformed manifest")
	}
}

func TestIntrospectCargoDegenerateManifests(t *testing.T) {
	// Empty manifest: no package, no workspace -> nothing.
	empty := filepath.Join(t.TempDir(), "empty")
	writeManifest(t, empty, "")
	res, err := IntrospectCargo(empty)
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if len(res.Nodes) != 0 || len(res.Edges) != 0 {
		t.Errorf("empty manifest produced %d nodes / %d edges, want 0/0", len(res.Nodes), len(res.Edges))
	}

	// Nameless package: a [package] table without a name yields no crate.
	nameless := filepath.Join(t.TempDir(), "nameless")
	writeManifest(t, nameless, `
[package]
version = "0.1.0"
`)
	res, err = IntrospectCargo(nameless)
	if err != nil {
		t.Fatalf("nameless: %v", err)
	}
	if len(res.Nodes) != 0 || len(res.Edges) != 0 {
		t.Errorf("nameless package produced %d nodes / %d edges, want 0/0", len(res.Nodes), len(res.Edges))
	}

	// Scalar dependencies: a non-table dependencies value is ignored, the node stays.
	scalar := filepath.Join(t.TempDir(), "scalar")
	writeManifest(t, scalar, `
[package]
name = "app"
version = "0.1.0"

dependencies = "not-a-table"
`)
	res, err = IntrospectCargo(scalar)
	if err != nil {
		t.Fatalf("scalar: %v", err)
	}
	if len(res.Nodes) != 1 || !hasNode(res, model.Node{ID: "crate:app", Label: "app", FileType: "code", SourceFile: "Cargo.toml", SourceLocation: "L1"}) {
		t.Errorf("scalar deps: nodes = %+v, want single crate:app", res.Nodes)
	}
	if len(res.Edges) != 0 {
		t.Errorf("scalar deps produced %d edges, want 0", len(res.Edges))
	}
}

func TestIntrospectCargoVirtualAndRootPackageWorkspaces(t *testing.T) {
	// Virtual root: a glob member pattern + workspace.dependencies, deps via { workspace = true }.
	virtual := t.TempDir()
	writeManifest(t, virtual, `
[workspace]
members = ["crates/*"]

[workspace.dependencies]
beta = { path = "crates/beta" }
serde = "1"
`)
	writeManifest(t, filepath.Join(virtual, "crates", "alpha"), `
[package]
name = "alpha"
version = "0.1.0"
edition = "2021"

[dependencies]
beta = { workspace = true }
serde = { workspace = true }
`)
	writeManifest(t, filepath.Join(virtual, "crates", "beta"), `
[package]
name = "beta"
version = "0.1.0"
edition = "2021"
`)
	res, err := IntrospectCargo(virtual)
	if err != nil {
		t.Fatalf("virtual: %v", err)
	}
	if ids := nodeIDs(res); !ids["crate:alpha"] || !ids["crate:beta"] || len(ids) != 2 {
		t.Fatalf("virtual node ids = %v, want crate:alpha + crate:beta", ids)
	}
	if len(res.Edges) != 1 {
		t.Fatalf("virtual edges = %d, want 1", len(res.Edges))
	}
	if !hasEdge(res, model.Edge{
		Source: "crate:alpha", Target: "crate:beta", Relation: "crate_depends_on",
		Confidence: "EXTRACTED", Weight: 1.0, SourceFile: "crates/alpha/Cargo.toml", SourceLocation: "L1",
	}) {
		t.Errorf("missing alpha->beta edge; got %+v", res.Edges)
	}

	// Root-package workspace: the root manifest is itself a package member.
	pkgRoot := t.TempDir()
	writeManifest(t, pkgRoot, `
[package]
name = "root_pkg"
version = "0.1.0"
edition = "2021"

[workspace]
members = ["crates/*"]
`)
	writeManifest(t, filepath.Join(pkgRoot, "crates", "member"), `
[package]
name = "member"
version = "0.1.0"
edition = "2021"

[dependencies]
root_pkg = { path = "../.." }
`)
	res, err = IntrospectCargo(pkgRoot)
	if err != nil {
		t.Fatalf("pkgRoot: %v", err)
	}
	if ids := nodeIDs(res); !ids["crate:root_pkg"] || !ids["crate:member"] || len(ids) != 2 {
		t.Fatalf("pkgRoot node ids = %v, want crate:root_pkg + crate:member", ids)
	}
	if !hasEdge(res, model.Edge{
		Source: "crate:member", Target: "crate:root_pkg", Relation: "crate_depends_on",
		Confidence: "EXTRACTED", Weight: 1.0, SourceFile: "crates/member/Cargo.toml", SourceLocation: "L1",
	}) {
		t.Errorf("missing member->root_pkg edge; got %+v", res.Edges)
	}
}

func TestIntrospectCargoLargeWorkspaceChain(t *testing.T) {
	const crateCount = 200
	root := t.TempDir()
	writeManifest(t, root, `
[workspace]
members = ["crates/*"]
`)
	for i := 0; i < crateCount; i++ {
		dep := ""
		if i+1 < crateCount {
			dep = fmt.Sprintf("\n[dependencies]\ncrate_%03d = { path = \"../crate_%03d\" }\n", i+1, i+1)
		}
		writeManifest(t, filepath.Join(root, "crates", fmt.Sprintf("crate_%03d", i)), fmt.Sprintf(`
[package]
name = "crate_%03d"
version = "0.1.0"
edition = "2021"
%s`, i, dep))
	}

	res, err := IntrospectCargo(root)
	if err != nil {
		t.Fatalf("IntrospectCargo: %v", err)
	}
	if len(res.Nodes) != crateCount {
		t.Errorf("nodes = %d, want %d", len(res.Nodes), crateCount)
	}
	if len(res.Edges) != crateCount-1 {
		t.Errorf("edges = %d, want %d", len(res.Edges), crateCount-1)
	}
	if !hasEdge(res, model.Edge{
		Source: "crate:crate_000", Target: "crate:crate_001", Relation: "crate_depends_on",
		Confidence: "EXTRACTED", Weight: 1.0, SourceFile: "crates/crate_000/Cargo.toml", SourceLocation: "L1",
	}) {
		t.Error("missing head-of-chain edge crate_000->crate_001")
	}
	if !hasEdge(res, model.Edge{
		Source: "crate:crate_198", Target: "crate:crate_199", Relation: "crate_depends_on",
		Confidence: "EXTRACTED", Weight: 1.0, SourceFile: "crates/crate_198/Cargo.toml", SourceLocation: "L1",
	}) {
		t.Error("missing tail-of-chain edge crate_198->crate_199")
	}
}

// A corpus with no Cargo.toml at the root yields a clean error, so the build
// command can surface it rather than silently producing nothing.
func TestIntrospectCargoNoRootManifest(t *testing.T) {
	if _, err := IntrospectCargo(t.TempDir()); err == nil {
		t.Fatal("expected an error when root has no Cargo.toml")
	}
}
