package analysis

import "testing"

func TestExternalMethodDeclarationLookup(t *testing.T) {
	for _, tc := range []struct {
		pkg, name, receiver string
		params              int
	}{
		{"strings", "WriteString", "strings.Builder", 1},
		{"strings", "WriteString", "*strings.Builder", 1},
		{"os", "Chmod", "", 2},
		{"os", "Chmod", "os.File", 1},
		{"os", "Chmod", "os.Root", 2},
	} {
		t.Run(tc.pkg+tc.receiver+tc.name, func(t *testing.T) {
			f, err := AnalyzeExternalFunction(tc.pkg, tc.name, tc.receiver, BuildContext{})
			if err != nil || f == nil {
				t.Fatalf("missing external declaration: %+v %v", f, err)
			}
			if len(f.Params) != tc.params {
				t.Fatalf("wrong signature: %+v", f)
			}
			if tc.receiver != "" && receiverID(f.Receiver) != receiverID(tc.receiver) {
				t.Fatalf("wrong receiver: %+v", f)
			}
		})
	}
	for _, receiver := range []string{"", "strings.Reader"} {
		f, err := AnalyzeExternalFunction("strings", "WriteString", receiver, BuildContext{})
		if err != nil || f != nil {
			t.Fatalf("method mistaken for different symbol: %+v %v", f, err)
		}
	}
}

func TestExternalMethodsEndReverseTraversal(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod": "module example.com/p\ngo 1.26\n",
		"p.go":   "package p\nimport \"strings\"\nfunc Caller(){var b strings.Builder;b.WriteString(\"x\")}\nfunc Expression(b *strings.Builder){(*strings.Builder).WriteString(b,\"x\")}\nfunc Above(){Caller()}\n",
	}, "strings.Builder#WriteString", Options{})
	if len(result.Root.Callers) != 2 {
		t.Fatalf("callers: %+v", result.Root.Callers)
	}
	for _, node := range result.Root.Callers {
		edge := node.Edge
		if edge.Resolution.Status != External || edge.Resolution.Kind != "standard-library" || edge.Compatibility.Status != Compatible {
			t.Fatalf("external method: %+v", edge)
		}
		if len(node.Callers) != 0 {
			t.Fatalf("external boundary crossed: %+v", node)
		}
	}
}

func TestExternalMethodUsesTargetContext(t *testing.T) {
	for _, os := range []string{"windows", "linux"} {
		f, err := AnalyzeExternalFunction("syscall", "FindProc", "*syscall.DLL", BuildContext{GOOS: os, GOARCH: "amd64"})
		if err != nil {
			t.Fatal(err)
		}
		if (f != nil) != (os == "windows") {
			t.Fatalf("%s declaration: %+v", os, f)
		}
	}
}

func TestAbsentExternalMethodRemainsUnknown(t *testing.T) {
	result := runFixture(t, map[string]string{"go.mod": "module example.com/p\ngo 1.26\n", "p.go": "package p\nimport \"strings\"\nfunc Caller(){var b strings.Builder;b.AbsentMethod()}\nfunc Above(){Caller()}"}, "strings.Builder#AbsentMethod", Options{})
	edge := regressionEdge(t, result, "example.com/p.Caller", "strings.Builder#AbsentMethod")
	if edge.Resolution.Status != Unknown {
		t.Fatalf("invented external declaration: %+v", edge)
	}
	regressionEdge(t, result, "example.com/p.Above", "example.com/p.Caller")
}
