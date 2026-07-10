package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateURLScheme(t *testing.T) {
	for _, bad := range []string{"file:///etc/passwd", "ftp://host/x", "data:text/plain,hi"} {
		if err := ValidateURL(bad); err == nil {
			t.Errorf("ValidateURL(%q) = nil, want error", bad)
		}
	}
}

func TestValidateURLBlocksLoopback(t *testing.T) {
	// Resolves to 127.0.0.1 without a network call.
	if err := ValidateURL("http://localhost/x"); err == nil {
		t.Error("ValidateURL(localhost) = nil, want SSRF block")
	}
}

func TestValidateGraphPathContainment(t *testing.T) {
	base := t.TempDir()
	inside := filepath.Join(base, "graph.json")
	os.WriteFile(inside, []byte("{}"), 0o644)

	if _, err := ValidateGraphPath(inside, base); err != nil {
		t.Errorf("inside path rejected: %v", err)
	}
	escape := filepath.Join(base, "..", "etc")
	if _, err := ValidateGraphPath(escape, base); err == nil {
		t.Error("path escaping base was accepted, want error")
	}
}

func TestSanitizeLabel(t *testing.T) {
	if got := SanitizeLabel("a\x00b\x1fc"); got != "abc" {
		t.Errorf("SanitizeLabel stripped wrong: %q", got)
	}
}

func TestMaxGraphFileBytes(t *testing.T) {
	const defaultCap = int64(MaxGraphFileBytes)
	for _, tc := range []struct {
		name string
		set  bool
		env  string
		want int64
	}{
		{"unset", false, "", defaultCap},
		{"blank", true, "  ", defaultCap},
		{"gb suffix", true, "1GB", 1073741824},
		{"mb suffix lowercase", true, "640mb", 640 * 1024 * 1024},
		{"plain bytes", true, "123456", 123456},
		{"invalid", true, "not-a-number", defaultCap},
		{"zero", true, "0", defaultCap},
		{"negative", true, "-5", defaultCap},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("GRAPHIFY_MAX_GRAPH_BYTES", tc.env)
			} else {
				os.Unsetenv("GRAPHIFY_MAX_GRAPH_BYTES")
			}
			if got := maxGraphFileBytes(); got != tc.want {
				t.Errorf("maxGraphFileBytes() = %d, want %d", got, tc.want)
			}
		})
	}
}
