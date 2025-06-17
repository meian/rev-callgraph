package gomod_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/meian/rev-callgraph/internal/gomod"
	"github.com/stretchr/testify/assert"
)

func TestScan_ContextCancellation(t *testing.T) {
	// テストデータのパスを取得
	testdataPath := filepath.Join("..", "..", "testdata")

	// 即時キャンセル用のコンテキストを作成
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 直ちにキャンセル

	// Scan を実行（キャンセルされたコンテキストで）
	_, err := gomod.Scan(ctx, testdataPath)

	// キャンセルエラーが返されることを確認
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"context.DeadlineExceeded または context.Canceled が期待されますが、実際: %v", err)
}

func TestScan_NormalOperation(t *testing.T) {
	// 正常なコンテキストでテスト
	ctx := context.Background()
	testdataPath := filepath.Join("..", "..", "testdata")

	modules, err := gomod.Scan(ctx, testdataPath)
	assert.NoError(t, err, "予期しないエラー")
	assert.NotNil(t, modules, "modules が nil です")
}
