package analysis

import "testing"

func TestExternalMethodThroughTypeAlias(t *testing.T) {
	for _, tc := range []struct {
		name, alias, call string
		want              CompatibilityStatus
	}{
		{"direct-alias", "type B = strings.Builder", "var b B; b.WriteString(\"x\")", Compatible},
		{"alias-chain", "type A = strings.Builder; type B = A", "var b B; b.WriteString(\"x\")", Compatible},
		{"pointer-alias", "type B = *strings.Builder", "var b B; b.WriteString(\"x\")", Compatible},
		{"pointer-method-expression", "type B = strings.Builder", "var b *B; (*B).WriteString(b, \"x\")", Compatible},
		{"invalid-value-method-expression", "type B = strings.Builder", "var b B; B.WriteString(b, \"x\")", Incompatible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runFixture(t, map[string]string{
				"go.mod": "module example.com/p\ngo 1.26\n",
				"p.go":   "package p\nimport \"strings\"\n" + tc.alias + "\nfunc Caller(){" + tc.call + "}\nfunc Above(){Caller()}\n",
			}, "strings.Builder#WriteString", Options{})
			edge := regressionEdge(t, result, "example.com/p.Caller", "strings.Builder#WriteString")
			if edge.Resolution.Status != External || edge.Resolution.Kind != "standard-library" || edge.Compatibility.Status != tc.want {
				t.Fatalf("alias method edge: %+v", edge)
			}
			if len(result.Root.Callers) != 1 || len(result.Root.Callers[0].Callers) != 0 {
				t.Fatalf("external boundary crossed: %+v", result.Root)
			}
		})
	}
}

func TestDefinedTypeDoesNotInheritExternalMethods(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod": "module example.com/p\ngo 1.26\n",
		"p.go":   "package p\nimport \"strings\"\ntype B strings.Builder\nfunc Caller(){var b B; b.WriteString(\"x\")}\n",
	}, "strings.Builder#WriteString", Options{})
	for _, edge := range result.Edges {
		if edge.Caller == "example.com/p.Caller" {
			t.Fatalf("defined type inherited external method: %+v", edge)
		}
	}
}
