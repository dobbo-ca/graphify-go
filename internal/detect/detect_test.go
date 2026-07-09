package detect

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		// Document formats carry a supported extension (markdown is extracted
		// structurally) but upstream classifies them as FileType.DOCUMENT, not
		// CODE, so a secret-keyword doc is NOT exempt and must be dropped.
		"secret_notes.md",
		"credentials.md",
		"password.mdx",
		"secrets.markdown",
	}
	for _, name := range dropped {
		if !isSensitive(name) {
			t.Errorf("isSensitive(%q) = false, want true (secret store)", name)
		}
	}
}

// Two generic keywords sitting directly adjacent (only a separator between them)
// must both be detected: the trailing keyword ends the stem and names the secret
// store, so the data file is dropped. Regression for the RE2 boundary emulation —
// a consuming boundary class plus non-overlapping FindAll ate the shared
// separator and lost the trailing keyword, keeping the secret store. Mirrors
// upstream _generic_keyword_hit (finditer over zero-width lookarounds).
func TestIsSensitiveAdjacentKeywords(t *testing.T) {
	dropped := []string{
		"aws_secret_credentials.json", // >2 words, trailing keyword ends the stem
		"foo_secret_password.txt",     // >2 words, trailing keyword ends the stem
	}
	for _, name := range dropped {
		if !isSensitive(name) {
			t.Errorf("isSensitive(%q) = false, want true (trailing keyword names a secret store)", name)
		}
	}
	// A trailing keyword in a genuine source file is still exempt (it is a module).
	kept := []string{
		"aws_secret_credentials.go", // same shape but real source -> exempt
	}
	for _, name := range kept {
		if isSensitive(name) {
			t.Errorf("isSensitive(%q) = true, want false (real source, not a secret store)", name)
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

// An unreadable directory must be recorded in the walk-errors report rather than
// silently swallowed — swallowing it truncates the enumeration into a partial
// graph.json with no trace. The walk must still continue past the failure and
// collect readable siblings. Skipped when running as root (root bypasses the
// permission bits) or on Windows (chmod 000 does not restrict directory reads
// there). Mirrors upstream detect.py's walk_errors surfacing.
func TestCollectFilesReportsWalkError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 does not restrict directory reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permission bits")
	}
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.go"), `package p`)
	locked := filepath.Join(root, "locked")
	mustWrite(t, filepath.Join(locked, "hidden.go"), `package p`)
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) }) // restore so TempDir cleanup can remove it

	rep, err := CollectFilesReport(root)
	if err != nil {
		t.Fatalf("CollectFilesReport: %v", err)
	}
	if len(rep.WalkErrors) == 0 {
		t.Fatalf("expected a walk error for the unreadable dir, got none")
	}
	found := false
	for _, we := range rep.WalkErrors {
		if strings.Contains(we, "locked") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a walk error mentioning %q, got %v", "locked", rep.WalkErrors)
	}
	// The walk must continue past the failure: the readable sibling is still collected.
	got := map[string]bool{}
	for _, f := range rep.Files {
		got[filepath.ToSlash(f)] = true
	}
	if !got["main.go"] {
		t.Errorf("expected main.go collected despite the unreadable sibling dir, got %v", rep.Files)
	}
}

// Files walked past but not collected because no extractor handles their
// extension must be counted (previously they left no trace at all), mirroring
// upstream detect.py's `unclassified` count. Supported files are not counted;
// deliberately-skipped lock files are not counted.
func TestCollectFilesReportCountsSkipped(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.go"), `package p`)      // collected
	mustWrite(t, filepath.Join(root, "notes.unknownext"), `x`)     // no extractor -> counted
	mustWrite(t, filepath.Join(root, "image.png"), `x`)            // no extractor -> counted
	mustWrite(t, filepath.Join(root, "Makefile"), `all:`)          // extensionless -> counted
	mustWrite(t, filepath.Join(root, "go.sum"), `x`)               // lock file -> NOT counted

	rep, err := CollectFilesReport(root)
	if err != nil {
		t.Fatalf("CollectFilesReport: %v", err)
	}
	if rep.Skipped != 3 {
		t.Errorf("expected 3 skipped (no-extractor) files, got %d", rep.Skipped)
	}
	if len(rep.Files) != 1 || filepath.ToSlash(rep.Files[0]) != "main.go" {
		t.Errorf("expected only main.go collected, got %v", rep.Files)
	}
}

// .mts/.cts are TypeScript module/CommonJS source and must be collected, while
// an extensionless file is collected only when its first line is a shebang
// naming an interpreter that has a Go extractor (bash, python, ...). A plain
// extensionless file and an interpreter with no extractor (perl) stay skipped.
func TestCollectFilesShebangAndMts(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "mod.mts"), `export const x = 1;`)
	mustWrite(t, filepath.Join(root, "cmod.cts"), `export const y = 2;`)
	mustWrite(t, filepath.Join(root, "deploy"), "#!/usr/bin/env bash\necho hi\n")
	mustWrite(t, filepath.Join(root, "run.py.helper"), "") // has an (unsupported) ext, not extensionless
	mustWrite(t, filepath.Join(root, "notes"), "just some text, no shebang\n")
	mustWrite(t, filepath.Join(root, "oldscript"), "#!/usr/bin/perl\nprint 1;\n") // perl: no Go extractor

	files, err := CollectFiles(root)
	if err != nil {
		t.Fatalf("CollectFiles: %v", err)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[filepath.ToSlash(f)] = true
	}
	for _, want := range []string{"mod.mts", "cmod.cts", "deploy"} {
		if !got[want] {
			t.Errorf("expected %q to be collected, got %v", want, files)
		}
	}
	for _, notWant := range []string{"run.py.helper", "notes", "oldscript"} {
		if got[notWant] {
			t.Errorf("did not expect %q to be collected, got %v", notWant, files)
		}
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
