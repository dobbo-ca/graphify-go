package extract

import "testing"

// A .vue SFC's <script> block must yield its function defs and import edges even
// though the surrounding template/style is not valid JS. The mask preserves line
// numbers, so the function node keeps its true source location, and a template-
// layer dynamic import is recovered by the regex fallback.
func TestExtractVueComponent(t *testing.T) {
	src := []byte(`<template>
  <component :is="() => import('./LazyWidget.vue')" />
  <div>{{ greet() }}</div>
</template>
<script setup lang="ts">
import { helper } from './helper'
import { ref } from 'vue'

function greet(): string {
  return helper('hi')
}
</script>
<style scoped>
.foo { color: red; }
</style>
`)
	rel := "components/Greeter.vue"
	res := FileFromBytes(rel, src)

	// Function node from the <script> block, at its true (masked-preserving) line.
	var greet bool
	for _, n := range res.Nodes {
		if n.Label == "greet()" {
			greet = true
			if n.SourceLocation != "L9" {
				t.Errorf("greet() source location = %q, want L9", n.SourceLocation)
			}
		}
	}
	if !greet {
		t.Errorf("missing greet() function node; nodes=%v", res.Nodes)
	}

	// Static import from the <script> block plus the template-layer dynamic import.
	imps := map[string]bool{}
	for _, im := range res.Imps {
		imps[im.Spec] = true
	}
	for _, want := range []string{"./helper", "vue", "./LazyWidget.vue"} {
		if !imps[want] {
			t.Errorf("missing import %q; imps=%v", want, res.Imps)
		}
	}

	// Resolve turns the bare 'vue' import into an external concept + imports edge.
	ext := Resolve([]Result{res}, []string{rel})
	var importEdge bool
	for _, e := range ext.Edges {
		if e.Relation == "imports" && e.SourceFile == rel {
			importEdge = true
		}
	}
	if !importEdge {
		t.Errorf("no import edge produced for %s", rel)
	}
}
