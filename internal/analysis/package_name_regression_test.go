package analysis

import (
	"errors"
	"go/build"
	"path/filepath"
	"testing"
)

func TestStandardLibraryVersionedImportUsesDeclaredPackageName(t *testing.T) {
	for _, tc := range []struct {
		name, imported, call string
	}{
		{
			name:     "default",
			imported: `"math/rand/v2"`,
			call:     "rand.Int()",
		},
		{
			name:     "explicit alias",
			imported: `rng "math/rand/v2"`,
			call:     "rng.Int()",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runFixture(t, map[string]string{
				"go.mod": "module example.com/app\ngo 1.24\n",
				"app.go": "package app\nimport " + tc.imported + "\nfunc Caller(){" + tc.call + "}\n",
			}, "math/rand/v2.Int", Options{})
			edge := regressionEdge(t, result, "example.com/app.Caller", "math/rand/v2.Int")
			if edge.Resolution.Status != External || edge.Compatibility.Status != Compatible {
				t.Fatalf("versioned standard import: %+v", edge)
			}
		})
	}
}

func TestExternalDeclarationVersionedImportUsesDeclaredPackageName(t *testing.T) {
	root := fixture(t, map[string]string{
		"src/depx/v2/dep.go": "package dep\ntype T struct{}\n",
		"src/api/api.go":     "package api\nimport \"depx/v2\"\nfunc Target(v dep.T){}\n",
	})
	original := build.Default
	build.Default.GOROOT = root
	t.Cleanup(func() { build.Default = original })
	fn, err := AnalyzeExternalFunction("api", "Target", "", BuildContext{})
	if err != nil || fn == nil {
		t.Fatalf("external function: %+v %v", fn, err)
	}
	if len(fn.Params) != 1 || fn.Params[0].Type.Name != "depx/v2.T" {
		t.Fatalf("external parameter type: %+v", fn.Params)
	}
}

func TestStandardImportNameUsesTargetBuildContext(t *testing.T) {
	root := fixture(t, map[string]string{
		"src/contextpkg/host.go":   "//go:build !selected\npackage host\n",
		"src/contextpkg/target.go": "//go:build selected\npackage target\n",
		"caller.go":                "package caller\nimport \"contextpkg\"\nfunc Caller(){target.Run()}\n",
	})
	original := build.Default
	build.Default.GOROOT = root
	t.Cleanup(func() { build.Default = original })
	model, err := AnalyzeSource(Source{
		Path:    filepath.Join(root, "caller.go"),
		Package: "example.com/caller",
		Build:   BuildContext{Tags: []string{"selected"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.Imports["target"] != "contextpkg" {
		t.Fatalf("selected package name: %+v", model.Imports)
	}
}

func TestImportNamePriorityAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name, spec, path, want string
		names                  map[string]string
	}{
		{
			name:  "standard declaration",
			spec:  `"math/rand/v2"`,
			path:  "math/rand/v2",
			want:  "rand",
			names: nil,
		},
		{
			name:  "workspace declaration",
			spec:  `"math/rand/v2"`,
			path:  "math/rand/v2",
			want:  "workspace",
			names: map[string]string{"math/rand/v2": "workspace"},
		},
		{
			name:  "explicit alias",
			spec:  `alias "math/rand/v2"`,
			path:  "math/rand/v2",
			want:  "alias",
			names: map[string]string{"math/rand/v2": "workspace"},
		},
		{
			name:  "unavailable declaration",
			spec:  `"missing/v2"`,
			path:  "missing/v2",
			want:  "v2",
			names: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t, map[string]string{"caller.go": "package caller\nimport " + tc.spec + "\n"})
			model, err := AnalyzeSource(Source{
				Path:         filepath.Join(root, "caller.go"),
				Package:      "example.com/caller",
				PackageNames: tc.names,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(model.Imports) != 1 || model.Imports[tc.want] != tc.path {
				t.Fatalf("import name priority: %+v", model.Imports)
			}
		})
	}
}

func TestStandardPackageNameCacheCachesSuccessfulAndEmptyResults(t *testing.T) {
	calls := map[string]int{}
	cache := &standardPackageNameCache{
		names: map[string]string{},
		importPackage: func(path string) (*build.Package, error) {
			calls[path]++
			if path == "found" {
				return &build.Package{
					Name:   "declared",
					Goroot: true,
				}, nil
			}
			return nil, errors.New("missing package")
		},
	}
	for _, path := range []string{"found", "found", "missing", "missing"} {
		standardPackageName(build.Context{}, path, cache)
	}
	if calls["found"] != 1 || calls["missing"] != 1 {
		t.Fatalf("Import calls: %+v", calls)
	}
	if cache.names["found"] != "declared" || cache.names["missing"] != "" {
		t.Fatalf("cached names: %+v", cache.names)
	}
}

func TestStandardPackageNameCacheIsSharedBySourceAndExternalAnalysis(t *testing.T) {
	root := fixture(t, map[string]string{
		"src/depx/v2/dep.go": "package dep\ntype T struct{}\n",
		"src/api/api.go":     "package api\nimport \"depx/v2\"\nfunc Target(dep.T){}\n",
		"caller.go":          "package caller\nimport \"depx/v2\"\nfunc Caller(dep.T){}\n",
	})
	original := build.Default
	build.Default.GOROOT = root
	t.Cleanup(func() { build.Default = original })
	buildConfig := sourceBuildConfig(BuildContext{})
	calls := 0
	cache := newStandardPackageNameCache(buildConfig)
	cache.importPackage = func(path string) (*build.Package, error) {
		calls++
		return buildConfig.Import(path, "", 0)
	}
	if _, err := analyzeSource(Source{
		Path:    filepath.Join(root, "caller.go"),
		Package: "example.com/caller",
	}, cache); err != nil {
		t.Fatal(err)
	}
	fn, err := analyzeExternalFunction("api", "Target", "", BuildContext{}, cache)
	if err != nil {
		t.Fatal(err)
	}
	if fn == nil || len(fn.Params) != 1 || fn.Params[0].Type.Name != "depx/v2.T" {
		t.Fatalf("external function: %+v", fn)
	}
	if calls != 1 {
		t.Fatalf("shared standard import calls = %d, want 1", calls)
	}
}
