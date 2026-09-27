package analysis

import (
	"go/build"
	"path/filepath"
	"testing"
)

func TestStandardLibraryVersionedImportUsesDeclaredPackageName(t *testing.T) {
	for _, tc := range []struct {
		name, imported, call string
	}{
		{"default", `"math/rand/v2"`, "rand.Int()"},
		{"explicit alias", `rng "math/rand/v2"`, "rng.Int()"},
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
	model, err := AnalyzeSource(Source{Path: filepath.Join(root, "caller.go"), Package: "example.com/caller", Build: BuildContext{Tags: []string{"selected"}}})
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
		{"standard declaration", `"math/rand/v2"`, "math/rand/v2", "rand", nil},
		{"workspace declaration", `"math/rand/v2"`, "math/rand/v2", "workspace", map[string]string{"math/rand/v2": "workspace"}},
		{"explicit alias", `alias "math/rand/v2"`, "math/rand/v2", "alias", map[string]string{"math/rand/v2": "workspace"}},
		{"unavailable declaration", `"missing/v2"`, "missing/v2", "v2", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t, map[string]string{"caller.go": "package caller\nimport " + tc.spec + "\n"})
			model, err := AnalyzeSource(Source{Path: filepath.Join(root, "caller.go"), Package: "example.com/caller", PackageNames: tc.names})
			if err != nil {
				t.Fatal(err)
			}
			if len(model.Imports) != 1 || model.Imports[tc.want] != tc.path {
				t.Fatalf("import name priority: %+v", model.Imports)
			}
		})
	}
}
