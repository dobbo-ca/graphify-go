// Package langfamily maps a source file's extension to its language interop
// family. A call in one language can never bind by name to a definition in a
// different family — a Python function does not invoke a Java method, a TSX
// component does not call a Kotlin fun — so a name-matched "calls" edge across
// families is a phantom label collision and must never be emitted or kept.
//
// Families group by REAL interop so legitimate cross-language resolution keeps
// working: Java/Kotlin/Scala share the JVM, C/C++ share headers and symbols,
// and JS/TS variants compile into one module graph. Extensions absent from the
// map resolve to "" (unknown: docs, configs, unrecognized languages) and are
// never filtered, preserving the previous permissive default.
package langfamily

import (
	"path/filepath"
	"strings"
)

// byExt groups the extensions of every language the extractors emit call/def
// symbols for into interop families.
var byExt = map[string]string{
	// Go
	".go": "go",
	// Rust
	".rs": "rust",
	// JS/TS module graph
	".js": "jsts", ".jsx": "jsts", ".mjs": "jsts", ".cjs": "jsts",
	".ts": "jsts", ".mts": "jsts", ".cts": "jsts", ".tsx": "jsts",
	// Python
	".py": "py",
	// JVM interop (Java/Kotlin/Scala)
	".java": "jvm", ".kt": "jvm", ".kts": "jvm", ".scala": "jvm", ".sc": "jvm",
	// C-family: shared headers, C/C++ mix
	".c": "native", ".h": "native", ".cc": "native", ".cpp": "native",
	".cxx": "native", ".hpp": "native", ".hh": "native", ".hxx": "native",
	// .NET
	".cs": "dotnet",
	// Ruby
	".rb": "ruby",
	// PHP
	".php": "php", ".phtml": "php",
	// Lua
	".lua": "lua",
	// Julia
	".jl": "julia",
	// Zig
	".zig": "zig",
	// Verilog / SystemVerilog
	".v": "verilog", ".sv": "verilog", ".svh": "verilog", ".vh": "verilog",
	// Shell
	".sh": "shell", ".bash": "shell",
}

// Of returns the interop family of sourceFile's language, or "" when the
// extension is unknown or sourceFile is empty.
func Of(sourceFile string) string {
	if sourceFile == "" {
		return ""
	}
	return byExt[strings.ToLower(filepath.Ext(sourceFile))]
}

// Cross reports whether a and b belong to different, both-known families — the
// condition under which a name-matched call between them is phantom. Unknown
// families ("") are never treated as cross, keeping the permissive default for
// non-code and unrecognized files.
func Cross(a, b string) bool {
	fa, fb := Of(a), Of(b)
	return fa != "" && fb != "" && fa != fb
}
