package extract

import (
	"encoding/json"
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/model"
)

// javaBuilder names its steps like methods of an unrelated class, so only
// the declared return types can pick the right ones.
func javaBuilder(client string) map[string]string {
	return map[string]string{
		"Product.java": "class Product { void ship() {} }\n",
		"Builder.java": "class Builder {\n" +
			"    Builder size(int n) { return this; }\n" +
			"    Product build() { return new Product(); }\n" +
			"}\n",
		"Factory.java": "class Factory { static Builder builder() { return new Builder(); } }\n",
		"Decoy.java":   "class Decoy { Decoy size(int n) { return this; } void build() {} void ship() {} }\n",
		"Client.java":  client,
	}
}

func TestJavaChainTypedFromReturnTypes(t *testing.T) {
	wantEdges(t, javaBuilder("class Client { void run() { Factory.builder().size(3).build().ship(); } }\n"),
		[][3]string{
			{"Client.run()", "calls", "Builder.size()"},
			{"Client.run()", "calls", "Builder.build()"},
			{"Client.run()", "calls", "Product.ship()"},
		},
		[3]string{"Client.run()", "calls", "Decoy.size()"},
		[3]string{"Client.run()", "calls", "Decoy.build()"},
		[3]string{"Client.run()", "calls", "Decoy.ship()"},
	)
}

func TestJavaChainNewCastBareThisSuperReceivers(t *testing.T) {
	wantEdges(t, javaBuilder("class Client {\n"+
		"    Builder mine() { return new Builder(); }\n"+
		"    void viaNew() { new Builder().size(1); }\n"+
		"    void viaCast(Object o) { ((Builder) o).build(); }\n"+
		"    void viaBare() { mine().build(); }\n"+
		"    void viaField() { this.factory().size(2); }\n"+
		"    Builder factory() { return Factory.builder(); }\n"+
		"}\n"+
		"class Kid extends Builder { void viaSuper() { super.size(1).build(); } }\n"),
		[][3]string{
			{"Client.viaNew()", "calls", "Builder.size()"},
			{"Client.viaCast()", "calls", "Builder.build()"},
			{"Client.viaBare()", "calls", "Builder.build()"},
			{"Client.viaField()", "calls", "Builder.size()"},
			{"Kid.viaSuper()", "calls", "Builder.build()"},
		},
		[3]string{"Client.viaNew()", "calls", "Decoy.size()"},
		[3]string{"Client.viaBare()", "calls", "Decoy.build()"},
	)
}

// Upstream nests each Builder; nested types have no node here.
func TestJavaChainReturnTypeNamedInSeveralFiles(t *testing.T) {
	srcs := map[string]string{
		"Alpha.java": "class Alpha { static Builder builder() { return new Builder(); } }\n" +
			"class Builder { Alpha get() { return null; } }\n",
		"Beta.java":   "class Beta {}\nclass Builder { Beta get() { return null; } }\n",
		"Client.java": "class Client { void run() { Alpha.builder().get(); } }\n",
	}
	var results []Result
	var files []string
	for rel, src := range srcs {
		results = append(results, FileFromBytes(rel, []byte(src)))
		files = append(files, rel)
	}
	ext := Resolve(results, files)
	file := map[string]string{}
	for _, n := range ext.Nodes {
		file[n.ID] = n.SourceFile
	}
	var got []string
	for _, e := range ext.Edges {
		if e.Relation == model.RelCalls && file[e.Source] == "Client.java" && file[e.Target] != "Client.java" {
			got = append(got, e.Target)
		}
	}
	alphaGet, betaGet := "alpha_java_builder_get", "beta_java_builder_get"
	if !contains(got, alphaGet) || contains(got, betaGet) {
		t.Errorf("Client.run calls %v; want %s and not %s", got, alphaGet, betaGet)
	}
}

// A type parameter, a library type or an untyped local carries no type.
func TestJavaChainUntypedLinkStaysUnlinked(t *testing.T) {
	srcs := javaBuilder("class Client<T> {\n" +
		"    T self() { return null; }\n" +
		"    String text() { return \"\"; }\n" +
		"    void viaTypeParam() { self().build(); }\n" +
		"    void viaLibrary() { text().length(); }\n" +
		"    void viaVar() { var b = Factory.builder(); b.size(1).build(); }\n" +
		"}\n")
	srcs["String.java"] = "class String { int length() { return 0; } }\n"
	srcs["T.java"] = "class T { void build() {} }\n"
	wantEdges(t, srcs, nil,
		[3]string{"Client.viaLibrary()", "calls", "String.length()"},
		[3]string{"Client.viaTypeParam()", "calls", "T.build()"},
		[3]string{"Client.viaTypeParam()", "calls", "Builder.build()"},
		[3]string{"Client.viaTypeParam()", "calls", "Decoy.build()"},
		[3]string{"Client.viaVar()", "calls", "Builder.build()"},
		[3]string{"Client.viaVar()", "calls", "Decoy.build()"},
	)
}

// Upstream types this from the field; no local type table here yet.
func TestJavaChainLowercaseRootUnlinked(t *testing.T) {
	wantEdges(t, map[string]string{
		"Services.java": "class Gateway { void charge() {} Gateway create() { return this; } }\n" +
			"class Checkout { Gateway factory; void chained() { factory.create().charge(); } }\n",
	}, nil, [3]string{"Checkout.chained()", "calls", "Gateway.charge()"})
}

func TestJavaChainOverloadsMustAgree(t *testing.T) {
	wantEdges(t, javaBuilder("class Client {\n"+
		"    Builder same() { return null; }\n"+
		"    Builder same(int n) { return null; }\n"+
		"    Product mixed(int n) { return null; }\n"+
		"    Builder mixed() { return null; }\n"+
		"    void agree() { same().build(); }\n"+
		"    void disagree() { mixed().build(); }\n"+
		"}\n"),
		[][3]string{{"Client.agree()", "calls", "Builder.build()"}},
		[3]string{"Client.disagree()", "calls", "Builder.build()"},
	)
}

// One base of another language stops the bases walk.
func TestJavaChainNonJVMBaseBindsNothing(t *testing.T) {
	wantEdges(t, map[string]string{
		"base.py": "class Base:\n    def run(self):\n        pass\n",
		"Sub.java": "interface JRun { void run(); }\n" +
			"interface Sub extends Base, JRun {}\n" +
			"interface JSub extends JRun {}\n",
		"Client.java": "class Client { void viaPy(Object o) { ((Sub) o).run(); } void viaJava(Object o) { ((JSub) o).run(); } }\n",
	},
		[][3]string{{"Client.viaJava()", "calls", "JRun.run()"}},
		[3]string{"Client.viaPy()", "calls", "JRun.run()"},
		[3]string{"Client.viaPy()", "calls", "Base.run()"},
	)
}

// A builtin base gets no upstream edge, so the walk goes on.
func TestJavaChainBuiltinBaseIsTransparent(t *testing.T) {
	wantEdges(t, map[string]string{
		"J.java":   "interface J { void jm(); }\n",
		"I.java":   "interface I extends J, Comparable<I> {}\n",
		"Use.java": "class Use { static I i() { return null; } void u() { i().jm(); } }\n",
	}, [][3]string{{"Use.u()", "calls", "J.jm()"}})
}

// Upstream's caller there is the inner method.
func TestJavaChainInAnonymousClassUnlinked(t *testing.T) {
	wantEdges(t, map[string]string{
		"Types.java": "class Tool { void go() {} }\nclass Item { void stop() {} }\nclass Other { void stop() {} }\n",
		"Outer.java": "class Outer {\n" +
			"    Tool helper() { return null; }\n" +
			"    Item item() { return null; }\n" +
			"    void start() { Runnable r = new Runnable() {\n" +
			"        Other item() { return null; }\n" +
			"        public void run() { helper().go(); this.item().stop(); }\n" +
			"    }; }\n" +
			"}\n",
	}, nil,
		[3]string{"Outer.start()", "calls", "Tool.go()"},
		[3]string{"Outer.start()", "calls", "Item.stop()"},
		[3]string{"Outer.start()", "calls", "Other.stop()"},
	)
}

// An unchanged file comes back from the cache as JSON.
func TestJavaChainThroughCachedResult(t *testing.T) {
	srcs := javaBuilder("class Client { void run() { Factory.builder().size(3).build(); } }\n")
	var results []Result
	var files []string
	for rel, src := range srcs {
		r := FileFromBytes(rel, []byte(src))
		if rel != "Client.java" {
			b, err := r.AppendJSON(nil)
			if err != nil {
				t.Fatal(err)
			}
			r = Result{}
			if err := json.Unmarshal(b, &r); err != nil || !r.CallsOK() {
				t.Fatalf("decode %s: %v", rel, err)
			}
		}
		results = append(results, r)
		files = append(files, rel)
	}
	ext := Resolve(results, files)
	for _, e := range ext.Edges {
		if e.Source == "client_java_client_run" && e.Target == "builder_java_builder_build" {
			if e.Confidence != model.ConfInferred || e.ConfidenceScore != 0.85 {
				t.Errorf("edge %+v; want INFERRED 0.85", e)
			}
			return
		}
	}
	t.Error("missing Client.run --calls--> Builder.build after a cache round trip")
}

// Upstream's nested and anonymous Items make the name ambiguous.
func TestJavaChainNestedNamesakeUnlinked(t *testing.T) {
	for _, shadow := range []string{
		"class Outer { static class Item { void run() {} } }\n",
		"class Anon { void a() { new Item() { void run() {} }; } }\n",
	} {
		wantEdges(t, map[string]string{
			"Item.java":   "class Item { void run() {} }\n",
			"Shadow.java": shadow,
			"Use.java":    "class Use { void u() { new Item().run(); } }\n",
			"Own.java":    "class Own { static class Item { void run() {} } void u() { new Item().run(); } }\n",
		}, nil,
			[3]string{"Use.u()", "calls", "Item.run()"},
			[3]string{"Own.u()", "calls", "Item.run()"},
		)
	}
}

// Upstream binds the nested Builder, which has no node here.
func TestJavaChainNestedReturnTypeUnlinked(t *testing.T) {
	wantEdges(t, map[string]string{
		"Builder.java": "class Builder { Builder size(int n) { return this; } void build() {} }\n",
		"Foo.java": "class Foo {\n" +
			"    static Builder builder() { return null; }\n" +
			"    static class Builder { Builder size(int n) { return this; } Foo build() { return null; } }\n" +
			"    void make() { builder().size(3).build(); }\n" +
			"}\n",
		"Use.java": "class Use { void u() { Foo.builder().size(3).build(); } }\n",
	}, nil,
		[3]string{"Foo.make()", "calls", "Builder.size()"},
		[3]string{"Foo.make()", "calls", "Builder.build()"},
		[3]string{"Use.u()", "calls", "Builder.size()"},
		[3]string{"Use.u()", "calls", "Builder.build()"},
	)
}
