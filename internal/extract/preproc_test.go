package extract

import "testing"

// C items behind a preprocessor conditional or an extern "C" block are extracted.
func TestCItemsInsideConditionals(t *testing.T) {
	guard := "#ifndef GUARD_H\n#define GUARD_H\n#include <stdio.h>\n#include \"other.h\"\n" +
		"static inline int inl(int a) { return a; }\n" +
		"#ifdef __cplusplus\nextern \"C\" {\n#endif\nint in_extern(void) { return inl(1); }\n#ifdef __cplusplus\n}\n#endif\n#endif\n"
	a := "int base(void) { return 0; }\n" +
		"#ifdef A\nint in_ifdef(void) { return base(); }\n#else\n#include \"guard.h\"\nint in_else(void) { return base(); }\n#endif\n" +
		"#if X\nint in_if(void) { return 1; }\n#elif Y\nint in_elif(void) { return 2; }\n#else\n#if Z\nint nested(void) { return in_if(); }\n#endif\n#endif\n"
	nodes, edges := resolveSrc(t, [][2]string{{"guard.h", guard}, {"other.h", "int o;\n"}, {"a.c", a}})
	for _, want := range [][2]string{
		{"guard.h", "inl()"}, {"guard.h", "in_extern()"},
		{"a.c", "in_ifdef()"}, {"a.c", "in_else()"}, {"a.c", "in_if()"}, {"a.c", "in_elif()"}, {"a.c", "nested()"},
	} {
		if !nodes[want] {
			t.Errorf("missing node %v", want)
		}
	}
	for _, want := range [][3]string{
		{"guard.h:in_extern()", "calls", "guard.h:inl()"},
		{"a.c:in_else()", "calls", "a.c:base()"},
		{"a.c:nested()", "calls", "a.c:in_if()"},
		{"guard.h:guard.h", "imports_from", "other.h:other.h"},
		{"a.c:a.c", "imports_from", "guard.h:guard.h"},
	} {
		if !edges[want] {
			t.Errorf("missing edge %v\n%v", want, edges)
		}
	}
}

// C++ items behind a conditional, in an extern "C" block, and methods behind
// a conditional in a class body are extracted.
func TestCppItemsInsideConditionals(t *testing.T) {
	hpp := "#ifndef W_HPP\n#define W_HPP\n#include \"dep.hpp\"\nnamespace ns {\n#ifdef FEATURE\n" +
		"class Widget : public Base {\npublic:\n  void draw() {}\n#if EXTRA\n  void extra() { draw(); }\n#else\n  void plain() {}\n#endif\n};\n" +
		"template <typename T> T tmpl(T v) { return v; }\n#endif\n}\n" +
		"#ifdef __cplusplus\nextern \"C\" {\n#endif\nint c_api(void) { return 0; }\n#ifdef __cplusplus\n}\n#endif\n#endif\n"
	e := "extern \"C\" {\nint plain_extern(void) { return 0; }\n}\nextern \"C\" int single_extern(void) { return plain_extern(); }\n"
	nodes, edges := resolveSrc(t, [][2]string{
		{"w.hpp", hpp}, {"dep.hpp", "class Base {};\n"}, {"e.cpp", e},
	})
	for _, want := range [][2]string{
		{"w.hpp", "Widget"}, {"w.hpp", "Widget.draw()"}, {"w.hpp", "Widget.extra()"}, {"w.hpp", "Widget.plain()"},
		{"w.hpp", "tmpl()"}, {"w.hpp", "c_api()"}, {"e.cpp", "plain_extern()"}, {"e.cpp", "single_extern()"},
	} {
		if !nodes[want] {
			t.Errorf("missing node %v", want)
		}
	}
	for _, want := range [][3]string{
		{"w.hpp:Widget", "contains", "w.hpp:Widget.extra()"},
		{"w.hpp:Widget.extra()", "calls", "w.hpp:Widget.draw()"},
		{"w.hpp:Widget", "inherits", "dep.hpp:Base"},
		{"w.hpp:w.hpp", "imports_from", "dep.hpp:dep.hpp"},
		{"e.cpp:single_extern()", "calls", "e.cpp:plain_extern()"},
	} {
		if !edges[want] {
			t.Errorf("missing edge %v\n%v", want, edges)
		}
	}
}

// A signature split by a conditional is a parse error; the definitions under
// the ERROR node are still extracted.
func TestCItemsUnderParseError(t *testing.T) {
	src := "#ifdef A\nint split(int a) {\n#else\nint split(long a) {\n#endif\n  return after();\n}\nint after(void) { return 0; }\n"
	nodes, _ := resolveSrc(t, [][2]string{{"e.c", src}})
	if !nodes[[2]string{"e.c", "after()"}] {
		t.Errorf("missing after(): %v", nodes)
	}
}

// A function defined in both branches of a conditional is one call target.
func TestCallToFunctionDefinedInBothBranches(t *testing.T) {
	a := "int x1(void) { return 0; }\n#ifdef A\nint dup(void) { return x1(); }\n#else\nint dup(void) { return x1(); }\n#endif\n"
	_, edges := resolveSrc(t, [][2]string{{"a.c", a}, {"b.c", "int user(void) { return dup(); }\n"}})
	if want := [3]string{"b.c:user()", "calls", "a.c:dup()"}; !edges[want] {
		t.Errorf("missing edge %v\n%v", want, edges)
	}
}

// A struct without a body is a reference, and a typedef struct in a C++
// header is a definition.
func TestStructForwardDeclAndTypedef(t *testing.T) {
	nodes, _ := resolveSrc(t, [][2]string{
		{"a.c", "struct fwd;\nstruct var_t v;\nstruct real { int a; };\n"},
		{"n.hpp", "typedef struct Named { int a; } NamedT;\n"},
	})
	if nodes[[2]string{"a.c", "fwd"}] || nodes[[2]string{"a.c", "var_t"}] {
		t.Errorf("node for a struct without a body: %v", nodes)
	}
	if !nodes[[2]string{"a.c", "real"}] || !nodes[[2]string{"n.hpp", "Named"}] {
		t.Errorf("missing struct node: %v", nodes)
	}
}

// An overload declared abstract and then defined keeps its concrete target.
func TestAbstractThenConcreteOverloadStillBinds(t *testing.T) {
	_, edges := resolveSrc(t, [][2]string{
		{"a.scala", "trait T {\n  def run(x: Int): Int\n  def run(x: String): Int = 0\n}\n"},
		{"b.scala", "trait U {\n  def run(x: Int): Int\n}\n"},
		{"c.scala", "object C {\n  def go(): Int = run(\"a\")\n}\n"},
	})
	if want := [3]string{"c.scala:C.go()", "calls", "a.scala:T.run()"}; !edges[want] {
		t.Errorf("missing edge %v\n%v", want, edges)
	}
}
