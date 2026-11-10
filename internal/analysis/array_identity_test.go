package analysis

import "testing"

func TestArrayElementIdentity(t *testing.T) {
	tests := []struct {
		name, declarations, parameter, argument string
		want                                    CompatibilityStatus
	}{
		{
			name:         "predeclared-alias",
			declarations: "",
			parameter:    "[2]uint8",
			argument:     "[2]byte{}",
			want:         Compatible,
		},
		{
			name:         "named-alias",
			declarations: "type A = int; type B = A",
			parameter:    "[2]B",
			argument:     "[2]A{}",
			want:         Compatible,
		},
		{
			name:         "distinct-named-types",
			declarations: "type A int; type B int",
			parameter:    "[2]B",
			argument:     "[2]A{}",
			want:         Incompatible,
		},
		{
			name:         "pointer-alias",
			declarations: "type A = int",
			parameter:    "[2]*int",
			argument:     "[2]*A{}",
			want:         Compatible,
		},
		{
			name:         "nested-array-alias",
			declarations: "type A = int",
			parameter:    "[2][3]int",
			argument:     "[2][3]A{}",
			want:         Compatible,
		},
		{
			name:         "nested-array-different-length",
			declarations: "type A = int",
			parameter:    "[2][3]int",
			argument:     "[2][4]A{}",
			want:         Incompatible,
		},
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
