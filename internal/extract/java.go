package extract

import (
	"strings"
	"unicode"
	"unicode/utf8"

	tsjava "github.com/dobbo-ca/gositter/grammars/java"
	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/langfamily"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// extractJava pulls types (class/interface/enum/record/annotation), their
// methods, imports, and call edges out of a .java file. Java has no top-level
// functions, so every method is scoped under its enclosing type. Methods are
// registered under their bare name so `x.method()` call sites resolve.
func extractJava(rel string, src []byte) Result {
	root, done := parseRoot(src, tsjava.Language())
	defer done()
	b := newBuilder(rel)

	for i := uint(0); i < root.ChildCount(); i++ {
		n := root.Child(i)
		switch n.Kind() {
		case "class_declaration", "interface_declaration", "enum_declaration",
			"record_declaration", "annotation_type_declaration":
			b.javaType(n, src)
		case "import_declaration":
			b.javaImport(n, src)
		}
	}
	for _, d := range b.res.Defs {
		if t := b.javaRets[d.ID]; t != "" {
			b.res.JavaRets = append(b.res.JavaRets, d.ID+" "+t)
			delete(b.javaRets, d.ID)
		}
	}
	// Upstream's nested and anonymous types shadow namesakes.
	seen := map[string]bool{}
	walkKinds(root, javaShadowKinds, func(cv ts.Node) bool {
		n, name := &cv, ""
		if n.Kind() != "object_creation_expression" {
			if n.Parent().Kind() != "program" {
				name = fieldText(n, "name", src)
			}
		} else if t := n.ChildByFieldName("type"); t != nil && t.Kind() == "type_identifier" && childByKind(n, "class_body") != nil {
			name = t.Utf8Text(src)
		}
		if name != "" && !seen[name] {
			seen[name] = true
			b.res.JavaRets = append(b.res.JavaRets, " "+name)
		}
		return true
	})
	return b.res
}

var javaShadowKinds = tsjava.Language().KindMask("class_declaration", "interface_declaration",
	"enum_declaration", "record_declaration", "annotation_type_declaration", "object_creation_expression")

// javaType records a type definition and the methods/constructors in its body,
// scoping each method under the type's name.
func (b *builder) javaType(n *ts.Node, src []byte) {
	name := fieldText(n, "name", src)
	if name == "" {
		return
	}
	typeID := b.id(name)
	b.def(typeID, name, name, line(n))
	skip := javaTypeParams(n, src, javaNestedTypes(n, src, map[string]bool{}))
	b.javaSupertypes(typeID, n, skip, src)
	b.javaAnnotations(typeID, n, skip, src)
	if n.Kind() == "record_declaration" {
		b.javaRecordComponents(typeID, n, skip, src)
	}

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	for i := uint(0); i < body.ChildCount(); i++ {
		m := body.Child(i)
		switch m.Kind() {
		case "method_declaration", "constructor_declaration":
			b.javaMember(typeID, name, m, skip, src)
		case "field_declaration":
			b.javaTypeRefs(typeID, m.ChildByFieldName("type"), skip, false, src)
		case "annotation_type_element_declaration":
			b.javaTypeRefs(typeID, m.ChildByFieldName("type"), skip, true, src)
		case "enum_constant":
			b.javaEnumConstant(typeID, m, src)
		case "enum_body_declarations":
			// Enum methods have no node here, only the fields count.
			for j := uint(0); j < m.NamedChildCount(); j++ {
				if f := m.NamedChild(j); f.Kind() == "field_declaration" {
					b.javaTypeRefs(typeID, f.ChildByFieldName("type"), skip, false, src)
				}
			}
		}
	}
}

// javaMember records a method/constructor scoped under ownerID (a type or an
// enum constant with an anonymous body), labelled ownerName.method(). It is
// registered under the bare method name so `x.method()` call sites resolve.
func (b *builder) javaMember(ownerID, ownerName string, m *ts.Node, skip map[string]bool, src []byte) {
	mname := fieldText(m, "name", src)
	if mname == "" {
		return
	}
	mid := idutil.MakeID(ownerID, mname)
	b.addNode(mid, ownerName+"."+mname+"()", line(m))
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: ownerID, Target: mid, Relation: model.RelContains,
		Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(m),
	})
	b.res.Defs = append(b.res.Defs, Def{ID: mid, Name: mname, File: b.file})
	b.javaCalls(m.ChildByFieldName("body"), mid, src)
	if m.Kind() == "method_declaration" {
		if b.javaRets == nil {
			b.javaRets = map[string]string{}
		}
		// Overloads share a node: keep agreement.
		bindType(b.javaRets, mid, javaRecvType(m.ChildByFieldName("type"), src))
	}

	skip = javaTypeParams(m, src, skip)
	if ps := m.ChildByFieldName("parameters"); ps != nil {
		for i := uint(0); i < ps.NamedChildCount(); i++ {
			// A varargs parameter gives nothing upstream.
			if p := ps.NamedChild(i); p.Kind() == "formal_parameter" {
				b.javaTypeRefs(mid, p.ChildByFieldName("type"), skip, false, src)
			}
		}
	}
	b.javaTypeRefs(mid, m.ChildByFieldName("type"), skip, false, src)
	b.javaAnnotations(mid, m, skip, src)
}

// javaEnumConstant records an enum constant as a `case_of` the enum type. A
// constant with an anonymous class body (`MONDAY { void greet(){} }`) descends
// so the body's methods attach to the constant rather than being dropped.
func (b *builder) javaEnumConstant(typeID string, n *ts.Node, src []byte) {
	cname := fieldText(n, "name", src)
	if cname == "" {
		return
	}
	cid := idutil.MakeID(typeID, cname)
	b.addNode(cid, cname, line(n))
	b.res.Edges = append(b.res.Edges, model.Edge{
		Source: typeID, Target: cid, Relation: model.RelCaseOf,
		Confidence: model.ConfExtracted, SourceFile: b.src, SourceLocation: line(n),
	})
	for i := uint(0); i < n.ChildCount(); i++ {
		body := n.Child(i)
		if body.Kind() != "class_body" {
			continue
		}
		for j := uint(0); j < body.ChildCount(); j++ {
			m := body.Child(j)
			switch m.Kind() {
			case "method_declaration", "constructor_declaration":
				b.javaMember(cid, cname, m, nil, src)
			case "field_declaration":
				b.javaTypeRefs(cid, m.ChildByFieldName("type"), nil, false, src)
			}
		}
	}
}

// javaImport records the imported type or package path. A trailing `.*`
// (on-demand import) is stripped to the package path.
func (b *builder) javaImport(n *ts.Node, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c.Kind() == "scoped_identifier" || c.Kind() == "identifier" {
			b.imp(strings.TrimSuffix(c.Utf8Text(src), ".*"), line(n))
			return
		}
	}
}

var javaCallKinds = tsjava.Language().KindMask("method_invocation")

// javaCalls walks a method body and records each invocation. Both direct calls
// (`f()`) and object/method calls (`x.f()`) are recorded by the invoked name.
func (b *builder) javaCalls(body *ts.Node, callerID string, src []byte) {
	if body == nil {
		return
	}
	walkKinds(body, javaCallKinds, func(cv ts.Node) bool {
		c := &cv
		if name := c.ChildByFieldName("name"); name != nil {
			recv := recvText(c, name, src)
			if obj := c.ChildByFieldName("object"); obj != nil {
				if k := obj.Kind(); k != "identifier" && k != "this" && k != "super" && !javaInNestedType(c, body) {
					if chain, ok := javaChain(obj, src, 0); ok {
						recv = "#" + chain
					}
				}
			}
			b.callRecv(callerID, name.Utf8Text(src), recv, line(c))
		}
		return true
	})
}

// javaInNestedType: upstream's caller there is the inner method.
func javaInNestedType(c, body *ts.Node) bool {
	for p := c.Parent(); p != nil && p.StartByte() > body.StartByte(); p = p.Parent() {
		switch p.Kind() {
		case "class_body", "enum_body", "interface_body":
			return true
		}
	}
	return false
}

// javaChain encodes root then ".call" per link.
func javaChain(n *ts.Node, src []byte, depth int) (string, bool) {
	if n == nil || depth > 16 {
		return "", false
	}
	switch n.Kind() {
	case "parenthesized_expression":
		if n.NamedChildCount() == 1 {
			return javaChain(n.NamedChild(0), src, depth+1)
		}
	case "object_creation_expression", "cast_expression":
		t := javaRecvType(n.ChildByFieldName("type"), src)
		return t, t != ""
	case "this", "super":
		return n.Kind(), true
	case "identifier":
		// Untyped without a local type table.
		t := n.Utf8Text(src)
		r, _ := utf8.DecodeRuneInString(t)
		return t, unicode.IsUpper(r)
	case "method_invocation":
		name := n.ChildByFieldName("name")
		if name == nil {
			return "", false
		}
		inner := ""
		if obj := n.ChildByFieldName("object"); obj != nil {
			var ok bool
			if inner, ok = javaChain(obj, src, depth+1); !ok {
				return "", false
			}
		}
		return inner + "." + name.Utf8Text(src), true
	}
	return "", false
}

// javaRecvType names a resolvable declared type.
func javaRecvType(t *ts.Node, src []byte) string {
	if t == nil {
		return ""
	}
	name := ""
	switch t.Kind() {
	case "type_identifier":
		name = t.Utf8Text(src)
	case "scoped_type_identifier":
		raw := t.Utf8Text(src)
		name = raw[strings.LastIndex(raw, ".")+1:]
	case "generic_type":
		for i := uint(0); i < t.ChildCount(); i++ {
			if c := t.Child(i); c.Kind() == "type_identifier" || c.Kind() == "scoped_type_identifier" {
				return javaRecvType(c, src)
			}
		}
		return ""
	default:
		return ""
	}
	if name == "" || javaBuiltin[name] {
		return ""
	}
	for p := t.Parent(); p != nil; p = p.Parent() {
		switch p.Kind() {
		case "class_declaration", "interface_declaration", "record_declaration",
			"method_declaration", "constructor_declaration":
			if javaTypeParams(p, src, nil)[name] {
				return ""
			}
		}
	}
	return name
}

// javaSupertypes records the declared parents of a type: `extends X` on a class
// is an inherits edge, `implements I, J` are implements edges, and `extends` on
// an interface is interface inheritance (inherits).
func (b *builder) javaSupertypes(typeID string, n *ts.Node, skip map[string]bool, src []byte) {
	if sup := n.ChildByFieldName("superclass"); sup != nil && sup.NamedChildCount() > 0 {
		b.javaBase(typeID, sup.NamedChild(0), "inherits", skip, src)
	}
	if ifs := n.ChildByFieldName("interfaces"); ifs != nil {
		b.javaTypeList(typeID, ifs, "implements", skip, src)
	}
	if n.Kind() == "interface_declaration" {
		for i := uint(0); i < n.ChildCount(); i++ {
			if c := n.Child(i); c.Kind() == "extends_interfaces" {
				b.javaTypeList(typeID, c, "inherits", skip, src)
			}
		}
	}
}

// javaBase records a parent type, and a references edge for each of its
// type arguments. An unresolved parent gets a stub unless its name is a
// java.* builtin, which only binds to a definition of the corpus.
func (b *builder) javaBase(typeID string, t *ts.Node, relation string, skip map[string]bool, src []byte) {
	name := baseTypeName(t.Utf8Text(src))
	if javaBuiltin[name] {
		b.typeRef(typeID, name, relation, line(t))
	} else {
		b.ref(typeID, name, relation, false, line(t))
	}
	if t.Kind() == "generic_type" {
		b.javaTypeArgs(typeID, t, skip, src)
	}
}

// javaTypeList records every type named in the `type_list` under n.
func (b *builder) javaTypeList(typeID string, n *ts.Node, relation string, skip map[string]bool, src []byte) {
	for i := uint(0); i < n.ChildCount(); i++ {
		list := n.Child(i)
		if list.Kind() != "type_list" {
			continue
		}
		for j := uint(0); j < list.NamedChildCount(); j++ {
			b.javaBase(typeID, list.NamedChild(j), relation, skip, src)
		}
	}
}

// javaBuiltin is upstream's list of java.* type names that give no reference.
var javaBuiltin = map[string]bool{}

func init() {
	for _, n := range []string{
		"Object", "String", "CharSequence", "StringBuilder", "StringBuffer",
		"Number", "Byte", "Short", "Integer", "Long", "Float", "Double", "Boolean",
		"Character", "Void", "Class", "Enum", "Record", "Math", "System", "Thread",
		"Runnable", "Comparable", "Iterable", "Cloneable", "AutoCloseable",
		"Appendable", "Readable", "Process", "ProcessBuilder", "Runtime", "Package",
		"ThreadLocal", "InheritableThreadLocal", "Throwable", "Exception",
		"RuntimeException", "Error", "IllegalArgumentException",
		"IllegalStateException", "NullPointerException",
		"IndexOutOfBoundsException", "ArrayIndexOutOfBoundsException",
		"ClassCastException", "NumberFormatException", "ArithmeticException",
		"UnsupportedOperationException", "InterruptedException",
		"CloneNotSupportedException", "SecurityException", "StackOverflowError",
		"OutOfMemoryError", "AssertionError", "Collection", "List", "ArrayList",
		"LinkedList", "Vector", "Stack", "Set", "HashSet", "LinkedHashSet",
		"TreeSet", "SortedSet", "NavigableSet", "EnumSet", "Map", "HashMap",
		"LinkedHashMap", "TreeMap", "SortedMap", "NavigableMap", "Hashtable",
		"EnumMap", "Properties", "Queue", "Deque", "ArrayDeque", "PriorityQueue",
		"Iterator", "ListIterator", "Comparator", "Optional", "OptionalInt",
		"OptionalLong", "OptionalDouble", "Collections", "Arrays", "Objects",
		"Date", "Calendar", "Random", "UUID", "Scanner", "StringJoiner",
		"StringTokenizer", "BitSet", "Spliterator", "Locale",
		"NoSuchElementException", "ConcurrentModificationException", "Stream",
		"IntStream", "LongStream", "DoubleStream", "Collector", "Collectors",
		"Function", "BiFunction", "Consumer", "BiConsumer", "Supplier", "Predicate",
		"BiPredicate", "UnaryOperator", "BinaryOperator", "IntFunction",
		"ToIntFunction", "ToLongFunction", "ToDoubleFunction", "Callable", "Future",
		"CompletableFuture", "CompletionStage", "Executor", "ExecutorService",
		"Executors", "ScheduledExecutorService", "TimeUnit", "ConcurrentHashMap",
		"ConcurrentMap", "CopyOnWriteArrayList", "BlockingQueue", "CountDownLatch",
		"Semaphore", "CyclicBarrier", "AtomicInteger", "AtomicLong",
		"AtomicBoolean", "AtomicReference", "Instant", "Duration", "Period",
		"LocalDate", "LocalTime", "LocalDateTime", "ZonedDateTime",
		"OffsetDateTime", "ZoneId", "ZoneOffset", "DayOfWeek", "Month", "Year",
		"Clock", "DateTimeFormatter", "IOException", "UncheckedIOException",
		"FileNotFoundException", "File", "InputStream", "OutputStream", "Reader",
		"Writer", "BufferedReader", "BufferedWriter", "InputStreamReader",
		"OutputStreamWriter", "FileReader", "FileWriter", "PrintStream",
		"PrintWriter", "ByteArrayInputStream", "ByteArrayOutputStream",
		"Serializable", "Closeable", "Path", "Paths", "Files", "BigDecimal",
		"BigInteger",
	} {
		javaBuiltin[n] = true
	}
}

// javaNestedTypes adds the names of the types nested in n to out. They have
// no node, so a ref to one must not bind to a namesake elsewhere.
func javaNestedTypes(n *ts.Node, src []byte, out map[string]bool) map[string]bool {
	if body := n.ChildByFieldName("body"); body != nil {
		for i := uint(0); i < body.NamedChildCount(); i++ {
			c := body.NamedChild(i)
			switch c.Kind() {
			case "class_declaration", "interface_declaration", "enum_declaration",
				"record_declaration", "annotation_type_declaration":
				out[fieldText(c, "name", src)] = true
				javaNestedTypes(c, src, out)
			}
		}
	}
	return out
}

// javaTypeParams returns skip plus the type parameters n declares.
func javaTypeParams(n *ts.Node, src []byte, skip map[string]bool) map[string]bool {
	tp := n.ChildByFieldName("type_parameters")
	if tp == nil {
		return skip
	}
	out := map[string]bool{}
	for k := range skip {
		out[k] = true
	}
	for i := uint(0); i < tp.NamedChildCount(); i++ {
		p := tp.NamedChild(i)
		for j := uint(0); j < p.NamedChildCount(); j++ {
			if id := p.NamedChild(j); id.Kind() == "type_identifier" {
				out[id.Utf8Text(src)] = true
				break
			}
		}
	}
	return out
}

// javaTypeRefs records a references edge from id to each type t names,
// including type arguments. Primitives, builtins and type parameters in
// skip give nothing; a qualified name keeps its last segment, or all of it
// when qual is set.
func (b *builder) javaTypeRefs(id string, t *ts.Node, skip map[string]bool, qual bool, src []byte) {
	if t == nil {
		return
	}
	switch t.Kind() {
	case "integral_type", "floating_point_type", "boolean_type", "void_type":
	case "type_identifier":
		if name := t.Utf8Text(src); !skip[name] && !javaBuiltin[name] {
			b.ref(id, name, "references", false, line(t))
		}
	case "scoped_type_identifier":
		b.javaScoped(id, t, qual, src)
	case "generic_type":
		for i := uint(0); i < t.NamedChildCount(); i++ {
			c := t.NamedChild(i)
			if c.Kind() == "scoped_type_identifier" {
				b.javaScoped(id, c, qual, src)
				break
			}
			if c.Kind() == "type_identifier" {
				b.javaTypeRefs(id, c, skip, qual, src)
				break
			}
		}
		b.javaTypeArgs(id, t, skip, src)
	default:
		for i := uint(0); i < t.NamedChildCount(); i++ {
			b.javaTypeRefs(id, t.NamedChild(i), skip, qual, src)
		}
	}
}

func (b *builder) javaScoped(id string, t *ts.Node, qual bool, src []byte) {
	raw := t.Utf8Text(src)
	simple := raw[strings.LastIndex(raw, ".")+1:]
	if javaBuiltin[simple] {
		return
	}
	if !qual {
		b.refQ(id, simple, "references", strings.TrimSuffix(raw, "."+simple), line(t))
		return
	}
	b.ref(id, raw, "references", false, line(t))
}

// javaTypeArgs records the types in the type arguments of the generic_type t.
func (b *builder) javaTypeArgs(id string, t *ts.Node, skip map[string]bool, src []byte) {
	for i := uint(0); i < t.NamedChildCount(); i++ {
		if args := t.NamedChild(i); args.Kind() == "type_arguments" {
			for j := uint(0); j < args.NamedChildCount(); j++ {
				b.javaTypeRefs(id, args.NamedChild(j), skip, false, src)
			}
		}
	}
}

// javaRecordComponents records the component types of a record.
func (b *builder) javaRecordComponents(typeID string, n *ts.Node, skip map[string]bool, src []byte) {
	ps := n.ChildByFieldName("parameters")
	if ps == nil {
		return
	}
	for i := uint(0); i < ps.NamedChildCount(); i++ {
		p := ps.NamedChild(i)
		switch p.Kind() {
		case "formal_parameter":
			b.javaTypeRefs(typeID, p.ChildByFieldName("type"), skip, false, src)
		case "spread_parameter":
			for j := uint(0); j < p.NamedChildCount(); j++ {
				if c := p.NamedChild(j); c.Kind() != "modifiers" && c.Kind() != "variable_declarator" {
					b.javaTypeRefs(typeID, c, skip, false, src)
					break
				}
			}
		}
	}
}

// javaAnnotations records the annotations in the modifiers of declaration n,
// and the class literals in their arguments, once per name. A qualified
// annotation keeps its whole name.
func (b *builder) javaAnnotations(id string, n *ts.Node, skip map[string]bool, src []byte) {
	seen := map[string]bool{}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		mods := n.NamedChild(i)
		if mods.Kind() != "modifiers" {
			continue
		}
		for j := uint(0); j < mods.NamedChildCount(); j++ {
			a := mods.NamedChild(j)
			if a.Kind() != "marker_annotation" && a.Kind() != "annotation" {
				continue
			}
			if nm := a.ChildByFieldName("name"); nm != nil && !seen[nm.Utf8Text(src)] {
				seen[nm.Utf8Text(src)] = true
				b.ref(id, nm.Utf8Text(src), "references", false, line(a))
			}
			if args := a.ChildByFieldName("arguments"); args != nil {
				b.javaClassLiterals(id, args, skip, seen, src)
			}
		}
		break
	}
}

// javaClassLiterals records the types of the `T.class` under n not in seen.
func (b *builder) javaClassLiterals(id string, n *ts.Node, skip, seen map[string]bool, src []byte) {
	if n.Kind() == "class_literal" {
		if n.NamedChildCount() > 0 {
			before := len(b.res.TypeRefs)
			b.javaTypeRefs(id, n.NamedChild(0), skip, true, src)
			// Drop what the declaration already references.
			kept := b.res.TypeRefs[:before]
			for _, r := range b.res.TypeRefs[before:] {
				if !seen[r.Name] {
					seen[r.Name] = true
					kept = append(kept, r)
				}
			}
			b.res.TypeRefs = kept
		}
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		b.javaClassLiterals(id, n.NamedChild(i), skip, seen, src)
	}
}

// resolveJavaChains binds the calls javaChain encoded, as upstream does.
func resolveJavaChains(results []Result, owner, idFile map[string]string, methods, supers map[string][]string, edgedSuper map[string]bool, edges [][]model.Edge) []model.Edge {
	type chainCall struct {
		c    callText
		file model.File
	}
	var chains []chainCall
	for i := range results {
		r := &results[i]
		if !strings.HasSuffix(r.File, ".java") {
			continue
		}
		for _, n := range r.Calls {
			if strings.HasPrefix(r.CallText[n.Recv], "#") {
				chains = append(chains, chainCall{r.call(n), model.FileOf(r.File)})
			}
		}
	}
	if len(chains) == 0 {
		return nil
	}

	rets := map[string]string{}    // method id -> declared return type
	types := map[string][]string{} // type name -> its definitions
	for _, r := range results {
		for _, s := range r.JavaRets {
			if id, t, _ := strings.Cut(s, " "); id != "" {
				rets[id], types[t] = t, nil
			}
		}
	}
	for _, ch := range chains {
		root, _, _ := strings.Cut(ch.c.Recv[1:], ".")
		types[root] = nil
	}
	nodeFile := map[string]string{} // candidate type node -> its file
	label := map[string]string{}
	for _, r := range results {
		for _, n := range r.Nodes {
			if _, ok := types[n.Label]; ok && n.Label != "" && n.SourceFile != "" && n.FileType == "code" {
				nodeFile[n.ID], label[n.ID] = n.SourceFile, n.Label
			}
		}
	}
	for _, p := range edges {
		for _, e := range p {
			if e.Relation != model.RelContains {
				continue
			}
			if l, ok := label[e.Target]; ok && !contains(types[l], e.Target) {
				types[l] = append(types[l], e.Target)
			}
		}
	}
	for _, r := range results {
		for _, s := range r.JavaRets {
			if t, ok := strings.CutPrefix(s, " "); ok {
				if _, ok := types[t]; ok {
					nodeFile["#"+r.File] = r.File
					types[t] = append(types[t], "#"+r.File)
				}
			}
		}
	}
	// A name defined twice is the near file's.
	typeNamed := func(name, near string) string {
		defs := types[name]
		if len(defs) > 1 {
			defs = keep(defs, func(id string) bool { return nodeFile[id] == idFile[near] })
		}
		// A "#" def has no node to bind.
		if len(defs) == 1 && !strings.HasPrefix(defs[0], "#") {
			return defs[0]
		}
		return ""
	}
	jvmBases := func(t string) ([]string, bool) {
		if edgedSuper[t] {
			return nil, false
		}
		for _, b := range supers[t] {
			if b == t || langfamily.Of(idFile[b]) != "jvm" {
				return nil, false
			}
		}
		return supers[t], true
	}
	// Exactly one declaration across the bases.
	methodOn := func(t, name string) string {
		var hits []string
		seen := map[string]bool{}
		frontier := []string{t}
		for len(frontier) > 0 {
			nid := frontier[len(frontier)-1]
			frontier = frontier[:len(frontier)-1]
			if seen[nid] {
				continue
			}
			seen[nid] = true
			if declared := methods[nid+"\x00"+name]; len(declared) > 0 {
				for _, id := range declared {
					if !contains(hits, id) {
						hits = append(hits, id)
					}
				}
				continue
			}
			bases, ok := jvmBases(nid)
			if !ok {
				return ""
			}
			frontier = append(frontier, bases...)
		}
		if len(hits) == 1 {
			return hits[0]
		}
		return ""
	}
	chainType := func(enc, caller string) string {
		links := strings.Split(enc, ".")
		t := owner[caller]
		switch links[0] {
		case "", "this":
		case "super":
			bases, ok := jvmBases(t)
			if t = ""; ok && len(bases) == 1 {
				t = bases[0]
			}
		default:
			t = typeNamed(links[0], caller)
		}
		for _, call := range links[1:] {
			if t == "" {
				return ""
			}
			m := methodOn(t, call)
			t = typeNamed(rets[m], m)
		}
		return t
	}

	type found struct {
		ch  *chainCall
		tgt string
	}
	var hits []found
	pairs := map[[2]string]bool{} // caller, target -> already linked
	for i := range chains {
		ch := &chains[i]
		t := chainType(ch.c.Recv[1:], ch.c.CallerID)
		if t == "" {
			continue
		}
		if m := methodOn(t, ch.c.Callee); m != "" && m != ch.c.CallerID {
			hits = append(hits, found{ch, m})
			pairs[[2]string{ch.c.CallerID, m}] = false
		}
	}
	if len(hits) == 0 {
		return nil
	}
	callers := map[string]bool{}
	for _, h := range hits {
		callers[h.ch.c.CallerID] = true
	}
	for _, p := range edges {
		for _, e := range p {
			if !callers[e.Source] {
				continue
			}
			if _, ok := pairs[[2]string{e.Source, e.Target}]; ok {
				pairs[[2]string{e.Source, e.Target}] = true
			}
		}
	}
	var out []model.Edge
	for _, h := range hits {
		k := [2]string{h.ch.c.CallerID, h.tgt}
		if pairs[k] {
			continue
		}
		pairs[k] = true
		out = append(out, model.Edge{
			Source: k[0], Target: k[1], Relation: model.RelCalls,
			Confidence: model.ConfInferred, ConfidenceScore: 0.85,
			SourceFile: h.ch.file, SourceLocation: h.ch.c.Loc,
		})
	}
	return out
}
