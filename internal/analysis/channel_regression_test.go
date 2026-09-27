package analysis

import "testing"

func TestChannelDirectionCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, declarations, argument, parameter string
		want                                    CompatibilityStatus
	}{
		{"bidirectional-to-send", "", "chan int", "chan<- int", Compatible},
		{"bidirectional-to-receive", "", "chan int", "<-chan int", Compatible},
		{"send-to-bidirectional", "", "chan<- int", "chan int", Incompatible},
		{"receive-to-bidirectional", "", "<-chan int", "chan int", Incompatible},
		{"send-to-receive", "", "chan<- int", "<-chan int", Incompatible},
		{"receive-to-send", "", "<-chan int", "chan<- int", Incompatible},
		{"same-direction", "", "chan<- int", "chan<- int", Compatible},
		{"named-bidirectional-to-unnamed-receive", "type Both chan int", "Both", "<-chan int", Compatible},
		{"unnamed-bidirectional-to-named-send", "type Send chan<- int", "chan int", "Send", Compatible},
		{"named-send-to-unnamed-receive", "type Send chan<- int", "Send", "<-chan int", Incompatible},
		{"two-distinct-named-channels", "type Both chan int; type Send chan<- int", "Both", "Send", Incompatible},
		{"alias-to-send", "type Send = chan<- int", "chan int", "Send", Compatible},
		{"alias-from-send", "type Send = chan<- int", "Send", "<-chan int", Incompatible},
		{"different-element", "", "chan int", "chan<- string", Incompatible},
		{"nested-different-element", "", "chan chan int", "chan chan string", Incompatible},
		{"nested-different-direction", "", "chan chan int", "chan (<-chan int)", Incompatible},
		{"nested-same-element-outer-direction", "", "chan chan int", "chan<- chan int", Compatible},
		{"nested-alias-element", "type Inner = chan int", "chan Inner", "chan chan int", Compatible},
		{"nested-distinct-named-element", "type Inner chan int", "chan Inner", "chan chan int", Incompatible},
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
		{"bidirectional-element", "", "chan chan int", "chan int", false, Compatible},
		{"send-only-element", "", "<-chan chan<- int", "chan<- int", false, Compatible},
		{"direction-mismatch", "", "chan (<-chan int)", "chan int", false, Incompatible},
		{"local-named-channel", "type Channel chan chan int", "Channel", "chan int", false, Compatible},
		{"local-alias-chain", "type Channel chan chan int; type Alias = Channel", "Alias", "chan int", false, Compatible},
		{"other-file-named-channel", "type Channel chan chan int", "Channel", "chan int", true, CompatibilityUnknown},
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
