package extract

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dobbo-ca/graphify-go/internal/detect"
	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// manifestEcosystems maps a recognized package-manifest basename to its ecosystem
// tag, mirroring upstream graphify.manifest_ingest.PACKAGE_MANIFEST_NAMES (apm is
// omitted: this port carries no YAML dependency, and upstream treats apm
// identically to the others once parsed).
var manifestEcosystems = map[string]string{
	"pyproject.toml": "python",
	"go.mod":         "go",
	"pom.xml":        "maven",
}

// manifestInfo is one parsed manifest: its canonical package id/name, the
// manifest path (posix-slashed) the node is anchored to, and its dependency names.
type manifestInfo struct {
	pkgID string
	name  string
	rel   string
	deps  []string
}

// IntrospectManifests parses every package manifest under root (pyproject.toml,
// go.mod, pom.xml) into a canonical file_type "package" node per module — keyed by
// package NAME so a package referenced from several manifests collapses to one hub
// node — plus "depends_on" edges module->dependency. It mirrors upstream
// graphify/manifest_ingest.py; unlike upstream it also emits a stub package node
// for external dependencies, because this port's graph builder drops edges whose
// target node is absent. A dependency that is itself a module in the corpus gets
// only the edge (its real node already exists), never a stub that would clobber
// the real node's source_file. A malformed or nameless manifest is skipped, never
// fatal, so the default-on pass cannot break a build.
func IntrospectManifests(root string) (Result, error) {
	files, err := detect.CollectManifests(root)
	if err != nil {
		return Result{}, err
	}
	sort.Strings(files)

	// Pass 1: parse each manifest, keyed by canonical package id (last one wins
	// on a name collision — deterministic given the sorted file order).
	modules := map[string]manifestInfo{}
	for _, rel := range files {
		eco, ok := manifestEcosystems[filepath.Base(rel)]
		if !ok {
			continue
		}
		name, deps := parseManifest(eco, filepath.Join(root, rel))
		if name == "" {
			continue
		}
		id := idutil.MakeID("pkg", name)
		modules[id] = manifestInfo{pkgID: id, name: name, rel: filepath.ToSlash(rel), deps: deps}
	}

	ids := make([]string, 0, len(modules))
	for id := range modules {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var res Result
	// Module nodes first, then dependency stubs, for a deterministic node order.
	for _, id := range ids {
		m := modules[id]
		res.Nodes = append(res.Nodes, model.Node{
			ID: m.pkgID, Label: m.name, FileType: "package",
			SourceFile: m.rel, SourceLocation: "L1",
		})
	}
	stubbed := map[string]bool{} // external dep ids already emitted as a stub node
	for _, id := range ids {
		m := modules[id]
		depNames := make([]string, 0, len(m.deps))
		seen := map[string]bool{}
		for _, dep := range m.deps {
			if dep == "" {
				continue
			}
			depID := idutil.MakeID("pkg", dep)
			if depID == m.pkgID || seen[depID] {
				continue
			}
			seen[depID] = true
			depNames = append(depNames, dep)
		}
		sort.Strings(depNames)
		for _, dep := range depNames {
			depID := idutil.MakeID("pkg", dep)
			if _, isModule := modules[depID]; !isModule && !stubbed[depID] {
				stubbed[depID] = true
				res.Nodes = append(res.Nodes, model.Node{
					ID: depID, Label: dep, FileType: "package",
					SourceFile: m.rel, SourceLocation: "L1",
				})
			}
			res.Edges = append(res.Edges, model.Edge{
				Source: m.pkgID, Target: depID, Relation: "depends_on",
				Confidence: "EXTRACTED", Weight: 1.0,
				SourceFile: m.rel, SourceLocation: "L1",
			})
		}
	}
	return res, nil
}

// parseManifest dispatches a manifest to its ecosystem parser and returns the
// package name and dependency names. A read/parse failure yields an empty name so
// the caller skips the manifest (mirrors upstream's swallow-and-continue).
func parseManifest(eco, path string) (name string, deps []string) {
	switch eco {
	case "go":
		text, err := os.ReadFile(path)
		if err != nil {
			return "", nil
		}
		return parseGoMod(string(text))
	case "python":
		data, err := loadTOML(path)
		if err != nil {
			return "", nil
		}
		return parsePyproject(data)
	case "maven":
		text, err := os.ReadFile(path)
		if err != nil {
			return "", nil
		}
		return parsePom(string(text))
	}
	return "", nil
}

var (
	goModuleRe    = regexp.MustCompile(`^module\s+(\S+)`)
	goRequireOpen = regexp.MustCompile(`^require\s*\(`)
	goBlockDepRe  = regexp.MustCompile(`^(\S+)\s+v\S+`)
	goReqLineRe   = regexp.MustCompile(`^require\s+(\S+)\s+v\S+`)
)

// parseGoMod reads the module path and require directives from a go.mod, handling
// both the single-line `require x v1` form and the parenthesized block. Mirrors
// upstream manifest_ingest._parse_gomod.
func parseGoMod(text string) (string, []string) {
	var name string
	var deps []string
	inBlock := false
	for _, line := range strings.Split(text, "\n") {
		s := strings.TrimSpace(line)
		if name == "" {
			if m := goModuleRe.FindStringSubmatch(s); m != nil {
				name = m[1]
				continue
			}
		}
		if goRequireOpen.MatchString(s) {
			inBlock = true
			continue
		}
		if inBlock {
			if strings.HasPrefix(s, ")") {
				inBlock = false
				continue
			}
			if m := goBlockDepRe.FindStringSubmatch(s); m != nil {
				deps = append(deps, m[1])
			}
		} else if m := goReqLineRe.FindStringSubmatch(s); m != nil {
			deps = append(deps, m[1])
		}
	}
	return name, deps
}

var pep508SplitRe = regexp.MustCompile(`[\s<>=!~;\[\(]`)

// pep508Name reduces a PEP 508 requirement to its bare distribution name:
// `requests>=2.0` -> `requests`, `pkg[extra]==1; python_version<'3.9'` -> `pkg`.
func pep508Name(spec string) string {
	s := strings.TrimSpace(spec)
	if loc := pep508SplitRe.FindStringIndex(s); loc != nil {
		return s[:loc[0]]
	}
	return s
}

// parsePyproject reads the package name and dependency names from a decoded
// pyproject.toml, covering both PEP 621 ([project]) and Poetry
// ([tool.poetry]). Mirrors upstream manifest_ingest._parse_pyproject.
func parsePyproject(data map[string]any) (string, []string) {
	proj, _ := data["project"].(map[string]any)
	var poetry map[string]any
	if tool, ok := data["tool"].(map[string]any); ok {
		poetry, _ = tool["poetry"].(map[string]any)
	}
	name, _ := proj["name"].(string)
	if name == "" {
		name, _ = poetry["name"].(string)
	}
	if name == "" {
		return "", nil
	}
	var deps []string
	if arr, ok := proj["dependencies"].([]any); ok {
		for _, d := range arr {
			if s, ok := d.(string); ok {
				if n := pep508Name(s); n != "" {
					deps = append(deps, n)
				}
			}
		}
	}
	if pdeps, ok := poetry["dependencies"].(map[string]any); ok {
		for dep := range pdeps {
			if strings.ToLower(dep) != "python" {
				deps = append(deps, dep)
			}
		}
	}
	return name, deps
}

// xmlElem is a generic XML element used to walk a pom.xml namespace-agnostically:
// encoding/xml matches on the local name, so the Maven default xmlns needs no
// stripping (upstream regex-strips it before ElementTree parses).
type xmlElem struct {
	XMLName  xml.Name
	Chardata string    `xml:",chardata"`
	Children []xmlElem `xml:",any"`
}

func (e xmlElem) childText(local string) string {
	for _, c := range e.Children {
		if c.XMLName.Local == local {
			return strings.TrimSpace(c.Chardata)
		}
	}
	return ""
}

// parsePom reads the project coordinate and its dependency coordinates from a
// pom.xml. Names are `groupId:artifactId` (or bare artifactId when no groupId).
// Every <dependencies>/<dependency> at any depth is collected (so
// dependencyManagement and profile dependencies count), mirroring upstream's
// `.//dependencies/dependency`. Mirrors upstream manifest_ingest._parse_pom.
func parsePom(text string) (string, []string) {
	var root xmlElem
	if err := xml.Unmarshal([]byte(text), &root); err != nil {
		return "", nil
	}
	aid := root.childText("artifactId")
	if aid == "" {
		return "", nil
	}
	name := aid
	if gid := root.childText("groupId"); gid != "" {
		name = gid + ":" + aid
	}
	var deps []string
	var collect func(e xmlElem)
	collect = func(e xmlElem) {
		if e.XMLName.Local == "dependencies" {
			for _, c := range e.Children {
				if c.XMLName.Local != "dependency" {
					continue
				}
				da := c.childText("artifactId")
				if da == "" {
					continue
				}
				if dg := c.childText("groupId"); dg != "" {
					deps = append(deps, dg+":"+da)
				} else {
					deps = append(deps, da)
				}
			}
		}
		for _, c := range e.Children {
			collect(c)
		}
	}
	collect(root)
	return name, deps
}
