package analysis

import "testing"

func TestCrossFileFunctionValue(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod":    "module example.com/p\ngo 1.24\n",
		"target.go": "package p\nfunc Target(){}\n",
		"caller.go": "package p\nvar global = Target\nfunc Caller(){f := Target; f()}\nfunc VarCaller(){var f = Target; f()}\nfunc GlobalCaller(){global()}\nfunc Above(){Caller()}\n",
	}, "example.com/p.Target", Options{})
	if len(result.Root.Callers) != 3 {
		t.Fatalf("callers: %+v", result.Root.Callers)
	}
	seen := map[string]*Node{}
	for _, caller := range result.Root.Callers {
		seen[caller.Name] = caller
		if caller.Edge.Resolution.Status != Resolved || caller.Edge.Compatibility.Status != Compatible {
			t.Fatalf("call: %+v", caller.Edge)
		}
	}
	if seen["example.com/p.Caller"] == nil || seen["example.com/p.VarCaller"] == nil || seen["example.com/p.GlobalCaller"] == nil {
		t.Fatalf("missing callers: %+v", seen)
	}
	if len(seen["example.com/p.Caller"].Callers) != 1 || seen["example.com/p.Caller"].Callers[0].Name != "example.com/p.Above" {
		t.Fatalf("upper caller: %+v", seen["example.com/p.Caller"])
	}
}

func TestDotImportedFunctionValue(t *testing.T) {
	result := runFixture(t, map[string]string{
		"lib/go.mod": "module example.com/lib\ngo 1.24\n",
		"lib/lib.go": "package lib\nfunc Target(){}\n",
		"app/go.mod": "module example.com/app\ngo 1.24\nrequire example.com/lib v1.0.0\n",
		"app/app.go": "package app\nimport . \"example.com/lib\"\nfunc Caller(){f := Target; f()}\nfunc VarCaller(){var f = Target; f()}\nfunc Direct(){Target()}\n",
	}, "example.com/lib.Target", Options{})
	if len(result.Root.Callers) != 3 {
		t.Fatalf("dot import callers: %+v", result.Root.Callers)
	}
	seen := map[string]bool{}
	for _, caller := range result.Root.Callers {
		seen[caller.Name] = true
		if caller.Edge.Resolution.Status != Resolved || caller.Edge.Compatibility.Status != Compatible {
			t.Fatalf("dot import call: %+v", caller.Edge)
		}
	}
	for _, name := range []string{"Caller", "VarCaller", "Direct"} {
		if !seen["example.com/app."+name] {
			t.Fatalf("missing %s: %+v", name, result.Root.Callers)
		}
	}
}

func TestCrossFileMethodExpression(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod":    "module example.com/p\ngo 1.24\n",
		"target.go": "package p\ntype T struct{}\nfunc (T) Run(){}\n",
		"caller.go": "package p\nfunc Caller(v T){T.Run(v)}\nfunc ViaValue(v T){f := T.Run; f(v)}\n",
	}, "example.com/p.T#Run", Options{})
	if len(result.Root.Callers) != 2 {
		t.Fatalf("callers: %+v", result.Root.Callers)
	}
	for _, caller := range result.Root.Callers {
		if caller.Edge.Resolution.Status != Resolved || caller.Edge.Compatibility.Status != Compatible {
			t.Fatalf("method expression: %+v", caller.Edge)
		}
	}
}

func TestArrayLengthCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, length, argument string
		want                   CompatibilityStatus
	}{
		{"mismatch", "3", "[2]int{}", Incompatible},
		{"match", "3", "[3]int{}", Compatible},
		{"constant", "N", "[2]int{}", Incompatible},
		{"constant-expression", "N + 1", "[2]int{}", Incompatible},
		{"unknown", "external.N", "[2]int{}", CompatibilityUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runFixture(t, map[string]string{
				"go.mod": "module example.com/p\ngo 1.24\n",
				"p.go":   "package p\nconst N = 3\nfunc Target([" + tc.length + "]int){}\nfunc Caller(){Target(" + tc.argument + ")}\n",
			}, "example.com/p.Target", Options{})
			if len(result.Root.Callers) != 1 || result.Root.Callers[0].Edge.Compatibility.Status != tc.want {
				t.Fatalf("compatibility: %+v", result.Root.Callers)
			}
		})
	}
}
