package langfamily

import "testing"

// .rake is plain Ruby: it must join the ruby interop family so a call in a .rake
// file can bind to a .rb definition, but not to a different-family def (#1784).
func TestRakeIsRubyFamily(t *testing.T) {
	if got := Of("tasks/build.rake"); got != "ruby" {
		t.Errorf("Of(.rake) = %q, want ruby", got)
	}
	if Cross("a.rake", "b.rb") {
		t.Error(".rake and .rb must be the same (ruby) family — a cross-resolve must not be blocked")
	}
	if !Cross("a.rake", "b.go") {
		t.Error(".rake and .go must be different families — a name collision must be blocked")
	}
}
