package extract

import (
	"regexp"
	"strings"
	"unsafe"

	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tstsx "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
)

// Single-file components (.vue/.svelte/.astro) wrap a JS/TS script region in
// markup a JS grammar cannot parse. Feeding the whole file to the parser yields
// a top-level ERROR node, so defs and imports are silently dropped. Instead we
// mask every non-script region with blanks (newlines preserved so line numbers
// stay accurate) and run the existing JS/TS extractor over the masked source,
// then a regex pass recovers dynamic `import('…')` calls the AST does not edge
// (including template-layer imports the mask blanked out). Mirrors upstream
// extract_vue/extract_svelte/extract_astro.

// componentScriptRE matches a <script> block, capturing its open tag, body, and
// close tag. The open-tag matcher skips over quoted attribute values so a `>`
// inside one (e.g. `<script setup lang="ts" generic="T extends A<B>">`) does not
// prematurely end the tag.
var componentScriptRE = regexp.MustCompile(`(?i)(<script\b(?:"[^"]*"|'[^']*'|[^>"'])*>)([\s\S]*?)(</script\s*>)`)

// componentScriptLangRE reads the declared `lang` from a script open tag.
var componentScriptLangRE = regexp.MustCompile(`(?i)\blang\s*=\s*['"]?([A-Za-z]+)['"]?`)

// astroFrontmatterRE matches the leading `---\n…\n---` TypeScript frontmatter
// fence of an .astro file, capturing the code between the fences.
var astroFrontmatterRE = regexp.MustCompile(`\A\s*---\s*\r?\n([\s\S]*?)\r?\n---\s*(?:\r?\n|\z)`)

// dynamicImportRE matches a dynamic `import('…')` specifier.
var dynamicImportRE = regexp.MustCompile(`import\(\s*['"]([^'"]+)['"]\s*\)`)

// extractComponent extracts a single-file component. isAstro also keeps the
// leading `---…---` frontmatter fence as a script region.
func extractComponent(rel string, src []byte, isAstro bool) Result {
	masked, lang := maskComponentScript(src, isAstro)
	res := extractJS(rel, masked, componentLangPtr(lang))

	// Regex fallback for dynamic imports the AST never reaches, whether they live
	// in the script body or the template layer the mask blanked out.
	fileID := idutil.MakeID(rel)
	for _, m := range dynamicImportRE.FindAllSubmatchIndex(src, -1) {
		spec := string(src[m[2]:m[3]])
		if spec == "" {
			continue
		}
		res.Imps = append(res.Imps, Imp{FileID: fileID, File: rel, Spec: spec, Loc: "L" + itoa(lineOf(src, m[0]))})
	}
	return res
}

// maskComponentScript blanks everything outside script regions, replacing each
// non-newline byte with a space so the JS/TS grammar sees only the script while
// preserved \r/\n keep line numbers accurate. It returns the masked source and
// the first script block's declared lang (lowercased, "" when unset). For .astro
// the leading frontmatter fence is also kept.
func maskComponentScript(src []byte, isAstro bool) ([]byte, string) {
	masked := make([]byte, len(src))
	for i, c := range src {
		if c == '\n' || c == '\r' {
			masked[i] = c
		} else {
			masked[i] = ' '
		}
	}
	if isAstro {
		if m := astroFrontmatterRE.FindSubmatchIndex(src); m != nil {
			copy(masked[m[2]:m[3]], src[m[2]:m[3]])
		}
	}
	var lang string
	for _, m := range componentScriptRE.FindAllSubmatchIndex(src, -1) {
		copy(masked[m[4]:m[5]], src[m[4]:m[5]]) // script body, verbatim
		if lang == "" {
			if lm := componentScriptLangRE.FindSubmatch(src[m[2]:m[3]]); lm != nil {
				lang = strings.ToLower(string(lm[1]))
			}
		}
	}
	return masked, lang
}

// componentLangPtr picks the grammar for a script block's declared lang: TS for
// `ts`, TSX for `tsx`, else JS (js/jsx/unset). TS is a superset of JS, so JS is a
// safe default for a script that declares no lang.
func componentLangPtr(lang string) unsafe.Pointer {
	switch lang {
	case "ts":
		return tstsx.LanguageTypescript()
	case "tsx":
		return tstsx.LanguageTSX()
	default:
		return tsjs.Language()
	}
}

// lineOf returns the 1-based line number of byte offset off in src.
func lineOf(src []byte, off int) int {
	line := 1
	for i := 0; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			line++
		}
	}
	return line
}
