package detect

import (
	"os"
	"path/filepath"
	"testing"
)

// .terraform holds the provider/module cache a `terraform init` downloads. It is
// always generated, never source, and can hold tens of thousands of vendored
// .tf files. It must be skipped by the built-in rule even when no .gitignore
// covers it — e.g. when graphify builds a subdirectory of a repo whose
// .terraform ignore rule lives in the repo-root .gitignore.
func TestCollectFilesSkipsTerraform(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.tf"), `resource "aws_x" "y" {}`)
	mustWrite(t, filepath.Join(root, ".terraform", "modules", "vendored.tf"), `resource "aws_z" "w" {}`)

	files, err := CollectFiles(root)
	if err != nil {
		t.Fatalf("CollectFiles: %v", err)
	}
	for _, f := range files {
		if filepath.ToSlash(f) != "main.tf" {
			t.Errorf("expected only main.tf, got %q", f)
		}
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d: %v", len(files), files)
	}
}

// MCP config files carry a .json extension that is matched by basename; general
// .json files (e.g. package.json) are also collected now that .json is a
// supported extension, and the extractor decides per-file whether it is a
// config/manifest worth indexing.
func TestCollectFilesIncludesMCPConfigs(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".mcp.json"), `{"mcpServers": {}}`)
	mustWrite(t, filepath.Join(root, "mcp_servers.json"), `{"mcpServers": {}}`)
	mustWrite(t, filepath.Join(root, "sub", "claude_desktop_config.json"), `{"mcpServers": {}}`)
	mustWrite(t, filepath.Join(root, "package.json"), `{}`)

	files, err := CollectFiles(root)
	if err != nil {
		t.Fatalf("CollectFiles: %v", err)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[filepath.ToSlash(f)] = true
	}
	for _, want := range []string{".mcp.json", "mcp_servers.json", "sub/claude_desktop_config.json", "package.json"} {
		if !got[want] {
			t.Errorf("expected %q to be collected, got %v", want, files)
		}
	}
}

// A generic secret keyword in a filename must not drop a genuine source file
// (password_reset.go is a module, not a secret store), while specific credential
// stores (credentials.json, secrets.yaml, .env, id_rsa, *.pem) must still be
// dropped. Mirrors the upstream _is_sensitive / _generic_keyword_hit refinement.
func TestIsSensitiveSourceExemption(t *testing.T) {
	kept := []string{
		"password_reset.go",
		"passwords_controller.rb",
		"credentials_controller.rb",
		"secret_store.py",
	}
	for _, name := range kept {
		if isSensitive(name) {
			t.Errorf("isSensitive(%q) = true, want false (real source, not a secret store)", name)
		}
	}
	dropped := []string{
		"credentials.json",
		"secrets.yaml",
		".env",
		"id_rsa",
		"server.pem",
	}
	for _, name := range dropped {
		if !isSensitive(name) {
			t.Errorf("isSensitive(%q) = false, want true (secret store)", name)
		}
	}
}

// CollectFiles must keep source files whose names merely contain a secret keyword
// and still drop a genuine secret store (credentials.json) that shares a supported
// extension — exercising the real isSensitive path end-to-end.
func TestCollectFilesKeepsKeywordNamedSource(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "password_reset.go"), `package p`)
	mustWrite(t, filepath.Join(root, "passwords_controller.rb"), `class PasswordsController; end`)
	mustWrite(t, filepath.Join(root, "credentials_controller.rb"), `class CredentialsController; end`)
	mustWrite(t, filepath.Join(root, "secret_store.py"), `x = 1`)
	mustWrite(t, filepath.Join(root, "credentials.json"), `{"password": "hunter2"}`)

	files, err := CollectFiles(root)
	if err != nil {
		t.Fatalf("CollectFiles: %v", err)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[filepath.ToSlash(f)] = true
	}
	for _, want := range []string{"password_reset.go", "passwords_controller.rb", "credentials_controller.rb", "secret_store.py"} {
		if !got[want] {
			t.Errorf("expected %q to be collected, got %v", want, files)
		}
	}
	if got["credentials.json"] {
		t.Errorf("expected credentials.json to be dropped as a secret store, got %v", files)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
