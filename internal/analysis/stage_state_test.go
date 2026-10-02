package analysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type retryLocator struct{ sources []Source }

func (l retryLocator) Definitions(_, _ string) []Source { return l.sources }
func (l retryLocator) Callers(_, _ string) []Source     { return nil }

func TestMissingSymbolPrefersCallerModule(t *testing.T) {
	result := runFixture(t, map[string]string{
		"a/go.mod": "module example.com/p\ngo 1.24\n",
		"a/a.go":   "package p\nfunc First() { Missing() }\n",
		"b/go.mod": "module example.com/p\ngo 1.24\n",
		"b/b.go":   "package p\nfunc Second() { Missing() }\n",
	}, "example.com/p.Missing", Options{})
	if len(result.Edges) != 2 {
		t.Fatalf("edges = %+v, want callers from both owning modules", result.Edges)
	}
	for _, edge := range result.Edges {
		if edge.Resolution.Status != Unknown || edge.Resolution.Kind != "missing-symbol" {
			t.Errorf("resolution for %s = %+v, want unknown/missing-symbol", edge.Caller, edge.Resolution)
		}
	}
}

func TestDefinitionLoadFailureDoesNotCacheCompletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "target.go")
	source := Source{Path: path, Package: "example.com/p", PackageName: "p"}
	readyPath := filepath.Join(filepath.Dir(path), "ready.go")
	if err := os.WriteFile(readyPath, []byte("package p\nfunc Ready() { Target() }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e := &engine{
		ctx:             context.Background(),
		locator:         retryLocator{[]Source{{Path: readyPath, Package: source.Package}, source}},
		models:          map[string]SourceModel{},
		functions:       map[string]Function{},
		types:           map[string]Type{},
		definitionCache: map[string]bool{},
	}
	if err := e.definitions(source.Package, "Target"); err == nil {
		t.Fatal("missing file should fail")
	}
	if e.definitionCache[source.Package+"\x00Target"] || len(e.models) != 1 {
		t.Fatal("failed lookup must preserve only completed file state")
	}
	if err := os.WriteFile(path, []byte("package p\nfunc Target() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := e.definitions(source.Package, "Target"); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.functions["example.com/p.Target"]; !ok {
		t.Fatal("retry did not load definition")
	}
	if e.stats.LocatorLookups != 2 || e.stats.AnalyzedSources != 2 || e.stats.CacheHits != 1 {
		t.Fatalf("unexpected retry stats: %+v", e.stats)
	}
	if err := e.definitions(source.Package, "Target"); err != nil {
		t.Fatal(err)
	}
	if e.stats.LocatorLookups != 2 || e.stats.CacheHits != 2 {
		t.Fatalf("completed lookup was not reused: %+v", e.stats)
	}
}
