package analysis_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/meian/rev-callgraph/internal/analysis"
)

func TestMethodCallReceiverAddressability(t *testing.T) {
	for _, tc := range []struct {
		name         string
		declarations string
		body         string
		want         analysis.CompatibilityStatus
	}{
		{
			name: "variable",
			body: "var v T; v.Run()",
			want: analysis.Compatible,
		},
		{
			name: "parenthesized-variable",
			body: "var v T; (v).Run()",
			want: analysis.Compatible,
		},
		{
			name: "pointer-literal",
			body: "(&T{}).Run()",
			want: analysis.Compatible,
		},
		{
			name: "dereference",
			body: "var v *T; (*v).Run()",
			want: analysis.Compatible,
		},
		{
			name:         "pointer-result",
			declarations: "func Make() *T {return nil}",
			body:         "Make().Run()",
			want:         analysis.Compatible,
		},
		{
			name: "slice-element",
			body: "var v []T; v[0].Run()",
			want: analysis.Compatible,
		},
		{
			name: "slice-literal-element",
			body: "([]T{{}})[0].Run()",
			want: analysis.Compatible,
		},
		{
			name:         "named-slice-element",
			declarations: "type Items []T",
			body:         "var v Items; v[0].Run()",
			want:         analysis.Compatible,
		},
		{
			name: "array-element",
			body: "var v [1]T; v[0].Run()",
			want: analysis.Compatible,
		},
		{
			name:         "pointer-array-element",
			declarations: "func Make() *[1]T {return nil}",
			body:         "Make()[0].Run()",
			want:         analysis.Compatible,
		},
		{
			name:         "named-pointer-array-element",
			declarations: "type Items [1]T; func Make() *Items {return nil}",
			body:         "Make()[0].Run()",
			want:         analysis.Compatible,
		},
		{
			name: "map-slice-element",
			body: "var v map[int][]T; v[0][0].Run()",
			want: analysis.Compatible,
		},
		{
			name: "map-pointer-element",
			body: "var v map[int]*T; v[0].Run()",
			want: analysis.Compatible,
		},
		{
			name:         "struct-field",
			declarations: "type Box struct{Item T}",
			body:         "var v Box; v.Item.Run()",
			want:         analysis.Compatible,
		},
		{
			name:         "pointer-field",
			declarations: "type Box struct{Item *T}",
			body:         "Box{}.Item.Run()",
			want:         analysis.Compatible,
		},
		{
			name:         "pointer-base-field",
			declarations: "type Box struct{Item T}",
			body:         "(&Box{}).Item.Run()",
			want:         analysis.Compatible,
		},
		{
			name:         "promoted-addressable",
			declarations: "type Box struct{T}",
			body:         "var v Box; v.Run()",
			want:         analysis.Compatible,
		},
		{
			name:         "promoted-pointer-field",
			declarations: "type Box struct{*T}",
			body:         "Box{}.Run()",
			want:         analysis.Compatible,
		},
		{
			name:         "alias-variable",
			declarations: "type Alias = T",
			body:         "var v Alias; v.Run()",
			want:         analysis.Compatible,
		},
		{
			name: "method-value",
			body: "var v T; f := v.Run; f()",
			want: analysis.Compatible,
		},
		{
			name:         "value-method-literal",
			declarations: "func (T) Value(){}",
			body:         "T{}.Value()",
			want:         analysis.Compatible,
		},
		{
			name:         "value-method-map",
			declarations: "func (T) Value(){}",
			body:         "var v map[int]T; v[0].Value()",
			want:         analysis.Compatible,
		},
		{
			name: "literal",
			body: "T{}.Run()",
			want: analysis.Incompatible,
		},
		{
			name: "parenthesized-literal",
			body: "(T{}).Run()",
			want: analysis.Incompatible,
		},
		{
			name:         "function-result",
			declarations: "func Make() T {return T{}}",
			body:         "Make().Run()",
			want:         analysis.Incompatible,
		},
		{
			name: "map-element",
			body: "var v map[int]T; v[0].Run()",
			want: analysis.Incompatible,
		},
		{
			name: "nested-map-key",
			body: "var v map[[1]int]T; v[[1]int{}].Run()",
			want: analysis.Incompatible,
		},
		{
			name:         "named-map-element",
			declarations: "type Items map[int]T",
			body:         "var v Items; v[0].Run()",
			want:         analysis.Incompatible,
		},
		{
			name: "array-literal-element",
			body: "([1]T{{}})[0].Run()",
			want: analysis.Incompatible,
		},
		{
			name:         "array-result-element",
			declarations: "func Make() [1]T {return [1]T{}}",
			body:         "Make()[0].Run()",
			want:         analysis.Incompatible,
		},
		{
			name:         "struct-literal-field",
			declarations: "type Box struct{Item T}",
			body:         "Box{}.Item.Run()",
			want:         analysis.Incompatible,
		},
		{
			name:         "map-struct-field",
			declarations: "type Box struct{Item T}",
			body:         "var v map[int]Box; v[0].Item.Run()",
			want:         analysis.Incompatible,
		},
		{
			name:         "promoted-nonaddressable",
			declarations: "type Box struct{T}",
			body:         "Box{}.Run()",
			want:         analysis.Incompatible,
		},
		{
			name: "invalid-method-value",
			body: "f := T{}.Run; f()",
			want: analysis.Incompatible,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := "example.com/p.T#Run"
			if tc.name == "value-method-literal" || tc.name == "value-method-map" {
				target = "example.com/p.T#Value"
			}
			result := analyzeReceiverFixture(t, map[string]string{
				"p.go": "package p\ntype T struct{}\nfunc (*T) Run(){}\n" + tc.declarations +
					"\nfunc Caller(){" + tc.body + "}\nfunc Above(){Caller()}\n",
			}, target)
			if len(result.Root.Callers) != 1 {
				t.Fatalf("callers = %+v", result.Root.Callers)
			}
			caller := result.Root.Callers[0]
			if caller.Edge.Resolution.Status != analysis.Resolved || caller.Edge.Compatibility.Status != tc.want {
				t.Fatalf("edge = %+v, want resolved / %s", caller.Edge, tc.want)
			}
			wantUpper := 1
			if tc.want == analysis.Incompatible {
				wantUpper = 0
				if len(caller.Edge.Compatibility.Issues) != 1 || caller.Edge.Compatibility.Issues[0].Kind != "receiver-addressability" {
					t.Errorf("issues = %+v", caller.Edge.Compatibility.Issues)
				}
			}
			if len(caller.Callers) != wantUpper {
				t.Errorf("upper callers = %d, want %d", len(caller.Callers), wantUpper)
			}
		})
	}
}

func TestMethodCallConstantReceiverAddressability(t *testing.T) {
	for _, body := range []string{"c.Run()", "const v T = 0; v.Run()"} {
		t.Run(body, func(t *testing.T) {
			result := analyzeReceiverFixture(t, map[string]string{
				"p.go": "package p\ntype T int; func (*T) Run(){}; const c T = 0\nfunc Caller(){" + body + "}\nfunc Above(){Caller()}\n",
			}, "example.com/p.T#Run")
			if len(result.Root.Callers) != 1 {
				t.Fatalf("callers = %+v", result.Root.Callers)
			}
			caller := result.Root.Callers[0]
			if caller.Edge.Resolution.Status != analysis.Resolved || caller.Edge.Compatibility.Status != analysis.Incompatible || len(caller.Callers) != 0 {
				t.Fatalf("constant receiver must stop traversal: %+v", caller)
			}
		})
	}
}

func TestMethodCallReceiverAddressabilityUnknown(t *testing.T) {
	result := analyzeReceiverFixture(t, map[string]string{
		"types.go": "package p\ntype T struct{}; func (*T) Run(){}; type Box struct{Item T}\n",
		"p.go":     "package p\nfunc Make() Box {return Box{}}\nfunc Caller(){Make().Item.Run()}\nfunc Above(){Caller()}\n",
	}, "example.com/p.T#Run")
	if len(result.Root.Callers) != 1 {
		t.Fatalf("callers = %+v", result.Root.Callers)
	}
	caller := result.Root.Callers[0]
	if caller.Edge.Resolution.Status != analysis.Resolved || caller.Edge.Compatibility.Status != analysis.CompatibilityUnknown {
		t.Fatalf("edge = %+v, want resolved / unknown", caller.Edge)
	}
	if len(caller.Callers) != 1 {
		t.Fatalf("unknown receiver must continue traversal: %+v", caller)
	}
}

func TestExternalMethodCallReceiverAddressability(t *testing.T) {
	for _, tc := range []struct {
		name         string
		declarations string
		body         string
		want         analysis.CompatibilityStatus
	}{
		{
			name: "variable",
			body: "var v strings.Builder; v.WriteString(\"x\")",
			want: analysis.Compatible,
		},
		{
			name: "pointer-literal",
			body: "(&strings.Builder{}).WriteString(\"x\")",
			want: analysis.Compatible,
		},
		{
			name:         "alias-variable",
			declarations: "type B = strings.Builder",
			body:         "var v B; v.WriteString(\"x\")",
			want:         analysis.Compatible,
		},
		{
			name:         "pointer-alias-result",
			declarations: "type B = *strings.Builder; func Make() B {return nil}",
			body:         "Make().WriteString(\"x\")",
			want:         analysis.Compatible,
		},
		{
			name: "literal",
			body: "strings.Builder{}.WriteString(\"x\")",
			want: analysis.Incompatible,
		},
		{
			name:         "alias-literal",
			declarations: "type B = strings.Builder",
			body:         "B{}.WriteString(\"x\")",
			want:         analysis.Incompatible,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := analyzeReceiverFixture(t, map[string]string{
				"p.go": "package p\nimport \"strings\"\n" + tc.declarations + "\nfunc Caller(){" + tc.body + "}\n",
			}, "strings.Builder#WriteString")
			if len(result.Root.Callers) != 1 {
				t.Fatalf("callers = %+v", result.Root.Callers)
			}
			edge := result.Root.Callers[0].Edge
			if edge.Resolution.Status != analysis.External || edge.Compatibility.Status != tc.want {
				t.Fatalf("edge = %+v, want external / %s", edge, tc.want)
			}
		})
	}
}

func analyzeReceiverFixture(t *testing.T, files map[string]string, target string) *analysis.Result {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = "module example.com/p\ngo 1.27\n"
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := analysis.Analyze(context.Background(), target, analysis.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
