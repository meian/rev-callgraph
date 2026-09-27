package analysis

import (
	"context"
	"go/build"
	"os"
	"path/filepath"
	"testing"
)

func TestExternalParseErrorIsCachedAsUnavailable(t *testing.T) {
	// 実toolchainは変更せず、このテスト専用のGOROOTに壊れた宣言を用意する。
	root := fixture(t, map[string]string{"src/strings/broken.go": "package strings\nfunc Broken( {\n"})
	original := build.Default
	build.Default.GOROOT = root
	t.Cleanup(func() { build.Default = original })
	if _, err := AnalyzeExternalFunction("strings", "Broken", "", BuildContext{}); err == nil {
		t.Fatal("fixture must cause a parse error")
	}
	workspace := &Workspace{}
	locator, err := NewLocator(workspace)
	if err != nil {
		t.Fatal(err)
	}
	e := &engine{ctx: context.Background(), workspace: workspace, locator: locator, models: map[string]SourceModel{}, functions: map[string]Function{}, types: map[string]Type{}, definitionCache: map[string]bool{}, externalCache: map[string]*Function{}}
	caller := Function{ID: "example.com/p.Caller", Package: "example.com/p"}
	call := Call{Package: "strings", Name: "Broken"}
	id, first, callee, err := e.resolve(caller, call)
	if err != nil || id != "strings.Broken" || callee != nil || first.Status != Unknown || first.Kind != "definition-unavailable" {
		t.Fatalf("parse error fallback: %s %+v %+v %v", id, first, callee, err)
	}
	if cached, ok := e.externalCache[id]; !ok || cached != nil {
		t.Fatal("failed external lookup not cached")
	}
	// 宣言を書き直しても同一実行のcacheは再解析せず、最初と同じunknownを返す。
	if err = os.WriteFile(filepath.Join(root, "src/strings/broken.go"), []byte("package strings\nfunc Broken(){}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if fresh, err := AnalyzeExternalFunction("strings", "Broken", "", BuildContext{}); err != nil || fresh == nil {
		t.Fatalf("corrected declaration: %+v %v", fresh, err)
	}
	_, second, callee, err := e.resolve(caller, call)
	if err != nil || first != second || callee != nil {
		t.Fatalf("cache changed outcome: %+v %+v %v", second, callee, err)
	}
}
