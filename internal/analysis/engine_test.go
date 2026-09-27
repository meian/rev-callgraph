package analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func runFixture(t *testing.T, files map[string]string, target string, opts Options) *Result {
	t.Helper()
	opts.Dir = fixture(t, files)
	r, err := Analyze(context.Background(), target, opts)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestSeriesAndBrokenCallBoundary(t *testing.T) {
	for _, versions := range [][2]string{{"v1.2.0", "v1.7.1"}, {"v0.1.0", "v0.9.0"}, {"v1.2.1", "v1.2.2"}, {"v1.2.0", "v1.2.0"}} {
		t.Run(versions[0]+versions[1], func(t *testing.T) {
			r := runFixture(t, map[string]string{
				"lib/go.mod": "module example.com/lib\ngo 1.24\n", "lib/lib.go": "package lib\nfunc Target(s string) {}\n",
				"app/go.mod":     "module example.com/app\ngo 1.24\nrequire example.com/lib " + versions[0] + "\n",
				"app/app.go":     "package app\nimport \"example.com/lib\"\nfunc B(){lib.Target()}\nfunc A(){B()}\n",
				"other/go.mod":   "module example.com/other\ngo 1.24\nrequire example.com/lib " + versions[1] + "\n",
				"other/other.go": "package other\nimport \"example.com/lib\"\nfunc Good(){lib.Target(\"ok\")}\n",
			}, "example.com/lib.Target", Options{})
			if len(r.Root.Callers) != 2 {
				t.Fatalf("callers: %+v", r.Root.Callers)
			}
			for _, n := range r.Root.Callers {
				if n.Name == "example.com/app.B" {
					if n.Edge.Compatibility.Status != Incompatible || len(n.Callers) != 0 {
						t.Fatalf("boundary: %+v", n)
					}
				}
			}
		})
	}
}
func TestMajorAmbiguityDoesNotCross(t *testing.T) {
	r := runFixture(t, map[string]string{
		"lib/go.mod": "module example.com/lib\ngo 1.24\n", "lib/lib.go": "package lib\nfunc Target(){}",
		"a/go.mod": "module example.com/a\ngo 1.24\nrequire example.com/lib v0.9.0\n", "a/a.go": "package a\nimport \"example.com/lib\"\nfunc A(){lib.Target()}",
		"b/go.mod": "module example.com/b\ngo 1.24\nrequire example.com/lib v1.0.0\n", "b/b.go": "package b\nimport \"example.com/lib\"\nfunc B(){lib.Target()}",
	}, "example.com/lib.Target", Options{})
	if len(r.Root.Callers) != 0 {
		t.Fatal("ambiguous major must not silently connect")
	}
}
func TestSymbolSetsAndCycles(t *testing.T) {
	files := map[string]string{"go.mod": "module example.com/p\ngo 1.24\n", "p.go": "package p\nfunc Target(){B()}\nfunc B(){Target()}", "p_test.go": "package p\nfunc TestCaller(){Target()}", "external_test.go": "package p_test\nimport \"example.com/p\"\nfunc External(){p.Target()}"}
	r := runFixture(t, files, "example.com/p.Target", Options{})
	if len(r.Root.Callers) != 1 || !r.Root.Callers[0].Callers[0].Cycle {
		t.Fatalf("runtime cycle: %+v", r.Root)
	}
	r = runFixture(t, files, "example.com/p.Target", Options{SymbolSet: Test})
	if len(r.Root.Callers) != 3 {
		t.Fatalf("test callers: %+v", r.Root.Callers)
	}
}
func TestLazyLargePackage(t *testing.T) {
	files := map[string]string{"go.mod": "module example.com/p\ngo 1.24\n", "target.go": "package p\nfunc Target(v Foo){}", "caller.go": "package p\nfunc Caller(v Foo){Target(v)}", "type.go": "package p\ntype Foo string"}
	for i := 0; i < 300; i++ {
		files[fmt.Sprintf("unused%03d.go", i)] = fmt.Sprintf("package p\nfunc Unused%d(){}", i)
	}
	r := runFixture(t, files, "example.com/p.Target", Options{})
	if len(r.Root.Callers) != 1 {
		t.Fatal("missing caller")
	}
	if r.Stats.AnalyzedSources > 3 {
		t.Fatalf("eager analysis: %+v", r.Stats)
	}
}
func TestMissingDefinitionAndUnknownContinue(t *testing.T) {
	r := runFixture(t, map[string]string{"go.mod": "module example.com/p\ngo 1.24\n", "p.go": "package p\nfunc B(){Gone()}\nfunc A(){B()}"}, "example.com/p.Gone", Options{})
	if len(r.Root.Callers) != 1 || r.Root.Callers[0].Edge.Compatibility.Status != Incompatible || len(r.Root.Callers[0].Callers) != 0 {
		t.Fatalf("missing target: %+v", r.Root)
	}
}
func TestDepthAndPolicy(t *testing.T) {
	dir := fixture(t, map[string]string{"go.mod": "module example.com/p\ngo 1.24\n", "p.go": "package p\nfunc Target(x int){}\nfunc B(){Target()}\nfunc A(){B()}"})
	r, err := AnalyzeWithPolicy(context.Background(), "example.com/p.Target", Options{Dir: dir}, TraversalPolicy{ContinueIncompatible: true})
	if err != nil || len(r.Root.Callers[0].Callers) != 1 {
		t.Fatalf("policy: %v %+v", err, r)
	}
	r, err = AnalyzeWithPolicy(context.Background(), "example.com/p.Target", Options{Dir: dir, MaxDepth: 1}, TraversalPolicy{ContinueIncompatible: true})
	if err != nil || len(r.Root.Callers[0].Callers) != 0 {
		t.Fatalf("depth: %v %+v", err, r)
	}
}
func TestCompatibilityChecks(t *testing.T) {
	tests := []struct {
		name, definition, caller string
		status                   CompatibilityStatus
		kind                     string
	}{
		{"argument-added", "func Target(x int){}", "Target()", Incompatible, "argument-count"},
		{"argument-removed", "func Target(){}", "Target(1)", Incompatible, "argument-count"},
		{"argument-type", "func Target(x int){}", "Target(\"x\")", Incompatible, "argument-type"},
		{"variadic-compatible", "func Target(x ...int){}", "Target(1,2)", Compatible, ""},
		{"result-added", "func Target()(int,error){return 0,nil}", "x := Target(); _ = x", Incompatible, "result-count"},
		{"result-removed", "func Target(){}", "x := Target(); _ = x", Incompatible, "result-count"},
		{"discard-result-valid", "func Target()int{return 0}", "Target()", Compatible, ""},
		{"unknown-type", "func Target(x int){}", "Target(undefined)", CompatibilityUnknown, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := runFixture(t, map[string]string{"go.mod": "module example.com/p\ngo 1.24\n", "p.go": "package p\n" + tc.definition + "\nfunc Caller(){" + tc.caller + "}"}, "example.com/p.Target", Options{})
			if len(r.Edges) != 1 {
				t.Fatalf("edges: %+v", r.Edges)
			}
			c := r.Edges[0].Compatibility
			if c.Status != tc.status {
				t.Fatalf("got %+v want %s", c, tc.status)
			}
			if tc.kind != "" {
				found := false
				for _, i := range c.Issues {
					found = found || i.Kind == tc.kind
				}
				if !found {
					t.Fatalf("missing %s: %+v", tc.kind, c)
				}
			}
		})
	}
}
func TestParseTarget(t *testing.T) {
	for _, v := range []string{"example.com/p.F", "example.com/p.T#M"} {
		p, e := ParseTarget(v)
		if e != nil || p.ID() != v {
			t.Fatalf("%q: %+v %v", v, p, e)
		}
	}
	for _, v := range []string{"", "foo", "a.", "a.#M", "a.T#"} {
		if _, e := ParseTarget(v); e == nil {
			t.Fatalf("accepted %q", v)
		}
	}
}
func TestCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Analyze(ctx, "example.com/p.F", Options{})
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatal(err)
	}
}
