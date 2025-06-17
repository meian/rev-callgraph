package astquery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/meian/rev-callgraph/internal/gomod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractCallers_ContextCancellation(t *testing.T) {
	// テスト用のGoファイルを作成
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	err := os.WriteFile(testFile, []byte(`package test

func TestFunction() {
	targetFunc()
}

func targetFunc() {
	// target function
}
`), 0644)
	require.NoError(t, err, "テストファイル作成に失敗")

	// モジュールマップの作成
	modules := gomod.NewModuleMap(map[string]gomod.Module{
		"test": {
			Path: "test",
			Root: tmpDir,
		},
	})

	files := []string{testFile}

	// 即時キャンセル用のコンテキストを作成
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 直ちにキャンセル

	// ExtractCallers を実行（キャンセルされたコンテキストで）
	_, err = ExtractCallers(ctx, "test.targetFunc", files, *modules)

	// キャンセルエラーが返されることを確認
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"context.DeadlineExceeded または context.Canceled が期待されますが、実際: %v", err)
}

func TestExtractCallers_NormalOperation(t *testing.T) {
	// 正常なケースのテスト
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	require.NoError(t, os.WriteFile(testFile, []byte(`package test

func TestFunction() {
	targetFunc()
}

func targetFunc() {
	// target function
}
`), 0644))

	modules := gomod.NewModuleMap(map[string]gomod.Module{
		"test": {
			Path: "test",
			Root: tmpDir,
		},
	})

	files := []string{testFile}
	ctx := context.Background()

	callers, err := ExtractCallers(ctx, "test.targetFunc", files, *modules)
	assert.NoError(t, err, "予期しないエラー")

	// targetFunc を呼び出しているのは TestFunction なので、呼び出し元が1つ見つかるはず
	assert.Len(t, callers, 1, "呼び出し元が見つかりませんでした")
}
