package analysis

import "testing"

func TestPromotedMethodsSatisfyInterface(t *testing.T) {
	tests := []struct {
		name, types, argument string
		want                  CompatibilityStatus
	}{
		{"embedded-value", "type Inner struct{}; func (Inner) Run(){}; type Outer struct{ Inner }", "Outer{}", Compatible},
		{"embedded-pointer", "type Inner struct{}; func (*Inner) Run(){}; type Outer struct{ *Inner }", "Outer{}", Compatible},
		{"pointer-outer", "type Inner struct{}; func (*Inner) Run(){}; type Outer struct{ Inner }", "&Outer{}", Compatible},
		{"value-outer", "type Inner struct{}; func (*Inner) Run(){}; type Outer struct{ Inner }", "Outer{}", Incompatible},
		{"shadowed-field", "type Inner struct{}; func (Inner) Run(){}; type Outer struct{ Inner; Run int }", "Outer{}", Incompatible},
		{"ambiguous-promotion", "type A struct{}; func (A) Run(){}; type B struct{}; func (B) Run(){}; type Outer struct{ A; B }", "Outer{}", Incompatible},
		{"direct-method-shadows", "type Inner struct{}; func (Inner) Run(int){}; type Outer struct{ Inner }; func (Outer) Run(){}", "Outer{}", Compatible},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runFixture(t, map[string]string{
				"go.mod":    "module example.com/p\ngo 1.24\n",
				"types.go":  "package p\ntype I interface{ Run() }\n" + tc.types + "\n",
				"caller.go": "package p\nfunc Target(I){}\nfunc Caller(){Target(" + tc.argument + ")}\nfunc Above(){Caller()}\n",
			}, "example.com/p.Target", Options{})
			edge := regressionEdge(t, result, "example.com/p.Caller", "example.com/p.Target")
			if edge.Compatibility.Status != tc.want {
				t.Fatalf("got %+v, want %s", edge.Compatibility, tc.want)
			}
			if tc.want == Compatible {
				regressionEdge(t, result, "example.com/p.Above", "example.com/p.Caller")
			} else if len(result.Root.Callers) != 1 || len(result.Root.Callers[0].Callers) != 0 {
				t.Fatalf("incompatible call must stop traversal: %+v", result.Root)
			}
		})
	}
}

func TestBasicTypeAliasLacksInterfaceMethod(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod":    "module example.com/p\ngo 1.24\n",
		"types.go":  "package p\ntype NoMethods = int\ntype I interface{ Missing() }\n",
		"caller.go": "package p\nfunc Target(I){}\nfunc Caller(v NoMethods){Target(v)}\nfunc Above(){Caller(1)}\n",
	}, "example.com/p.Target", Options{})
	edge := regressionEdge(t, result, "example.com/p.Caller", "example.com/p.Target")
	if edge.Compatibility.Status != Incompatible {
		t.Fatalf("basic type alias lacks Missing method: %+v", edge.Compatibility)
	}
	if len(result.Root.Callers) != 1 || len(result.Root.Callers[0].Callers) != 0 {
		t.Fatalf("incompatible call must stop traversal: %+v", result.Root)
	}
}
