package extract

import (
	"strings"

	tsjava "github.com/dobbo-ca/gositter/grammars/java"
	ts "github.com/dobbo-ca/graphify-go/internal/ts"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
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
	return b.res
}

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
			b.callRecv(callerID, name.Utf8Text(src), recvText(c, name, src), line(c))
		}
		return true
	})
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
