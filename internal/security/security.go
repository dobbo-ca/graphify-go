// Package security holds the guards every piece of external input passes
// through: URL validation (SSRF), graph-file path containment, file-size caps,
// and label sanitisation. It mirrors the threat model of the Python original's
// graphify/security.py.
package security

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// MaxGraphFileBytes rejects graph.json files larger than this before parsing,
	// so a crafted multi-gigabyte file cannot exhaust memory.
	MaxGraphFileBytes = 512 * 1024 * 1024
	maxLabelLen       = 256
)

var blockedHosts = map[string]bool{
	"metadata.google.internal": true,
	"metadata.google.com":      true,
}

// ValidateURL rejects any URL that is not http/https or that resolves to a
// private, loopback, link-local, or cloud-metadata address (SSRF defence).
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if s := strings.ToLower(u.Scheme); s != "http" && s != "https" {
		return fmt.Errorf("blocked URL scheme %q - only http and https are allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL %q has no host", raw)
	}
	if blockedHosts[strings.ToLower(host)] {
		return fmt.Errorf("blocked cloud metadata endpoint %q", host)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %q: %w", host, err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("blocked private/internal IP %s (resolved from %q)", ip, host)
		}
	}
	return nil
}

// cgnNet is RFC 6598 shared address space (carrier-grade NAT), which IsPrivate misses.
var cgnNet = mustCIDR("100.64.0.0/10")

func isBlockedIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || cgnNet.Contains(ip)
}

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// ValidateGraphPath resolves path and verifies it stays inside base (the
// graphify-out directory), which must exist. Prevents path traversal when a
// caller is handed an attacker-influenced graph path.
func ValidateGraphPath(path, base string) (string, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(absBase); err != nil || !info.IsDir() {
		return "", fmt.Errorf("graph base directory does not exist: %s", absBase)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absBase, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the allowed directory %s", path, absBase)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("graph file not found: %s", abs)
	}
	return abs, nil
}

// CheckGraphFileSize returns an error if path exceeds MaxGraphFileBytes.
func CheckGraphFileSize(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil // existence is the caller's concern; nothing to cap
	}
	if info.Size() > MaxGraphFileBytes {
		return fmt.Errorf("graph file %s is %d bytes, exceeds %d-byte cap", path, info.Size(), MaxGraphFileBytes)
	}
	return nil
}

var controlChars = regexp.MustCompile(`[\x00-\x1f\x7f]`)

// SanitizeLabel strips control characters and caps length, making a label safe
// to embed in JSON or plain text. For raw HTML injection, additionally escape it.
func SanitizeLabel(s string) string {
	s = controlChars.ReplaceAllString(s, "")
	if len(s) > maxLabelLen {
		s = s[:maxLabelLen]
	}
	return s
}
