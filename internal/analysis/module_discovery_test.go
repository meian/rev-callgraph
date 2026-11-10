package analysis_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/meian/rev-callgraph/internal/analysis"
)

func TestDiscoverModuleBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  map[string]string
	}{
		{
			name: "nested modules",
			files: map[string]string{
				"go.mod":                       "module example.com/parent\ngo 1.24\n",
				"parent.go":                    "package parent\nfunc Parent() {}\n",
				"child/go.mod":                 "module example.com/child\ngo 1.24\n",
				"child/child.go":               "package child\nfunc Child() {}\n",
				"child/grandchild/go.mod":      "module example.com/grandchild\ngo 1.24\n",
				"child/grandchild/cmd/main.go": "package main\nfunc main() {}\n",
				"childish/regular.go":          "package childish\nfunc Regular() {}\n",
				"vendor/ignored/go.mod":        "invalid module\n",
				"vendor/ignored/broken.go":     "invalid source\n",
				"child/.git/ignored/go.mod":    "invalid module\n",
				"child/.git/ignored/broken.go": "invalid source\n",
			},
			want: map[string]string{
				"parent.go":                    "example.com/parent",
				"child/child.go":               "example.com/child",
				"child/grandchild/cmd/main.go": "example.com/grandchild/cmd",
				"childish/regular.go":          "example.com/parent/childish",
			},
		},
		{
			name: "workspace without root module",
			files: map[string]string{
				"orphan.go":  "invalid source\n",
				"a/go.mod":   "module example.com/a\ngo 1.24\n",
				"a/a.go":     "package a\nfunc A() {}\n",
				"ab/go.mod":  "module example.com/ab\ngo 1.24\n",
				"ab/b/b.go":  "package b\nfunc B() {}\n",
				"ab/test.go": "package ab\nfunc Test() {}\n",
			},
			want: map[string]string{
				"a/a.go":     "example.com/a",
				"ab/b/b.go":  "example.com/ab/b",
				"ab/test.go": "example.com/ab",
			},
		},
		{
			name: "parent without sources",
			files: map[string]string{
				"go.mod":         "module example.com/parent\ngo 1.24\n",
				"child/go.mod":   "module example.com/child\ngo 1.24\n",
				"child/child.go": "package child\nfunc Child() {}\n",
			},
			want: map[string]string{
				"child/child.go": "example.com/child",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			w, err := analysis.Discover(context.Background(), analysis.Options{Dir: root})
			if err != nil {
				t.Fatal(err)
			}
			if len(w.Sources) != len(tc.want) {
				t.Fatalf("sources = %d, want %d", len(w.Sources), len(tc.want))
			}
			seen := make(map[string]bool)
			for _, source := range w.Sources {
				rel, err := filepath.Rel(root, source.Path)
				if err != nil {
					t.Fatal(err)
				}
				rel = filepath.ToSlash(rel)
				if seen[rel] || tc.want[rel] != source.Package {
					t.Fatalf("unexpected or duplicate source: %+v", source)
				}
				seen[rel] = true
				module := w.Modules[source.Module]
				dir, err := filepath.Rel(module.Dir, filepath.Dir(source.Path))
				if err != nil {
					t.Fatal(err)
				}
				pkg := module.Path
				if dir != "." {
					pkg += "/" + filepath.ToSlash(dir)
				}
				if source.Package != pkg {
					t.Errorf("source %s: package = %q, want %q for module %s", rel, source.Package, pkg, module.Path)
				}
			}
		})
	}
}
