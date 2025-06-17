package grep_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/meian/rev-callgraph/internal/grep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchFiles(t *testing.T) {
	root := t.TempDir()
	// ライブラリ用フォルダ
	require.NoError(t, os.MkdirAll(filepath.Join(root, "pkg"), 0755))
	// マッチするファイル
	file1 := filepath.Join(root, "pkg", "a.go")
	require.NoError(t, os.WriteFile(file1, []byte(`package pkg
// call targetFunc()`), 0644))
	// マッチしないファイル
	file2 := filepath.Join(root, "pkg", "b.go")
	require.NoError(t, os.WriteFile(file2, []byte(`package pkg
func foo() {}`), 0644))
	// vendor 配下のファイル（マッチしても候補外）
	vendorDir := filepath.Join(root, "vendor", "pkg")
	require.NoError(t, os.MkdirAll(vendorDir, 0755))
	file3 := filepath.Join(vendorDir, "c.go")
	require.NoError(t, os.WriteFile(file3, []byte(`package pkg
// call targetFunc()`), 0644))

	files, err := grep.SearchFiles(context.Background(), root, "targetFunc")
	require.NoError(t, err, "SearchFiles error")
	// file1 のみが返るべき
	expected := []string{file1}
	assert.ElementsMatch(t, files, expected, "SearchFiles returned unexpected files")
}

func TestSearchFiles_ContextCancellation(t *testing.T) {
	root := t.TempDir()
	// ライブラリ用フォルダ
	os.MkdirAll(filepath.Join(root, "pkg"), 0755)
	// マッチするファイル
	file1 := filepath.Join(root, "pkg", "a.go")
	os.WriteFile(file1, []byte(`package pkg
// call targetFunc()`), 0644)

	// 即時キャンセル用のコンテキストを作成
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 直ちにキャンセル

	// SearchFiles を実行（キャンセルされたコンテキストで）
	_, err := grep.SearchFiles(ctx, root, "targetFunc")

	// キャンセルエラーが返されることを確認
	require.Error(t, err, "キャンセルエラーが期待されます")
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"context.DeadlineExceeded または context.Canceled が期待されますが、実際: %v", err)
}
