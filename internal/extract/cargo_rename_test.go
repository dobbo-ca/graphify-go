package extract

import (
	"path/filepath"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// A dependency renamed via `package = "..."` must bind to the workspace crate of
// that real name, not the dep-table key. Before the fix the edge was dropped
// because the lookup used the key "db" (#1858).
func TestIntrospectCargoHonorsPackageRename(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `
[workspace]
members = ["app", "storage"]
`)
	writeManifest(t, filepath.Join(root, "app"), `
[package]
name = "app"
version = "0.1.0"

[dependencies]
db = { path = "../storage", package = "internal-storage" }
`)
	writeManifest(t, filepath.Join(root, "storage"), `
[package]
name = "internal-storage"
version = "0.1.0"
`)

	res, err := IntrospectCargo(root)
	if err != nil {
		t.Fatalf("IntrospectCargo: %v", err)
	}
	wantEdge := model.Edge{
		Source: "crate:app", Target: "crate:internal-storage", Relation: "crate_depends_on",
		Confidence: "EXTRACTED", Weight: 1.0, SourceFile: "app/Cargo.toml", SourceLocation: "L1",
	}
	if !hasEdge(res, wantEdge) {
		t.Errorf("rename not honored: missing edge %+v; got %+v", wantEdge, res.Edges)
	}
	// Exactly one edge — no spurious edge to a phantom crate:db (the raw dep key).
	if len(res.Edges) != 1 {
		t.Errorf("want exactly 1 edge, got %d: %+v", len(res.Edges), res.Edges)
	}
}

// A rename pointing at a registry/external crate (no workspace member of that
// name) must stay a no-op — no edge (#1858).
func TestIntrospectCargoRenameToExternalIsNoEdge(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `
[workspace]
members = ["app"]
`)
	writeManifest(t, filepath.Join(root, "app"), `
[package]
name = "app"
version = "0.1.0"

[dependencies]
tokio_rt = { version = "1", package = "tokio" }
`)
	res, err := IntrospectCargo(root)
	if err != nil {
		t.Fatalf("IntrospectCargo: %v", err)
	}
	if len(res.Edges) != 0 {
		t.Errorf("rename to a non-workspace crate must yield no edge, got %+v", res.Edges)
	}
}

// A member inheriting `db = { workspace = true }` takes its package rename from
// the root [workspace.dependencies] table.
func TestIntrospectCargoInheritedRename(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `
[workspace]
members = ["app", "crates/real"]

[workspace.dependencies]
db = { path = "crates/real", package = "real" }
`)
	writeManifest(t, filepath.Join(root, "app"), `
[package]
name = "app"
version = "0.1.0"

[dependencies]
db = { workspace = true }
`)
	writeManifest(t, filepath.Join(root, "crates", "real"), `
[package]
name = "real"
version = "0.1.0"
`)
	res, err := IntrospectCargo(root)
	if err != nil {
		t.Fatalf("IntrospectCargo: %v", err)
	}
	want := model.Edge{
		Source: "crate:app", Target: "crate:real", Relation: "crate_depends_on",
		Confidence: "EXTRACTED", Weight: 1.0, SourceFile: "app/Cargo.toml", SourceLocation: "L1",
	}
	if !hasEdge(res, want) || len(res.Edges) != 1 {
		t.Errorf("want only %+v, got %+v", want, res.Edges)
	}
}

// An inherited registry dep sharing a workspace crate's name is not internal.
func TestIntrospectCargoInheritedRegistryNameIsNoEdge(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `
[workspace]
members = ["app", "util"]

[workspace.dependencies]
util = "1"
`)
	writeManifest(t, filepath.Join(root, "app"), `
[package]
name = "app"
version = "0.1.0"

[dependencies]
util = { workspace = true }
`)
	writeManifest(t, filepath.Join(root, "util"), `
[package]
name = "util"
version = "0.1.0"
`)
	res, err := IntrospectCargo(root)
	if err != nil {
		t.Fatalf("IntrospectCargo: %v", err)
	}
	if len(res.Edges) != 0 {
		t.Errorf("registry dep must yield no edge, got %+v", res.Edges)
	}
}

// Relative roots (`build .`) must compare paths like absolute ones.
func TestIntrospectCargoInheritedRelativeRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "E")
	writeManifest(t, root, `
[workspace]
members = ["app", "crates/dd"]

[workspace.dependencies]
dd = { path = "../E/crates/dd" }
abs = { path = "`+filepath.Join(root, "crates", "dd")+`", package = "dd" }
`)
	writeManifest(t, filepath.Join(root, "app"), `
[package]
name = "app"
version = "0.1.0"

[dependencies]
dd = { workspace = true }
abs = { workspace = true }
`)
	writeManifest(t, filepath.Join(root, "crates", "dd"), `
[package]
name = "dd"
version = "0.1.0"
`)
	t.Chdir(root)
	res, err := IntrospectCargo(".")
	if err != nil {
		t.Fatalf("IntrospectCargo: %v", err)
	}
	if len(res.Edges) != 2 {
		t.Errorf("want 2 edges (dd, abs), got %+v", res.Edges)
	}
}
