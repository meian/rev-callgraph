package grep_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meian/rev-callgraph/internal/grep"
)

func TestSearchFiles(t *testing.T) {
	root := t.TempDir()
	// ライブラリ用フォルダ
	os.MkdirAll(filepath.Join(root, "pkg"), 0755)
	// マッチするファイル
	file1 := filepath.Join(root, "pkg", "a.go")
	os.WriteFile(file1, []byte(`package pkg
// call targetFunc()`), 0644)
	// マッチしないファイル
	file2 := filepath.Join(root, "pkg", "b.go")
	os.WriteFile(file2, []byte(`package pkg
func foo() {}`), 0644)
	// vendor 配下のファイル（マッチしても候補外）
	vendorDir := filepath.Join(root, "vendor", "pkg")
	os.MkdirAll(vendorDir, 0755)
	file3 := filepath.Join(vendorDir, "c.go")
	os.WriteFile(file3, []byte(`package pkg
// call targetFunc()`), 0644)

	files, err := grep.SearchFiles(root, "targetFunc")
	if err != nil {
		t.Fatalf("SearchFiles error: %v", err)
	}
	// file1 のみが返るべき
	expected := map[string]bool{file1: true}
	for _, f := range files {
		if !expected[f] {
			t.Errorf("unexpected file: %s", f)
		}
		expected[f] = false
	}
	for f, rem := range expected {
		if rem {
			t.Errorf("missing expected file: %s", f)
		}
	}
}
