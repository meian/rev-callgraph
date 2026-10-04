package analysis

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSwappedPackageNamesAndAliases(t *testing.T) {
	for _, tc := range []struct {
		name, imports, calls string
	}{
		{
			name:    "swapped declarations",
			imports: `"example.com/m/foo"; "example.com/m/bar"`,
			calls:   "bar.A(); foo.B()",
		},
		{
			name:    "explicit aliases",
			imports: `left "example.com/m/foo"; right "example.com/m/bar"`,
			calls:   "left.A(); right.B()",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{
				"go.mod":   "module example.com/m\ngo 1.24\n",
				"foo/a.go": "package bar\nfunc A() {}\n",
				"bar/b.go": "package foo\nfunc B() {}\n",
				"app.go":   "package m\nimport (" + tc.imports + ")\nfunc Run() {" + tc.calls + "}\n",
			}
			for _, target := range []string{"example.com/m/foo.A", "example.com/m/bar.B"} {
				result := runFixture(t, files, target, Options{})
				edge := regressionEdge(t, result, "example.com/m.Run", target)
				if len(result.Edges) != 1 || edge.Resolution.Status != Resolved || edge.Compatibility.Status != Compatible {
					t.Fatalf("swapped import: %+v", result.Edges)
				}
			}
		})
	}
	result := runFixture(t, map[string]string{
		"go.mod":   "module example.com/m\ngo 1.24\n",
		"one/a.go": "package shared\nfunc A() {}\n",
		"two/b.go": "package shared\nfunc B() {}\n",
		"app.go":   "package m\nimport (a \"example.com/m/one\"; b \"example.com/m/two\")\nfunc Run() { a.A(); b.B() }\n",
	}, "example.com/m/two.B", Options{})
	regressionEdge(t, result, "example.com/m.Run", "example.com/m/two.B")
}

func TestPackageNameDoesNotExpandImportSearch(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod":     "module example.com/m\ngo 1.24\n",
		"foo/a.go":   "package bar\nfunc A() {}\n",
		"bar/b.go":   "package foo\nfunc OnlySibling() {}\n",
		"parent.go":  "package bar\nfunc OnlyParent() {}\n",
		"app/app.go": "package app\nimport \"example.com/m/foo\"\nfunc Run() { bar.OnlySibling(); bar.OnlyParent() }\n",
	}, "example.com/m/foo.OnlySibling", Options{})
	edge := regressionEdge(t, result, "example.com/m/app.Run", "example.com/m/foo.OnlySibling")
	if edge.Resolution.Kind != "missing-symbol" || edge.Compatibility.Status != Incompatible {
		t.Fatalf("must not search sibling by declaration name: %+v", edge)
	}
}

func collisionFiles() map[string]string {
	return map[string]string{
		"go.mod": "module example.com/m\ngo 1.24\n",
		"foo/regular.go": `package foo
func Target() {}
`,
		"foo/external_test.go": `package foo_test
import "example.com/m/foo"
func SameName() { foo.Target() }
func Caller() { SameName(); Other(); var x Box; x.Value.Run(); var v Item; Accept(v); Own(v); SameName(1) }
func Top() { Caller() }
`,
		"foo/other_test.go": `package foo_test
import regular "example.com/m/foo_test"
type Item int
type Box struct { Value Item }
func (Item) Run() {}
func Other() { SameName() }
func Accept(regular.Item) {}
func Own(Item) {}
func Return() regular.Item { return regular.Item("") }
func BadReturn() { var x Item; x = Return(); _ = x }
func ImportRegular() { regular.SameName(1) }
`,
		"foo_test/regular.go": `package foo_test
import "example.com/m/foo"
type Item string
func (Item) Run(n int) {}
func SameName(n int) { foo.Target() }
func Caller() { SameName(1) }
func Top() { Caller() }
`,
		"app/app.go": `package app
import "example.com/m/foo_test"
func Imported() { foo_test.SameName(1) }
`,
	}
}

func newScopeEngine(t *testing.T, files map[string]string) *engine {
	t.Helper()
	w, err := Discover(context.Background(), Options{
		Dir:       fixture(t, files),
		SymbolSet: Test,
	})
	if err != nil {
		t.Fatal(err)
	}
	locator, err := NewLocator(w)
	if err != nil {
		t.Fatal(err)
	}
	return &engine{
		ctx:             context.Background(),
		workspace:       w,
		locator:         locator,
		models:          map[string]SourceModel{},
		functions:       map[string]Function{},
		types:           map[string]Type{},
		callerCache:     map[string][]Edge{},
		definitionCache: map[string]bool{},
		externalCache:   map[string]*Function{},
	}
}

func TestCollidingDefinitionsResolutionAndCache(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "forward", true: "reverse"}[reverse], func(t *testing.T) {
			e := newScopeEngine(t, collisionFiles())
			sources := slices.Clone(e.workspace.Sources)
			if reverse {
				slices.Reverse(sources)
			}
			for _, source := range sources {
				if err := e.load(source); err != nil {
					t.Fatal(err)
				}
			}
			for _, pkg := range e.workspace.Packages {
				if pkg.Path != "example.com/m/foo_test" {
					continue
				}
				scope := packageScope(pkg.Path, pkg.ID)
				fn := e.functions[scope+".SameName"]
				wantParams := 1
				if pkg.ExternalTest {
					wantParams = 0
				}
				if fn.PackageID != pkg.ID || len(fn.Params) != wantParams {
					t.Fatalf("overwritten definition: %+v", fn)
				}
				first, err := e.callersFor(scope + ".SameName")
				if err != nil {
					t.Fatal(err)
				}
				hits := e.stats.CacheHits
				second, err := e.callersFor(scope + ".SameName")
				if err != nil || !reflect.DeepEqual(first, second) || e.stats.CacheHits != hits+1 {
					t.Fatalf("cache changed result: %+v / %+v / %v", first, second, err)
				}
				if len(first) != 3 {
					t.Fatalf("callers for external=%t: %+v", pkg.ExternalTest, first)
				}
				incompatible := 0
				for _, edge := range first {
					if pkg.ExternalTest && !strings.HasPrefix(edge.Caller, scope+".") {
						t.Fatalf("external test leaked into imported definitions: %+v", edge)
					}
					if edge.Compatibility.Status == Incompatible {
						incompatible++
					}
				}
				if (incompatible == 1) != pkg.ExternalTest {
					t.Fatalf("mixed signatures: %+v", first)
				}
				if pkg.ExternalTest {
					for _, target := range []string{"Item#Run", "Other", "Own", "Accept"} {
						edges, err := e.callersFor(scope + "." + target)
						if err != nil || len(edges) != 1 {
							t.Fatalf("cross-file %s: %+v, %v", target, edges, err)
						}
						want := Compatible
						if target == "Accept" {
							want = Incompatible
						}
						if edges[0].Compatibility.Status != want {
							t.Fatalf("type identity %s: %+v", target, edges)
						}
					}
				}
			}
		})
	}
}

func TestCollidingTargetAndResultDisplay(t *testing.T) {
	dir := fixture(t, collisionFiles())
	options := Options{
		Dir:       dir,
		SymbolSet: Test,
	}
	for _, target := range []string{"SameName", "Item#Run"} {
		_, err := Analyze(context.Background(), "example.com/m/foo_test."+target, options)
		if err == nil || !strings.Contains(err.Error(), "ambiguous target") || !strings.Contains(err.Error(), "external-test") || strings.ContainsRune(err.Error(), 0) {
			t.Fatalf("ambiguous target must fail clearly: %v", err)
		}
	}
	result, err := Analyze(context.Background(), "example.com/m/foo.Target", options)
	if err != nil {
		t.Fatal(err)
	}
	var roots []string
	for _, node := range result.Root.Callers {
		roots = append(roots, node.Name)
	}
	if len(roots) != 2 || roots[0] == roots[1] || !strings.Contains(strings.Join(roots, " "), "external-test") {
		t.Fatalf("colliding callers merged: %+v", roots)
	}
	// 同じ公開名の Caller と Top を別々にたどり、別 package の経路を混ぜない。
	for _, node := range result.Root.Callers {
		external := strings.Contains(node.Name, "external-test")
		for _, caller := range node.Callers {
			if strings.Contains(caller.Name, ".Caller [") {
				if caller.Edge.Compatibility.Status == Incompatible {
					if len(caller.Callers) != 0 {
						t.Fatal("incompatible branch must stop")
					}
					continue
				}
				if strings.Contains(caller.Name, "external-test") != external || len(caller.Callers) != 1 {
					t.Fatalf("mixed traversal: %+v", caller)
				}
				if strings.Contains(caller.Callers[0].Name, "external-test") != external {
					t.Fatalf("mixed ancestor: %+v", caller.Callers[0])
				}
			}
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), `\u0000`) {
		t.Fatalf("internal identity leaked to output: %s, %v", encoded, err)
	}
	options.SymbolSet = Runtime
	result, err = Analyze(context.Background(), "example.com/m/foo_test.SameName", options)
	if err != nil || result.Root.Name != "example.com/m/foo_test.SameName" || len(result.Root.Callers) != 2 {
		t.Fatalf("runtime target must remain unambiguous: %+v, %v", result, err)
	}
}

// TestCollidingTypeDiagnostics は同名の型を診断上で区別できることを確認する。
func TestCollidingTypeDiagnostics(t *testing.T) {
	dir := fixture(t, collisionFiles())
	for _, test := range []struct {
		target, caller, kind string
	}{
		{"Accept", "Caller", "argument-type"},
		{"Return", "BadReturn", "result-type"},
	} {
		result, err := Analyze(context.Background(), "example.com/m/foo_test."+test.target, Options{
			Dir:       dir,
			SymbolSet: Test,
		})
		if err != nil {
			t.Fatal(err)
		}
		edge := regressionEdge(t, result, "example.com/m/foo_test."+test.caller, "example.com/m/foo_test."+test.target)
		if len(edge.Compatibility.Issues) != 1 || edge.Compatibility.Issues[0].Kind != test.kind {
			t.Fatalf("issues = %+v", edge.Compatibility.Issues)
		}
		message := edge.Compatibility.Issues[0].Message
		for _, want := range []string{
			"example.com/m/foo_test.Item [" + filepath.ToSlash(filepath.Join(dir, "foo")) + "; external-test]",
			"example.com/m/foo_test.Item [" + filepath.ToSlash(filepath.Join(dir, "foo_test")) + "; package]",
		} {
			if !strings.Contains(message, want) {
				t.Fatalf("diagnostic does not identify both types: %q", message)
			}
		}
	}
}

func TestInternalTestOfPackageWithTestSuffix(t *testing.T) {
	dir := fixture(t, map[string]string{
		"go.mod":               "module example.com/m\ngo 1.24\n",
		"foo_test/regular.go":  "package foo_test\nfunc Target() {}\n",
		"foo_test/in_test.go":  "package foo_test\nfunc Internal() { Target() }\n",
		"foo_test/out_test.go": "package foo_test_test\nimport \"example.com/m/foo_test\"\nfunc External() { foo_test.Target() }\n",
	})
	result, err := Analyze(context.Background(), "example.com/m/foo_test.Target", Options{
		Dir:       dir,
		SymbolSet: Test,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edges) != 2 {
		t.Fatalf("internal/external test selection: %+v", result.Edges)
	}
	regressionEdge(t, result, "example.com/m/foo_test.Internal", "example.com/m/foo_test.Target")
	regressionEdge(t, result, "example.com/m/foo_test_test.External", "example.com/m/foo_test.Target")
	w, err := Discover(context.Background(), Options{
		Dir:       dir,
		SymbolSet: Test,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range w.Sources {
		if filepath.Base(source.Path) == "in_test.go" && source.ExternalTest {
			t.Fatal("internal test was classified as external")
		}
	}
}

func TestSamePublicNameAcrossPackagesIsNotCycle(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod":     "module example.com/m\ngo 1.24\n",
		"foo/foo.go": "package foo\nfunc Target() {}\n",
		"foo/external_test.go": `package foo_test
import regular "example.com/m/foo_test"
func SameName() { regular.SameName(1) }
func Top() { SameName() }
`,
		"foo_test/regular.go": `package foo_test
import "example.com/m/foo"
func SameName(int) { foo.Target() }
`,
	}, "example.com/m/foo.Target", Options{SymbolSet: Test})
	if len(result.Root.Callers) != 1 || len(result.Root.Callers[0].Callers) != 1 {
		t.Fatalf("missing identity-specific path: %+v", result.Edges)
	}
	external := result.Root.Callers[0].Callers[0]
	if external.Cycle || len(external.Callers) != 1 || external.Callers[0].Name != "example.com/m/foo_test.Top" {
		t.Fatalf("public ID collision treated as cycle: %+v", external)
	}
}

func TestImportCannotReachExternalTestOnlyPackage(t *testing.T) {
	files := map[string]string{
		"go.mod":               "module example.com/m\ngo 1.24\n",
		"foo/foo.go":           "package foo\nfunc Target() {}\n",
		"foo/external_test.go": "package foo_test\nfunc Local() {}\nfunc Caller() { Local() }\n",
		"app/app.go":           "package app\nimport ext \"example.com/m/foo_test\"\nfunc Invalid() { ext.Local() }\n",
	}
	result := runFixture(t, files, "example.com/m/foo_test.Local", Options{SymbolSet: Test})
	if len(result.Edges) != 1 || result.Edges[0].Caller != "example.com/m/foo_test.Caller" {
		t.Fatalf("normal import reached external tests: %+v", result.Edges)
	}
}

func TestCollidingImportUsesRegularDeclarationName(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod":               "module example.com/m\ngo 1.24\n",
		"foo/external_test.go": "package foo_test\nfunc OnlyExternal() {}\n",
		"foo_test/regular.go":  "package declared\nfunc Target() {}\n",
		"app.go":               "package m\nimport \"example.com/m/foo_test\"\nfunc Caller() { declared.Target() }\n",
	}, "example.com/m/foo_test.Target", Options{SymbolSet: Test})
	edge := regressionEdge(t, result, "example.com/m.Caller", "example.com/m/foo_test.Target")
	if edge.Resolution.Status != Resolved || edge.Compatibility.Status != Compatible {
		t.Fatalf("regular declaration name lost: %+v", edge)
	}
}

// TestMutualReplaceKeepsDistinctImportTargets は入れ替わる replace の参照先を固定する。
func TestMutualReplaceKeepsDistinctImportTargets(t *testing.T) {
	files := map[string]string{
		"app/go.mod": `module example.com/app

go 1.24

require (
 example.com/a v1.0.0
 example.com/b v1.0.0
)
replace example.com/a => ../b
replace example.com/b => ../a
`,
		"app/app.go": `package app
import (
 fromA "example.com/a"
 fromB "example.com/b"
)
func Caller() { fromA.Target("text"); fromB.Target(1) }
`,
		"a/go.mod": "module example.com/a\ngo 1.24\n",
		"a/a.go":   "package a\nfunc Target(int) {}\n",
		"b/go.mod": "module example.com/b\ngo 1.24\n",
		"b/b.go":   "package b\nfunc Target(string) {}\n",
	}
	root := fixture(t, files)
	for range 12 {
		for _, target := range []string{"example.com/a.Target", "example.com/b.Target"} {
			result, err := Analyze(context.Background(), target, Options{Dir: root})
			if err != nil {
				t.Fatal(err)
			}
			edge := regressionEdge(t, result, "example.com/app.Caller", target)
			if len(result.Edges) != 1 || edge.Resolution.Status != Resolved || edge.Compatibility.Status != Compatible {
				t.Fatalf("replace mapping for %s: %+v", target, result.Edges)
			}
		}
	}
}

// TestDistinctReplacementsWithSameModulePath は置換先の module path が重なる場合を確認する。
func TestDistinctReplacementsWithSameModulePath(t *testing.T) {
	files := map[string]string{
		"app/go.mod": `module example.com/app

go 1.24

require (
 example.com/a v1.0.0
 example.com/b v1.0.0
)
replace example.com/a => ../one
replace example.com/b => ../two
`,
		"app/app.go": `package app
import (
 one "example.com/a"
 two "example.com/b"
)
func Caller() { one.A(); two.B() }
`,
		"one/go.mod": "module example.com/lib\ngo 1.24\n",
		"one/lib.go": "package lib\nfunc A() {}\n",
		"two/go.mod": "module example.com/lib\ngo 1.24\n",
		"two/lib.go": "package lib\nfunc B() {}\n",
	}
	root := fixture(t, files)
	for _, target := range []string{"example.com/lib.A", "example.com/lib.B"} {
		result, err := Analyze(context.Background(), target, Options{Dir: root})
		if err != nil {
			t.Fatal(err)
		}
		edge := regressionEdge(t, result, "example.com/app.Caller", target)
		if len(result.Edges) != 1 || edge.Resolution.Status != Resolved || edge.Compatibility.Status != Compatible {
			t.Fatalf("replace mapping for %s: %+v", target, result.Edges)
		}
	}
}
