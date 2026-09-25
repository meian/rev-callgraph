package analysis

import "testing"

func TestArrayElementIdentity(t *testing.T) {
	tests := []struct {
		name, declarations, parameter, argument string
		want                                    CompatibilityStatus
	}{
		{"predeclared-alias", "", "[2]uint8", "[2]byte{}", Compatible},
		{"named-alias", "type A = int; type B = A", "[2]B", "[2]A{}", Compatible},
		{"distinct-named-types", "type A int; type B int", "[2]B", "[2]A{}", Incompatible},
		{"pointer-alias", "type A = int", "[2]*int", "[2]*A{}", Compatible},
		{"nested-array-alias", "type A = int", "[2][3]int", "[2][3]A{}", Compatible},
		{"nested-array-different-length", "type A = int", "[2][3]int", "[2][4]A{}", Incompatible},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runFixture(t, map[string]string{
				"go.mod":    "module example.com/p\ngo 1.24\n",
				"types.go":  "package p\n" + tc.declarations + "\n",
				"caller.go": "package p\nfunc Target(v " + tc.parameter + "){}\nfunc Caller(){Target(" + tc.argument + ")}\n",
			}, "example.com/p.Target", Options{})
			edge := regressionEdge(t, result, "example.com/p.Caller", "example.com/p.Target")
			if edge.Compatibility.Status != tc.want {
				t.Fatalf("got %+v, want %s", edge.Compatibility, tc.want)
			}
		})
	}
}
