package analysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type retryLocator struct{ sources []Source }

func (l retryLocator) Definitions(_, _ string) []Source { return l.sources }
func (l retryLocator) Callers(_, _ string) []Source     { return nil }

func TestMissingSymbolPrefersCallerModule(t *testing.T) {
	result := runFixture(t, map[string]string{
		"a/go.mod": "module example.com/p\ngo 1.24\n",
		"a/a.go":   "package p\nfunc First() { Missing() }\n",
		"b/go.mod": "module example.com/p\ngo 1.24\n",
		"b/b.go":   "package p\nfunc Second() { Missing() }\n",
	}, "example.com/p.Missing", Options{})
	if len(result.Edges) != 2 {
		t.Fatalf("edges = %+v, want callers from both owning modules", result.Edges)
	}
	for _, edge := range result.Edges {
		if edge.Resolution.Status != Unknown || edge.Resolution.Kind != "missing-symbol" {
			t.Errorf("resolution for %s = %+v, want unknown/missing-symbol", edge.Caller, edge.Resolution)
		}
	}
}

func TestDefinitionLoadFailureDoesNotCacheCompletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "target.go")
	source := Source{
		Path:        path,
		Package:     "example.com/p",
		PackageName: "p",
	}
	readyPath := filepath.Join(filepath.Dir(path), "ready.go")
	if err := os.WriteFile(readyPath, []byte("package p\nfunc Ready() { Target() }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e := &engine{
		ctx: context.Background(),
		locator: retryLocator{sources: []Source{
			{
				Path:    readyPath,
				Package: source.Package,
			},
			source,
		}},
		models:          map[string]SourceModel{},
		functions:       map[string]Function{},
		types:           map[string]Type{},
		definitionCache: map[string]bool{},
	}
	if err := e.definitions(source.Package, "Target"); err == nil {
		t.Fatal("missing file should fail")
	}
	if e.definitionCache[source.Package+"\x00Target"] || len(e.models) != 1 {
		t.Fatal("failed lookup must preserve only completed file state")
	}
	if err := os.WriteFile(path, []byte("package p\nfunc Target() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := e.definitions(source.Package, "Target"); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.functions["example.com/p.Target"]; !ok {
		t.Fatal("retry did not load definition")
	}
	if e.stats.LocatorLookups != 2 || e.stats.AnalyzedSources != 2 || e.stats.CacheHits != 1 {
		t.Fatalf("unexpected retry stats: %+v", e.stats)
	}
	if err := e.definitions(source.Package, "Target"); err != nil {
		t.Fatal(err)
	}
	if e.stats.LocatorLookups != 2 || e.stats.CacheHits != 2 {
		t.Fatalf("completed lookup was not reused: %+v", e.stats)
	}
}

func TestDefinitionCacheSeparatesPackagesWithSamePath(t *testing.T) {
	root := fixture(t, map[string]string{
		"a/go.mod": "module example.com/p\ngo 1.24\n",
		"a/p.go":   "package p\nfunc Target() {}\n",
		"b/go.mod": "module example.com/p\ngo 1.24\n",
		"b/p.go":   "package p\nfunc Target() {}\n",
	})
	w, err := Discover(context.Background(), Options{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	locator, err := NewLocator(w)
	if err != nil {
		t.Fatal(err)
	}
	e := &engine{
		ctx:             context.Background(),
		locator:         locator,
		models:          map[string]SourceModel{},
		functions:       map[string]Function{},
		types:           map[string]Type{},
		definitionCache: map[string]bool{},
	}
	missing := filepath.Join(root, "b", "p.go")
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	if err := e.definitions("example.com/p", "Target"); err == nil {
		t.Fatal("second package load should fail")
	}
	first := w.Sources[0].PackageID + "\x00Target"
	second := w.Sources[1].PackageID + "\x00Target"
	if !e.definitionCache[first] || e.definitionCache[second] || e.stats.AnalyzedSources != 1 {
		t.Fatalf("partial completion = %+v, stats = %+v", e.definitionCache, e.stats)
	}
	if err := os.WriteFile(missing, []byte("package p\nfunc Target() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := e.definitions("example.com/p", "Target"); err != nil {
		t.Fatal(err)
	}
	if !e.definitionCache[second] || e.stats.AnalyzedSources != 2 || e.stats.LocatorLookups != 3 {
		t.Fatalf("retry completion = %+v, stats = %+v", e.definitionCache, e.stats)
	}
	if err := e.definitions("example.com/p", "Missing"); err != nil {
		t.Fatal(err)
	}
	if !e.definitionCache[w.Sources[0].PackageID+"\x00Missing"] || !e.definitionCache[w.Sources[1].PackageID+"\x00Missing"] || e.stats.AnalyzedSources != 2 {
		t.Fatalf("missing symbol completion = %+v, stats = %+v", e.definitionCache, e.stats)
	}
}

func TestMissingSymbolChecksAllPackageSeries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		series   []string
		wantID   string
		wantKind string
	}{
		{
			name:     "compatible last",
			series:   []string{"v0", "v1"},
			wantID:   "example.com/p.Missing",
			wantKind: "missing-symbol",
		},
		{
			name:     "compatible first",
			series:   []string{"v1", "v0"},
			wantID:   "example.com/p.Missing",
			wantKind: "missing-symbol",
		},
		{
			name:     "all incompatible",
			series:   []string{"v0", "v0"},
			wantKind: "different-module-series",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &Workspace{Modules: []Module{{
				Path:     "example.com/app",
				Requires: map[string]string{"example.com/p": "v1.0.0"},
			}}}
			for _, series := range tc.series {
				w.Packages = append(w.Packages, Package{
					Path:   "example.com/p",
					Module: len(w.Modules),
				})
				w.Modules = append(w.Modules, Module{
					Path:   "example.com/p",
					Series: series,
				})
			}
			e := &engine{
				workspace:       w,
				locator:         retryLocator{},
				definitionCache: map[string]bool{},
			}
			id, resolution, fn, err := e.resolve(Function{Module: 0}, Call{
				Package: "example.com/p",
				Name:    "Missing",
			})
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := Unknown
			if tc.wantKind == "different-module-series" {
				wantStatus = External
			}
			if id != tc.wantID || resolution.Status != wantStatus || resolution.Kind != tc.wantKind || fn != nil {
				t.Fatalf("resolution = %q %+v %+v", id, resolution, fn)
			}
		})
	}
}

func TestInterfaceMethodKeepsPackageIdentity(t *testing.T) {
	dir := fixture(t, map[string]string{
		"go.mod":   "module example.com/p\ngo 1.24\n",
		"types.go": "package p\ntype Contract interface { Run() }\n",
	})
	workspace, err := Discover(context.Background(), Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	locator, err := NewLocator(workspace)
	if err != nil {
		t.Fatal(err)
	}
	e := &engine{
		ctx:             context.Background(),
		workspace:       workspace,
		locator:         locator,
		models:          map[string]SourceModel{},
		functions:       map[string]Function{},
		types:           map[string]Type{},
		definitionCache: map[string]bool{},
	}
	method, ok, err := e.method("example.com/p.Contract", "Run", map[string]bool{})
	if err != nil || !ok {
		t.Fatalf("method = %+v, %t, %v", method, ok, err)
	}
	if method.PackageID != workspace.Sources[0].PackageID {
		t.Fatalf("package identity = %q, want %q", method.PackageID, workspace.Sources[0].PackageID)
	}
}
