package analysis

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeDiscoveryFixture(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverModulesBuildContextAndSymbolSet(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFixture(t, root, "consumer/go.mod", "module example.com/consumer\n\ngo 1.23\nrequire example.com/common v1.3.0\nreplace example.com/common => ../common\n")
	writeDiscoveryFixture(t, root, "consumer/main.go", "package consumer\nfunc Use() {}\n")
	writeDiscoveryFixture(t, root, "common/go.mod", "module example.com/common\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "common/a.go", "package common\nfunc A() {}\n")
	writeDiscoveryFixture(t, root, "common/linux_amd64.go", "//go:build linux && amd64\n\npackage common\nfunc Linux() {}\n")
	writeDiscoveryFixture(t, root, "common/only_darwin.go", "package common\nfunc Darwin() {}\n")
	writeDiscoveryFixture(t, root, "common/a_test.go", "package common\nfunc TestA() {}\n")
	writeDiscoveryFixture(t, root, "common/external_test.go", "package common_test\nfunc TestExternal() {}\n")
	writeDiscoveryFixture(t, root, "common/nested/go.mod", "module example.com/nested/v2\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "common/nested/nested.go", "package nested\nfunc Nested() {}\n")
	writeDiscoveryFixture(t, root, "yaml/go.mod", "module gopkg.in/yaml.v2\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "yaml/yaml.go", "package yaml\nfunc Parse() {}\n")
	build := BuildContext{
		GOOS:   "linux",
		GOARCH: "amd64",
	}
	w, err := Discover(context.Background(), Options{
		Dir:       root,
		SymbolSet: Runtime,
		Build:     build,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Modules) != 4 {
		t.Fatalf("modules: %d", len(w.Modules))
	}
	modules := make(map[string]Module)
	for _, module := range w.Modules {
		modules[module.Path] = module
	}
	if got := modules["example.com/common"].Series; got != "v1" {
		t.Errorf("common series = %q", got)
	}
	if got := modules["example.com/nested/v2"].Series; got != "v2" {
		t.Errorf("nested series = %q", got)
	}
	if got := modules["gopkg.in/yaml.v2"].Series; got != "v2" {
		t.Errorf("gopkg.in series = %q", got)
	}
	if got := modules["example.com/consumer"].Replaces["example.com/common"]; len(got) != 1 || got[0].NewPath != "../common" || got[0].OldVersion != "" {
		t.Errorf("replacement = %q", got)
	}
	if got := sourceNames(w.Sources); !equalStrings(got, []string{"a.go", "linux_amd64.go", "main.go", "nested.go", "yaml.go"}) {
		t.Errorf("runtime sources = %v", got)
	}
	w, err = Discover(context.Background(), Options{
		Dir:       root,
		SymbolSet: Test,
		Build:     build,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceNames(w.Sources); !equalStrings(got, []string{"a.go", "a_test.go", "external_test.go", "linux_amd64.go", "main.go", "nested.go", "yaml.go"}) {
		t.Errorf("test sources = %v", got)
	}
	for _, source := range w.Sources {
		if filepath.Base(source.Path) == "external_test.go" && source.Package != "example.com/common_test" {
			t.Errorf("external test package = %q", source.Package)
		}
	}
	type packageKey struct {
		path   string
		module int
	}
	packages := make(map[packageKey]Package)
	commonModule := -1
	for _, pkg := range w.Packages {
		key := packageKey{
			path:   pkg.Path,
			module: pkg.Module,
		}
		if _, exists := packages[key]; exists {
			t.Fatalf("duplicate package %v", key)
		}
		packages[key] = pkg
		if pkg.Path == "example.com/common" {
			commonModule = pkg.Module
		}
		for _, source := range pkg.Sources {
			if source.Package != pkg.Path || source.Module != pkg.Module {
				t.Errorf("package %q contains foreign source %+v", pkg.Path, source)
			}
		}
	}
	if commonModule < 0 {
		t.Fatal("common package not found")
	}
	regularKey := packageKey{
		path:   "example.com/common",
		module: commonModule,
	}
	externalKey := packageKey{
		path:   "example.com/common_test",
		module: commonModule,
	}
	if got := len(packages[regularKey].Sources); got != 3 {
		t.Errorf("internal test package sources = %d, want 3", got)
	}
	if got := len(packages[externalKey].Sources); got != 1 {
		t.Errorf("external test package sources = %d, want 1", got)
	}
}

func TestDiscoverSeparatesModulesWithSamePackagePath(t *testing.T) {
	root := fixture(t, map[string]string{
		"a/go.mod": "module example.com/p\ngo 1.24\n",
		"a/p.go":   "package p\nfunc Same() {}\n",
		"b/go.mod": "module example.com/p\ngo 1.24\n",
		"b/p.go":   "package p\nfunc Same() {}\n",
	})
	w, err := Discover(context.Background(), Options{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Packages) != 2 || len(w.Modules) != 2 {
		t.Fatalf("packages/modules = %d/%d, want 2/2", len(w.Packages), len(w.Modules))
	}
	seenModules := make(map[int]bool)
	for _, pkg := range w.Packages {
		if pkg.Module < 0 || pkg.Module >= len(w.Modules) || seenModules[pkg.Module] {
			t.Fatalf("invalid or duplicate module: %+v", pkg)
		}
		seenModules[pkg.Module] = true
		if pkg.Path != "example.com/p" || len(pkg.Sources) != 1 || pkg.Dir != w.Modules[pkg.Module].Dir {
			t.Fatalf("incorrect package ownership: %+v", pkg)
		}
		source := pkg.Sources[0]
		if source.Module != pkg.Module || source.Path != filepath.Join(pkg.Dir, "p.go") {
			t.Fatalf("package contains source from another module: %+v", pkg)
		}
	}
}

func TestDiscoverSeparatesExternalTestPathCollision(t *testing.T) {
	root := fixture(t, map[string]string{
		"go.mod":               "module example.com/p\ngo 1.24\n",
		"bar/regular.go":       "package bar\nfunc Regular() {}\n",
		"bar/internal_test.go": "package bar\nfunc Internal() {}\n",
		"bar/external_test.go": "package bar_test\nfunc SameName() {}\n",
		"bar_test/regular.go":  "package bar_test\nfunc SameName() {}\n",
	})
	w, err := Discover(context.Background(), Options{
		Dir:       root,
		SymbolSet: Test,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Packages) != 3 {
		t.Fatalf("packages = %d, want 3 separate Go packages", len(w.Packages))
	}
	var colliding int
	ids := map[string]bool{}
	for _, pkg := range w.Packages {
		if pkg.ID == "" || ids[pkg.ID] {
			t.Fatalf("package identity = %q, want a unique non-empty value", pkg.ID)
		}
		ids[pkg.ID] = true
		if pkg.Path == "example.com/p/bar" {
			if len(pkg.Sources) != 2 || pkg.ExternalTest || pkg.Dir != filepath.Join(root, "bar") {
				t.Fatalf("regular and internal test files must share a package: %+v", pkg)
			}
			continue
		}
		if pkg.Path != "example.com/p/bar_test" || len(pkg.Sources) != 1 {
			t.Fatalf("unexpected package: %+v", pkg)
		}
		source := pkg.Sources[0]
		wantExternal := filepath.Base(source.Path) == "external_test.go"
		if pkg.ExternalTest != wantExternal || pkg.Dir != filepath.Dir(source.Path) || source.PackageID != pkg.ID {
			t.Errorf("package ownership metadata = %+v", pkg)
		}
		colliding++
	}
	if colliding != 2 {
		t.Fatalf("colliding paths = %d, want 2 distinct packages", colliding)
	}
}

func TestDiscoverAmbiguousModuleSeries(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFixture(t, root, "common/go.mod", "module example.com/common\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "a/go.mod", "module example.com/a\n\ngo 1.23\nrequire example.com/common v0.9.0\n")
	writeDiscoveryFixture(t, root, "b/go.mod", "module example.com/b\n\ngo 1.23\nrequire example.com/common v1.1.0\n")
	w, err := Discover(context.Background(), Options{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range w.Modules {
		if module.Path == "example.com/common" && module.Series != "" {
			t.Errorf("ambiguous series = %q", module.Series)
		}
	}
}

func TestDiscoverSeriesThroughLocalReplace(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFixture(t, root, "fork/go.mod", "module example.com/fork\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "consumer/go.mod", "module example.com/consumer\n\ngo 1.23\nrequire example.com/common v0.8.0\nreplace example.com/common => ../fork\n")
	w, err := Discover(context.Background(), Options{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range w.Modules {
		if module.Path == "example.com/fork" && module.Series != "v0" {
			t.Errorf("replaced local module series = %q", module.Series)
		}
	}
}

func TestDiscoverVersionedReplaceSelection(t *testing.T) {
	root := t.TempDir()
	for _, module := range []string{"original", "old", "selected", "fallback"} {
		writeDiscoveryFixture(t, root, module+"/go.mod", "module example.com/"+module+"\n\ngo 1.23\n")
		writeDiscoveryFixture(t, root, module+"/lib.go", "package lib\nfunc Target() {}\n")
	}
	writeDiscoveryFixture(t, root, "consumer/go.mod", "module example.com/consumer\n\ngo 1.23\nrequire example.com/original v1.3.0\nreplace (\nexample.com/original v1.2.0 => ../old\nexample.com/original v1.3.0 => ../selected\nexample.com/original => ../fallback\n)\n")
	writeDiscoveryFixture(t, root, "consumer/main.go", "package consumer\nimport \"example.com/original\"\nfunc Caller() { lib.Target() }\n")
	w, err := Discover(context.Background(), Options{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	consumer := discoveryModule(t, w, "example.com/consumer")
	if got := consumer.Replaces["example.com/original"]; len(got) != 3 || got[0].OldVersion != "v1.2.0" || got[1].OldVersion != "v1.3.0" || got[2].OldVersion != "" {
		t.Fatalf("replacement directives = %+v", got)
	}
	if !requireTargetsModule(consumer, "example.com/original", discoveryModule(t, w, "example.com/selected")) {
		t.Error("selected version did not match")
	}
	for _, path := range []string{"example.com/original", "example.com/old", "example.com/fallback"} {
		if requireTargetsModule(consumer, "example.com/original", discoveryModule(t, w, path)) {
			t.Errorf("wrong replacement target: %s", path)
		}
	}
	if got := discoveryModule(t, w, "example.com/selected").Series; got != "v1" {
		t.Errorf("selected series = %q", got)
	}
	if got := discoveryModule(t, w, "example.com/old").Series; got != "" {
		t.Errorf("unselected series = %q", got)
	}
	for _, source := range w.Sources {
		if source.Package == "example.com/consumer" {
			if got := source.ImportPaths["example.com/original"]; got != "example.com/selected" {
				t.Errorf("import alias = %q", got)
			}
		}
	}
}

func TestDiscoverNonmatchingVersionedReplace(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFixture(t, root, "original/go.mod", "module example.com/original\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "fork/go.mod", "module example.com/fork\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "consumer/go.mod", "module example.com/consumer\n\ngo 1.23\nrequire example.com/original v1.3.0\nreplace example.com/original v1.2.0 => ../fork\n")
	w, err := Discover(context.Background(), Options{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	consumer := discoveryModule(t, w, "example.com/consumer")
	if !requireTargetsModule(consumer, "example.com/original", discoveryModule(t, w, "example.com/original")) {
		t.Error("nonmatching replacement hid original module")
	}
	if requireTargetsModule(consumer, "example.com/original", discoveryModule(t, w, "example.com/fork")) {
		t.Error("nonmatching replacement applied to fork")
	}
	if got := discoveryModule(t, w, "example.com/fork").Series; got != "" {
		t.Errorf("unselected fork series = %q", got)
	}
}

func TestDiscoverReplacementNewVersionSeries(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFixture(t, root, "fork/go.mod", "module example.com/fork\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "consumer/go.mod", "module example.com/consumer\n\ngo 1.23\nrequire example.com/original v1.3.0\nreplace example.com/original v1.3.0 => example.com/fork v1.7.0\n")
	w, err := Discover(context.Background(), Options{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	consumer := discoveryModule(t, w, "example.com/consumer")
	if got := consumer.Replaces["example.com/original"]; len(got) != 1 || got[0].NewVersion != "v1.7.0" {
		t.Fatalf("replacement new version = %+v", got)
	}
	if !requireTargetsModule(consumer, "example.com/original", discoveryModule(t, w, "example.com/fork")) {
		t.Error("versioned replacement did not resolve to fork")
	}
	if got := discoveryModule(t, w, "example.com/fork").Series; got != "v1" {
		t.Errorf("fork series = %q", got)
	}
}

func TestVersionedReplaceCallResolution(t *testing.T) {
	base := map[string]string{
		"original/go.mod": "module example.com/original\n\ngo 1.24\n",
		"original/lib.go": "package lib\nfunc Target() {}\n",
		"fork/go.mod":     "module example.com/fork\n\ngo 1.24\n",
		"fork/lib.go":     "package lib\nfunc Target() {}\n",
		"app/app.go":      "package app\nimport \"example.com/original\"\nfunc Caller() { lib.Target() }\n",
	}
	for _, test := range []struct {
		name, replace, target string
	}{
		{"nonmatching version", "replace example.com/original v1.2.0 => ../fork\n", "example.com/original.Target"},
		{"matching version", "replace example.com/original v1.3.0 => ../fork\n", "example.com/fork.Target"},
		{"specific over all", "replace (\nexample.com/original => ../original\nexample.com/original v1.3.0 => ../fork\n)\n", "example.com/fork.Target"},
		{"all when specific differs", "replace (\nexample.com/original => ../original\nexample.com/original v1.2.0 => ../fork\n)\n", "example.com/original.Target"},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := make(map[string]string, len(base)+1)
			for path, content := range base {
				files[path] = content
			}
			files["app/go.mod"] = "module example.com/app\n\ngo 1.24\nrequire example.com/original v1.3.0\n" + test.replace
			result := runFixture(t, files, test.target, Options{})
			regressionEdge(t, result, "example.com/app.Caller", test.target)
		})
	}
}

func discoveryModule(t *testing.T, w *Workspace, path string) Module {
	t.Helper()
	for _, module := range w.Modules {
		if module.Path == path {
			return module
		}
	}
	t.Fatalf("module %s not found", path)
	return Module{}
}

func TestDiscoverCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Discover(ctx, Options{Dir: t.TempDir()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestDiscoverExplicitBuildTags(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFixture(t, root, "go.mod", "module example.com/m\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "tagged.go", "//go:build custom\n\npackage m\n")
	writeDiscoveryFixture(t, root, "ordinary.go", "package m\n")
	w, err := Discover(context.Background(), Options{
		Dir:   root,
		Build: BuildContext{Tags: []string{"custom"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceNames(w.Sources); !equalStrings(got, []string{"ordinary.go", "tagged.go"}) {
		t.Errorf("tagged sources = %v", got)
	}
}

func TestDiscoverCgoContext(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFixture(t, root, "go.mod", "module example.com/m\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "ordinary.go", "package m\n")
	writeDiscoveryFixture(t, root, "cgo.go", "package m\n/* int f(void); */\nimport \"C\"\nfunc F() { C.f() }\n")
	without, err := Discover(context.Background(), Options{
		Dir: root,
		Build: BuildContext{
			CgoSet: true,
			Cgo:    false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceNames(without.Sources); !equalStrings(got, []string{"ordinary.go"}) {
		t.Errorf("cgo disabled sources = %v", got)
	}
	with, err := Discover(context.Background(), Options{
		Dir: root,
		Build: BuildContext{
			CgoSet: true,
			Cgo:    true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceNames(with.Sources); !equalStrings(got, []string{"cgo.go", "ordinary.go"}) {
		t.Errorf("cgo enabled sources = %v", got)
	}
}

func TestLocatorDefinitionsAndCallers(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFixture(t, root, "go.mod", "module example.com/m\n\ngo 1.23\n")
	writeDiscoveryFixture(t, root, "defs.go", "package m\ntype Thing struct{}\ntype (Grouped struct{})\nfunc (Thing) Run() {}\nfunc Target() {}\n")
	writeDiscoveryFixture(t, root, "calls.go", "package m\nfunc Use() { f := Target; f(); Thing{}.Run() }\n")
	w, err := Discover(context.Background(), Options{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	locator, err := NewLocator(w)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Thing", "Grouped", "Run", "Target"} {
		if got := locator.Definitions("example.com/m", name); len(got) != 1 || filepath.Base(got[0].Path) != "defs.go" {
			t.Errorf("definition %s = %v", name, got)
		}
	}
	if got := locator.Callers("example.com/m", "Target"); len(got) == 0 {
		t.Error("missing caller candidate")
	}
	if got := locator.Definitions("example.com/other", "Target"); len(got) != 0 {
		t.Errorf("unrelated package definitions = %v", got)
	}
}

func TestLocatorSeparatesCollidingPackageIdentities(t *testing.T) {
	root := fixture(t, map[string]string{
		"go.mod":               "module example.com/p\ngo 1.24\n",
		"bar/external_test.go": "package bar_test\nfunc SameName() {}\n",
		"bar_test/regular.go":  "package bar_test\nfunc SameName() {}\n",
	})
	w, err := Discover(context.Background(), Options{
		Dir:       root,
		SymbolSet: Test,
	})
	if err != nil {
		t.Fatal(err)
	}
	locator, err := NewLocator(w)
	if err != nil {
		t.Fatal(err)
	}
	index := locator.(*indexedLocator)
	path := "example.com/p/bar_test"
	if got := len(index.packageIDs[path]); got != 1 {
		t.Fatalf("importable package identities = %d, want 1", got)
	}
	for _, pkg := range w.Packages {
		if got := len(index.definitionsInPackage(pkg.ID, "SameName")); got != 1 {
			t.Errorf("definitions for %q = %d, want 1", pkg.ID, got)
		}
	}
	if got := locator.Definitions(path, "SameName"); len(got) != 1 || got[0].ExternalTest {
		t.Errorf("public package lookup = %+v, want only the regular package", got)
	}
}

func sourceNames(sources []Source) []string {
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, filepath.Base(source.Path))
	}
	sort.Strings(names)
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
