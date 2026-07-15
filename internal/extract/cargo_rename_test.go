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
