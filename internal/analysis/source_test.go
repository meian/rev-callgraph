package analysis

import (
	"os"
	"path/filepath"
	"testing"
)

func parseSourceForTest(t *testing.T, body string) SourceModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.go")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	model, err := AnalyzeSource(Source{Path: path, Package: "example.com/app", PackageName: "app"})
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func TestAnalyzeSourceDeclarationsAndCalls(t *testing.T) {
	model := parseSourceForTest(t, `package app
import alias "example.com/lib"
type Item struct { Name string }
type Other = alias.Item
type Contract interface { Run(string) error }
func Local(v string) (string, error) { return v, nil }
func (i *Item) Do(v string) string {
    f := alias.Run
    f(v)
    i.Name = Local(v)
    i.Do(v)
    return alias.Find(v)
}
`)
	if len(model.Types) != 3 {
		t.Fatalf("types: %+v", model.Types)
	}
	if model.Types[0].Fields["Name"].Name != "string" {
		t.Fatalf("field: %+v", model.Types[0])
	}
	if !model.Types[1].Alias || model.Types[1].Underlying != "example.com/lib.Item" {
		t.Fatalf("alias: %+v", model.Types[1])
	}
	if len(model.Types[2].Methods["Run"].Params) != 1 {
		t.Fatalf("interface: %+v", model.Types[2])
	}
	var method *Function
	for i := range model.Functions {
		if model.Functions[i].Name == "Do" {
			method = &model.Functions[i]
		}
	}
	if method == nil || method.ID != "example.com/app.Item#Do" || method.Receiver != "*example.com/app.Item" {
		t.Fatalf("method: %+v", method)
	}
	if len(method.Calls) != 4 {
		t.Fatalf("calls: %+v", method.Calls)
	}
	if method.Calls[0].Package != "example.com/lib" || method.Calls[0].Name != "Run" {
		t.Fatalf("function value: %+v", method.Calls[0])
	}
	if method.Calls[2].Receiver != "*example.com/app.Item" {
		t.Fatalf("method call: %+v", method.Calls[2])
	}
	if !method.Calls[3].CheckResults || method.Calls[3].ResultCount != 1 || method.Calls[3].ExpectedResults[0].Name != "string" {
		t.Fatalf("return context: %+v", method.Calls[3])
	}
}

func TestAnalyzeSourceUnknownAndResultCount(t *testing.T) {
	model := parseSourceForTest(t, `package app
func F() (int,error) { return 1,nil }
func G(v any) {
    x, err := F()
    _, _ = x, err
    v.Run(1)
    F()
    println(1)
}
`)
	var calls []Call
	for _, fn := range model.Functions {
		if fn.Name == "G" {
			calls = fn.Calls
		}
	}
	if len(calls) != 3 {
		t.Fatalf("calls: %+v", calls)
	}
	if !calls[0].CheckResults || calls[0].ResultCount != 2 {
		t.Fatalf("assignment: %+v", calls[0])
	}
	if !calls[1].Indirect || calls[1].Name != "" {
		t.Fatalf("unknown receiver: %+v", calls[1])
	}
	if calls[2].CheckResults {
		t.Fatalf("expression statement: %+v", calls[2])
	}
}

func TestAnalyzeSourceAssignmentTypesAndMethodExpressions(t *testing.T) {
	model := parseSourceForTest(t, `package app
import ext "example.com/ext"
type Item struct{}
func (i *Item) Use(v string) {}
func Target() int { return 0 }
func Generic[T any](v T) {}
func Caller() {
    var value string
    value = Target()
    {
        value := 1
        Generic[int](value)
    }
    Generic[string](value)
    var item Item
    (*Item).Use(&item, value)
    (*ext.Other).Use(&item, value)
}
`)
	var calls []Call
	for _, fn := range model.Functions {
		if fn.Name == "Caller" {
			calls = fn.Calls
		}
	}
	if len(calls) != 5 {
		t.Fatalf("calls: %+v", calls)
	}
	if !calls[0].CheckResults || calls[0].ExpectedResults[0].Name != "string" {
		t.Fatalf("typed assignment: %+v", calls[0])
	}
	if calls[1].Name != "Generic" || calls[1].Arguments[0].Name != "int" {
		t.Fatalf("inner generic: %+v", calls[1])
	}
	if calls[2].Arguments[0].Name != "string" {
		t.Fatalf("outer variable changed: %+v", calls[2])
	}
	if !calls[3].MethodExpression || calls[3].Receiver != "*example.com/app.Item" {
		t.Fatalf("local method expression: %+v", calls[3])
	}
	if !calls[4].MethodExpression || calls[4].Receiver != "*example.com/ext.Other" {
		t.Fatalf("imported method expression: %+v", calls[4])
	}
}

func TestAnalyzeSourceCgoPreambleAndHeader(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "local.h"), []byte("int from_header(int);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "source.go")
	content := `package app
/*
#include "local.h"
int from_preamble(int);
*/
import "C"
func F() { C.from_preamble(1); C.from_header(2); C.missing(3) }
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	model, err := AnalyzeSource(Source{Path: path, Package: "example.com/app", PackageName: "app"})
	if err != nil {
		t.Fatal(err)
	}
	calls := model.Functions[0].Calls
	if len(calls) != 3 {
		t.Fatalf("calls: %+v", calls)
	}
	if calls[0].ExternalKind != "cgo-preamble" || calls[0].Signature == nil || len(calls[0].Signature.Params) != 1 {
		t.Fatalf("preamble: %+v", calls[0])
	}
	if calls[1].ExternalKind != "cgo-header" || calls[1].Signature == nil {
		t.Fatalf("header: %+v", calls[1])
	}
	if calls[2].Signature != nil || calls[2].ExternalKind != "" {
		t.Fatalf("unknown C declaration: %+v", calls[2])
	}
}

func TestAnalyzeExternalStdlib(t *testing.T) {
	sig, err := AnalyzeExternal("fmt", "Sprintf")
	if err != nil || sig == nil || len(sig.Params) != 2 || !sig.Variadic || len(sig.Results) != 1 {
		t.Fatalf("fmt.Sprintf: %+v, %v", sig, err)
	}
	sig, err = AnalyzeExternal("fmt", "DefinitelyNotARealFunction")
	if err != nil || sig != nil {
		t.Fatalf("missing: %+v, %v", sig, err)
	}
}

func TestAnalyzeSourceFieldReceiverAndMethodValue(t *testing.T) {
	model := parseSourceForTest(t, `package app
type Item struct{}
func (i Item) Run() {}
type Holder struct { Value Item }
func Caller(h Holder) {
    h.Value.Run()
    f := h.Value.Run
    f()
}
`)
	var calls []Call
	for _, fn := range model.Functions {
		if fn.Name == "Caller" {
			calls = fn.Calls
		}
	}
	if len(calls) != 2 {
		t.Fatalf("calls: %+v", calls)
	}
	for _, call := range calls {
		if call.Receiver != "example.com/app.Item" || call.Name != "Run" {
			t.Fatalf("receiver: %+v", call)
		}
	}
}

func TestAnalyzeSourceImportAliasShadow(t *testing.T) {
	model := parseSourceForTest(t, `package app
import ext "example.com/ext"
type Local struct{}
func (Local) Run() {}
func Caller() {
    ext := Local{}
    ext.Run()
    f := ext.Run
    f()
}
`)
	var calls []Call
	for _, fn := range model.Functions {
		if fn.Name == "Caller" {
			calls = fn.Calls
		}
	}
	if len(calls) != 2 {
		t.Fatalf("calls: %+v", calls)
	}
	for _, call := range calls {
		if call.Package != "example.com/app" || call.Receiver != "example.com/app.Local" || call.Name != "Run" {
			t.Fatalf("shadowed import: %+v", call)
		}
	}
}

func TestAnalyzeExternalContextTargetOS(t *testing.T) {
	linux, err := AnalyzeExternalContext("syscall", "Gettid", BuildContext{GOOS: "linux", GOARCH: "amd64"})
	if err != nil || linux == nil {
		t.Fatalf("linux Gettid: %+v, %v", linux, err)
	}
	darwin, err := AnalyzeExternalContext("syscall", "Gettid", BuildContext{GOOS: "darwin", GOARCH: "arm64"})
	if err != nil || darwin != nil {
		t.Fatalf("darwin Gettid: %+v, %v", darwin, err)
	}
}
