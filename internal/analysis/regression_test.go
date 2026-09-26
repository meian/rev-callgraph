package analysis

import (
	"runtime"
	"testing"
)

func regressionEdge(t *testing.T, result *Result, caller, callee string) *Edge {
	t.Helper()
	for i := range result.Edges {
		if result.Edges[i].Caller == caller && result.Edges[i].Callee == callee {
			return &result.Edges[i]
		}
	}
	t.Fatalf("missing edge %s -> %s: %+v", caller, callee, result.Edges)
	return nil
}

func TestRegressionConstantOverflow(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod": "module example.com/p\ngo 1.24\n",
		"p.go":   "package p\nfunc Target(v uint8){}\nfunc Caller(){Target(300)}\nfunc Above(){Caller()}\n",
	}, "example.com/p.Target", Options{})
	edge := regressionEdge(t, result, "example.com/p.Caller", "example.com/p.Target")
	if edge.Compatibility.Status != Incompatible {
		t.Fatalf("out-of-range constant must be incompatible: %+v", edge.Compatibility)
	}
	if len(result.Root.Callers) != 1 || len(result.Root.Callers[0].Callers) != 0 {
		t.Fatalf("incompatible caller must stop upward traversal: %+v", result.Root)
	}
}

func TestRegressionPointerMethodDoesNotSatisfyValueInterface(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod": "module example.com/p\ngo 1.24\n",
		"p.go":   "package p\ntype I interface{ M() }\ntype T struct{}\nfunc (*T) M(){}\nfunc Target(I){}\nfunc Caller(){var value T; Target(value)}\n",
	}, "example.com/p.Target", Options{})
	edge := regressionEdge(t, result, "example.com/p.Caller", "example.com/p.Target")
	if edge.Compatibility.Status != Incompatible {
		t.Fatalf("value T lacks the pointer receiver method: %+v", edge.Compatibility)
	}
}

func TestRegressionImportAliasShadowedByLocal(t *testing.T) {
	result := runFixture(t, map[string]string{
		"lib/go.mod": "module example.com/lib\ngo 1.24\n",
		"lib/lib.go": "package lib\nfunc Target(){}\n",
		"app/go.mod": "module example.com/app\ngo 1.24\nrequire example.com/lib v1.2.0\n",
		"app/app.go": "package app\nimport lib \"example.com/lib\"\ntype Local struct{}\nfunc (Local) Target(){}\nfunc Caller(){lib := Local{}; lib.Target()}\n",
	}, "example.com/lib.Target", Options{})
	for _, edge := range result.Edges {
		if edge.Caller == "example.com/app.Caller" {
			t.Fatalf("local variable must shadow the import alias: %+v", edge)
		}
	}
}

func TestRegressionExternalStdlibUsesTargetContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test requires a host where syscall.ForkExec exists")
	}
	result := runFixture(t, map[string]string{
		"go.mod": "module example.com/p\ngo 1.24\n",
		"p.go":   "package p\nimport \"syscall\"\nfunc Caller(){syscall.ForkExec(\"\", nil, nil)}\n",
	}, "syscall.ForkExec", Options{Build: BuildContext{GOOS: "windows", GOARCH: "amd64"}})
	edge := regressionEdge(t, result, "example.com/p.Caller", "syscall.ForkExec")
	if edge.Resolution.Status != Unknown {
		t.Fatalf("syscall.ForkExec is unavailable in windows/amd64: %+v", edge.Resolution)
	}
}

func TestRegressionLazyAliasAndEmbeddedFieldResolution(t *testing.T) {
	t.Run("alias-method", func(t *testing.T) {
		result := runFixture(t, map[string]string{
			"go.mod":    "module example.com/p\ngo 1.24\n",
			"type.go":   "package p\ntype T struct{}\ntype Alias = T\n",
			"method.go": "package p\nfunc (T) Method(){}\n",
			"caller.go": "package p\nfunc Caller(v Alias){v.Method()}\n",
		}, "example.com/p.T#Method", Options{})
		edge := regressionEdge(t, result, "example.com/p.Caller", "example.com/p.T#Method")
		if edge.Compatibility.Status != Compatible {
			t.Fatalf("alias method call: %+v", edge.Compatibility)
		}
	})
	t.Run("embedded-field", func(t *testing.T) {
		result := runFixture(t, map[string]string{
			"go.mod":    "module example.com/p\ngo 1.24\n",
			"types.go":  "package p\ntype Inner struct{}\ntype Outer struct{ Field Inner }\n",
			"method.go": "package p\nfunc (Inner) Method(){}\n",
			"caller.go": "package p\nfunc Caller(v Outer){v.Field.Method()}\n",
		}, "example.com/p.Inner#Method", Options{})
		edge := regressionEdge(t, result, "example.com/p.Caller", "example.com/p.Inner#Method")
		if edge.Compatibility.Status != Compatible {
			t.Fatalf("field method call: %+v", edge.Compatibility)
		}
	})
}

func TestRegressionUnknownCompatibilityContinues(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod": "module example.com/p\ngo 1.24\n",
		"p.go":   "package p\nfunc Target(int){}\nfunc B(){Target(missingValue)}\nfunc A(){B()}\n",
	}, "example.com/p.Target", Options{})
	edge := regressionEdge(t, result, "example.com/p.B", "example.com/p.Target")
	if edge.Compatibility.Status != CompatibilityUnknown {
		t.Fatalf("unresolved argument should remain unknown: %+v", edge.Compatibility)
	}
	regressionEdge(t, result, "example.com/p.A", "example.com/p.B")
}

func TestRegressionMajorImportPathBoundary(t *testing.T) {
	result := runFixture(t, map[string]string{
		"v1/go.mod":  "module example.com/lib\ngo 1.24\n",
		"v1/lib.go":  "package lib\nfunc Target(){}\n",
		"v2/go.mod":  "module example.com/lib/v2\ngo 1.24\n",
		"v2/lib.go":  "package lib\nfunc Target(){}\n",
		"app/go.mod": "module example.com/app\ngo 1.24\nrequire (\nexample.com/lib v1.2.0\nexample.com/lib/v2 v2.3.0\n)\n",
		"app/app.go": "package app\nimport (v1 \"example.com/lib\"; v2 \"example.com/lib/v2\")\nfunc Old(){v1.Target()}\nfunc New(){v2.Target()}\n",
	}, "example.com/lib/v2.Target", Options{})
	regressionEdge(t, result, "example.com/app.New", "example.com/lib/v2.Target")
	for _, edge := range result.Edges {
		if edge.Caller == "example.com/app.Old" {
			t.Fatalf("v1 caller crossed v2 path boundary: %+v", edge)
		}
	}
}

func TestRegressionCgoHeaderExternalBoundary(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod":  "module example.com/p\ngo 1.24\n",
		"local.h": "int from_header(int);\n",
		"p.go":    "package p\n/*\n#include \"local.h\"\n*/\nimport \"C\"\nfunc Caller(){C.from_header(1)}\nfunc Above(){Caller()}\n",
	}, "C.from_header", Options{Build: BuildContext{CgoSet: true, Cgo: true}})
	edge := regressionEdge(t, result, "example.com/p.Caller", "C.from_header")
	if edge.Resolution.Status != External || edge.Resolution.Kind != "cgo-header" || edge.Compatibility.Status != Compatible {
		t.Fatalf("declared header symbol: %+v", edge)
	}
	if len(result.Root.Callers) != 1 || len(result.Root.Callers[0].Callers) != 0 {
		t.Fatalf("external target must end traversal: %+v", result.Root)
	}
}

func TestRegressionVersionedPackageName(t *testing.T) {
	result := runFixture(t, map[string]string{
		"lib/go.mod": "module example.com/lib/v2\ngo 1.24\n",
		"lib/lib.go": "package lib\nfunc Target(){}\n",
		"app/go.mod": "module example.com/app\ngo 1.24\nrequire example.com/lib/v2 v2.1.0\n",
		"app/app.go": "package app\nimport \"example.com/lib/v2\"\nfunc Caller(){lib.Target()}\n",
	}, "example.com/lib/v2.Target", Options{})
	regressionEdge(t, result, "example.com/app.Caller", "example.com/lib/v2.Target")
}
func TestRegressionVendorExcluded(t *testing.T) {
	result := runFixture(t, map[string]string{"go.mod": "module example.com/p\ngo 1.24\n", "p.go": "package p\nfunc Target(){}", "vendor/v/v.go": "package p\nfunc Wrong(){Target()}"}, "example.com/p.Target", Options{})
	if len(result.Edges) != 0 || result.Stats.DiscoveredSources != 1 {
		t.Fatalf("vendor entered analysis: %+v", result)
	}
}

func TestRegressionEmbeddedMethodAndInterfaceDeclaration(t *testing.T) {
	files := map[string]string{"go.mod": "module example.com/p\ngo 1.24\n", "type.go": "package p\ntype Inner struct{}\ntype Outer struct { Inner }\ntype Named struct{ Inner Inner }\ntype I interface{ Method() }", "method.go": "package p\nfunc(Inner)Method(){}", "caller.go": "package p\nfunc Good(v Outer){v.Method()}\nfunc Wrong(v Named){v.Method()}\nfunc Abstract(v I){v.Method()}"}
	result := runFixture(t, files, "example.com/p.Inner#Method", Options{})
	regressionEdge(t, result, "example.com/p.Good", "example.com/p.Inner#Method")
	for _, edge := range result.Edges {
		if edge.Caller == "example.com/p.Wrong" {
			t.Fatal("named field promoted as embedded")
		}
	}
	result = runFixture(t, files, "example.com/p.I#Method", Options{})
	edge := regressionEdge(t, result, "example.com/p.Abstract", "example.com/p.I#Method")
	if edge.Compatibility.Status != Compatible {
		t.Fatalf("declared interface method: %+v", edge)
	}
}

func TestRegressionReplacementAndResultTypes(t *testing.T) {
	result := runFixture(t, map[string]string{
		"lib/go.mod": "module example.com/fork\ngo 1.24\n", "lib/lib.go": "package lib\nfunc Target()int{return 1}",
		"app/go.mod": "module example.com/app\ngo 1.24\nrequire example.com/original v1.2.0\nreplace example.com/original => ../lib\n",
		"app/app.go": "package app\nimport \"example.com/original\"\nfunc Caller(){var x string; x=lib.Target();_ = x}",
	}, "example.com/fork.Target", Options{})
	edge := regressionEdge(t, result, "example.com/app.Caller", "example.com/fork.Target")
	if edge.Compatibility.Status != Incompatible {
		t.Fatalf("result type change: %+v", edge)
	}
}
func TestRegressionMissingMethodAndVariadicChange(t *testing.T) {
	result := runFixture(t, map[string]string{"go.mod": "module example.com/p\ngo 1.24\n", "p.go": "package p\ntype T struct{}\nfunc Target(v int){}\nfunc Caller(v T){v.Gone()}\nfunc Spread(xs []int){Target(xs...)}"}, "example.com/p.T#Gone", Options{})
	if edge := regressionEdge(t, result, "example.com/p.Caller", "example.com/p.T#Gone"); edge.Compatibility.Status != Incompatible {
		t.Fatalf("missing method: %+v", edge)
	}
	result = runFixture(t, map[string]string{"go.mod": "module example.com/p\ngo 1.24\n", "p.go": "package p\nfunc Target(v int){}\nfunc Spread(xs []int){Target(xs...)}"}, "example.com/p.Target", Options{})
	if edge := regressionEdge(t, result, "example.com/p.Spread", "example.com/p.Target"); edge.Compatibility.Status != Incompatible {
		t.Fatalf("variadic change: %+v", edge)
	}
}
