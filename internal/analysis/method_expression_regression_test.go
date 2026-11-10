package analysis

import "testing"

func TestMethodExpressionReceiverCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, declarations, call, target string
		want                             CompatibilityStatus
	}{
		{
			name:         "value-receiver",
			declarations: "type T struct{}; func (T) Run(){}",
			call:         "func Caller(){var v T; T.Run(v)}",
			target:       "example.com/p.T#Run",
			want:         Compatible,
		},
		{
			name:         "pointer-receiver",
			declarations: "type T struct{}; func (*T) Run(){}",
			call:         "func Caller(){var v *T; (*T).Run(v)}",
			target:       "example.com/p.T#Run",
			want:         Compatible,
		},
		{
			name:         "pointer-expression-value-method",
			declarations: "type T struct{}; func (T) Run(){}",
			call:         "func Caller(){var v *T; (*T).Run(v)}",
			target:       "example.com/p.T#Run",
			want:         Compatible,
		},
		{
			name:         "value-expression-pointer-method",
			declarations: "type T struct{}; func (*T) Run(){}",
			call:         "func Caller(){var v T; T.Run(v)}",
			target:       "example.com/p.T#Run",
			want:         Incompatible,
		},
		{
			name:         "value-expression-pointer-method-with-pointer-argument",
			declarations: "type T struct{}; func (*T) Run(){}",
			call:         "func Caller(){var v T; T.Run(&v)}",
			target:       "example.com/p.T#Run",
			want:         Incompatible,
		},
		{
			name:         "promoted-value-method",
			declarations: "type Inner struct{}; func (Inner) Run(){}; type Outer struct{Inner}",
			call:         "func Caller(){var v Outer; Outer.Run(v)}",
			target:       "example.com/p.Inner#Run",
			want:         Compatible,
		},
		{
			name:         "promoted-pointer-method",
			declarations: "type Inner struct{}; func (*Inner) Run(){}; type Outer struct{Inner}",
			call:         "func Caller(){var v *Outer; (*Outer).Run(v)}",
			target:       "example.com/p.Inner#Run",
			want:         Compatible,
		},
		{
			name:         "promoted-pointer-field-method",
			declarations: "type Inner struct{}; func (*Inner) Run(){}; type Outer struct{*Inner}",
			call:         "func Caller(){var v Outer; Outer.Run(v)}",
			target:       "example.com/p.Inner#Run",
			want:         Compatible,
		},
		{
			name:         "invalid-promoted-pointer-method",
			declarations: "type Inner struct{}; func (*Inner) Run(){}; type Outer struct{Inner}",
			call:         "func Caller(){var v Outer; Outer.Run(v)}",
			target:       "example.com/p.Inner#Run",
			want:         Incompatible,
		},
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
		{
			name:   "pointer-expression-value-method",
			source: "import \"time\"\nfunc Caller(v *time.Time){(*time.Time).String(v)}",
			target: "time.Time#String",
			want:   Compatible,
		},
		{
			name:   "value-expression-pointer-method",
			source: "import \"strings\"\nfunc Caller(v strings.Builder){strings.Builder.WriteString(v, \"x\")}",
			target: "strings.Builder#WriteString",
			want:   Incompatible,
		},
		{
			name:   "ordinary-addressable-method-call",
			source: "import \"strings\"\nfunc Caller(){var v strings.Builder; v.WriteString(\"x\")}",
			target: "strings.Builder#WriteString",
			want:   Compatible,
		},
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
