package analysis

import "testing"

func TestChannelDirectionCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, declarations, argument, parameter string
		want                                    CompatibilityStatus
	}{
		{
			name:         "bidirectional-to-send",
			declarations: "",
			argument:     "chan int",
			parameter:    "chan<- int",
			want:         Compatible,
		},
		{
			name:         "bidirectional-to-receive",
			declarations: "",
			argument:     "chan int",
			parameter:    "<-chan int",
			want:         Compatible,
		},
		{
			name:         "send-to-bidirectional",
			declarations: "",
			argument:     "chan<- int",
			parameter:    "chan int",
			want:         Incompatible,
		},
		{
			name:         "receive-to-bidirectional",
			declarations: "",
			argument:     "<-chan int",
			parameter:    "chan int",
			want:         Incompatible,
		},
		{
			name:         "send-to-receive",
			declarations: "",
			argument:     "chan<- int",
			parameter:    "<-chan int",
			want:         Incompatible,
		},
		{
			name:         "receive-to-send",
			declarations: "",
			argument:     "<-chan int",
			parameter:    "chan<- int",
			want:         Incompatible,
		},
		{
			name:         "same-direction",
			declarations: "",
			argument:     "chan<- int",
			parameter:    "chan<- int",
			want:         Compatible,
		},
		{
			name:         "named-bidirectional-to-unnamed-receive",
			declarations: "type Both chan int",
			argument:     "Both",
			parameter:    "<-chan int",
			want:         Compatible,
		},
		{
			name:         "unnamed-bidirectional-to-named-send",
			declarations: "type Send chan<- int",
			argument:     "chan int",
			parameter:    "Send",
			want:         Compatible,
		},
		{
			name:         "named-send-to-unnamed-receive",
			declarations: "type Send chan<- int",
			argument:     "Send",
			parameter:    "<-chan int",
			want:         Incompatible,
		},
		{
			name:         "two-distinct-named-channels",
			declarations: "type Both chan int; type Send chan<- int",
			argument:     "Both",
			parameter:    "Send",
			want:         Incompatible,
		},
		{
			name:         "alias-to-send",
			declarations: "type Send = chan<- int",
			argument:     "chan int",
			parameter:    "Send",
			want:         Compatible,
		},
		{
			name:         "alias-from-send",
			declarations: "type Send = chan<- int",
			argument:     "Send",
			parameter:    "<-chan int",
			want:         Incompatible,
		},
		{
			name:         "different-element",
			declarations: "",
			argument:     "chan int",
			parameter:    "chan<- string",
			want:         Incompatible,
		},
		{
			name:         "nested-different-element",
			declarations: "",
			argument:     "chan chan int",
			parameter:    "chan chan string",
			want:         Incompatible,
		},
		{
			name:         "nested-different-direction",
			declarations: "",
			argument:     "chan chan int",
			parameter:    "chan (<-chan int)",
			want:         Incompatible,
		},
		{
			name:         "nested-same-element-outer-direction",
			declarations: "",
			argument:     "chan chan int",
			parameter:    "chan<- chan int",
			want:         Compatible,
		},
		{
			name:         "nested-alias-element",
			declarations: "type Inner = chan int",
			argument:     "chan Inner",
			parameter:    "chan chan int",
			want:         Compatible,
		},
		{
			name:         "nested-distinct-named-element",
			declarations: "type Inner chan int",
			argument:     "chan Inner",
			parameter:    "chan chan int",
			want:         Incompatible,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runFixture(t, map[string]string{
				"go.mod": "module example.com/p\ngo 1.24\n",
				"p.go":   "package p\n" + tc.declarations + "\nfunc Target(v " + tc.parameter + "){}\nfunc Caller(){var v " + tc.argument + "; Target(v)}\nfunc Above(){Caller()}\n",
			}, "example.com/p.Target", Options{})
			edge := regressionEdge(t, result, "example.com/p.Caller", "example.com/p.Target")
			if edge.Compatibility.Status != tc.want {
				t.Fatalf("got %+v, want %s", edge.Compatibility, tc.want)
			}
			wantUpper := 0
			if tc.want == Compatible {
				wantUpper = 1
			}
			if len(result.Root.Callers) != 1 || len(result.Root.Callers[0].Callers) != wantUpper {
				t.Fatalf("unexpected traversal: %+v", result.Root)
			}
		})
	}
}

func TestReceivedChannelElementCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, declarations, argument, parameter string
		separateDeclaration                     bool
		want                                    CompatibilityStatus
	}{
		{
			name:                "bidirectional-element",
			declarations:        "",
			argument:            "chan chan int",
			parameter:           "chan int",
			separateDeclaration: false,
			want:                Compatible,
		},
		{
			name:                "send-only-element",
			declarations:        "",
			argument:            "<-chan chan<- int",
			parameter:           "chan<- int",
			separateDeclaration: false,
			want:                Compatible,
		},
		{
			name:                "direction-mismatch",
			declarations:        "",
			argument:            "chan (<-chan int)",
			parameter:           "chan int",
			separateDeclaration: false,
			want:                Incompatible,
		},
		{
			name:                "local-named-channel",
			declarations:        "type Channel chan chan int",
			argument:            "Channel",
			parameter:           "chan int",
			separateDeclaration: false,
			want:                Compatible,
		},
		{
			name:                "local-alias-chain",
			declarations:        "type Channel chan chan int; type Alias = Channel",
			argument:            "Alias",
			parameter:           "chan int",
			separateDeclaration: false,
			want:                Compatible,
		},
		{
			name:                "other-file-named-channel",
			declarations:        "type Channel chan chan int",
			argument:            "Channel",
			parameter:           "chan int",
			separateDeclaration: true,
			want:                CompatibilityUnknown,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			declarations := tc.declarations
			files := map[string]string{"go.mod": "module example.com/p\ngo 1.24\n"}
			if tc.separateDeclaration {
				files["types.go"] = "package p\n" + declarations + "\n"
				declarations = ""
			}
			files["p.go"] = "package p\n" + declarations + "\nfunc Target(v " + tc.parameter + "){}\nfunc Caller(ch " + tc.argument + "){Target(<-ch)}\nfunc Above(){Caller(nil)}\n"
			result := runFixture(t, files, "example.com/p.Target", Options{})
			edge := regressionEdge(t, result, "example.com/p.Caller", "example.com/p.Target")
			if edge.Compatibility.Status != tc.want {
				t.Fatalf("got %+v, want %s", edge.Compatibility, tc.want)
			}
			wantUpper := 1
			if tc.want == Incompatible {
				wantUpper = 0
			}
			if len(result.Root.Callers) != 1 || len(result.Root.Callers[0].Callers) != wantUpper {
				t.Fatalf("unexpected traversal: %+v", result.Root)
			}
		})
	}
}
