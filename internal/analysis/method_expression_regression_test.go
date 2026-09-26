package analysis

import "testing"

func TestMethodExpressionReceiverCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, declarations, call, target string
		want                             CompatibilityStatus
	}{
		{"value-receiver", "type T struct{}; func (T) Run(){}", "func Caller(){var v T; T.Run(v)}", "example.com/p.T#Run", Compatible},
		{"pointer-receiver", "type T struct{}; func (*T) Run(){}", "func Caller(){var v *T; (*T).Run(v)}", "example.com/p.T#Run", Compatible},
		{"pointer-expression-value-method", "type T struct{}; func (T) Run(){}", "func Caller(){var v *T; (*T).Run(v)}", "example.com/p.T#Run", Compatible},
		{"value-expression-pointer-method", "type T struct{}; func (*T) Run(){}", "func Caller(){var v T; T.Run(v)}", "example.com/p.T#Run", Incompatible},
		{"value-expression-pointer-method-with-pointer-argument", "type T struct{}; func (*T) Run(){}", "func Caller(){var v T; T.Run(&v)}", "example.com/p.T#Run", Incompatible},
		{"promoted-value-method", "type Inner struct{}; func (Inner) Run(){}; type Outer struct{Inner}", "func Caller(){var v Outer; Outer.Run(v)}", "example.com/p.Inner#Run", Compatible},
		{"promoted-pointer-method", "type Inner struct{}; func (*Inner) Run(){}; type Outer struct{Inner}", "func Caller(){var v *Outer; (*Outer).Run(v)}", "example.com/p.Inner#Run", Compatible},
		{"promoted-pointer-field-method", "type Inner struct{}; func (*Inner) Run(){}; type Outer struct{*Inner}", "func Caller(){var v Outer; Outer.Run(v)}", "example.com/p.Inner#Run", Compatible},
		{"invalid-promoted-pointer-method", "type Inner struct{}; func (*Inner) Run(){}; type Outer struct{Inner}", "func Caller(){var v Outer; Outer.Run(v)}", "example.com/p.Inner#Run", Incompatible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runFixture(t, map[string]string{
				"go.mod": "module example.com/p\ngo 1.24\n",
				"p.go":   "package p\n" + tc.declarations + "\n" + tc.call + "\nfunc Above(){Caller()}\n",
			}, tc.target, Options{})
			edge := regressionEdge(t, result, "example.com/p.Caller", tc.target)
			if edge.Resolution.Status != Resolved || edge.Compatibility.Status != tc.want {
				t.Fatalf("method expression: %+v", edge)
			}
			if tc.want == Incompatible {
				found := false
				for _, issue := range edge.Compatibility.Issues {
					found = found || issue.Kind == "method-expression"
				}
				if !found {
					t.Fatalf("missing method-set issue: %+v", edge)
				}
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

func TestExternalMethodExpressionReceiverCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, source, target string
		want                 CompatibilityStatus
	}{
		{"pointer-expression-value-method", "import \"time\"\nfunc Caller(v *time.Time){(*time.Time).String(v)}", "time.Time#String", Compatible},
		{"value-expression-pointer-method", "import \"strings\"\nfunc Caller(v strings.Builder){strings.Builder.WriteString(v, \"x\")}", "strings.Builder#WriteString", Incompatible},
		{"ordinary-addressable-method-call", "import \"strings\"\nfunc Caller(){var v strings.Builder; v.WriteString(\"x\")}", "strings.Builder#WriteString", Compatible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runFixture(t, map[string]string{
				"go.mod": "module example.com/p\ngo 1.26\n",
				"p.go":   "package p\n" + tc.source + "\n",
			}, tc.target, Options{})
			edge := regressionEdge(t, result, "example.com/p.Caller", tc.target)
			if edge.Resolution.Status != External || edge.Compatibility.Status != tc.want {
				t.Fatalf("external method expression: %+v", edge)
			}
		})
	}
}
