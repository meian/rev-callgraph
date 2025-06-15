package callgraph_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/meian/rev-callgraph/internal/callgraph"
	"github.com/meian/rev-callgraph/internal/gomod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallersTree_ContextCancellation(t *testing.T) {
	// テストデータのセットアップ
	testdataPath := filepath.Join("..", "..", "testdata")
	ctx := context.Background()

	modules, err := gomod.Scan(ctx, testdataPath)
	require.NoError(t, err, "モジュールスキャンに失敗しました")

	// テスト用のモジュールを取得
	var testMod gomod.Module
	for _, mod := range modules.Iter {
		if mod.Path == "testdata/app" {
			testMod = mod
			break
		}
	}
	if testMod.Path == "" {
		t.Skip("testdata/app モジュールが見つかりません")
	}

	// 短いタイムアウトでコンテキストを作成
	cancelCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// わずかな遅延を入れてキャンセルを確実にする
	time.Sleep(2 * time.Millisecond)

	// CallersTree を実行（キャンセルされたコンテキストで）
	target := "testdata/app.main"
	_, err = callgraph.CallersTree(cancelCtx, testMod, target, *modules, 0, nil, 0)

	// キャンセルエラーが返されることを確認
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"context.DeadlineExceeded または context.Canceled が期待されますが、実際: %v", err)
}

func TestCallersTree_CycleDetectionWithContext(t *testing.T) {
	testdataPath := filepath.Join("..", "..", "testdata")
	ctx := context.Background()

	modules, err := gomod.Scan(ctx, testdataPath)
	require.NoError(t, err, "モジュールスキャンに失敗しました")

	var testMod gomod.Module
	for _, mod := range modules.Iter {
		if mod.Path == "testdata/app" {
			testMod = mod
			break
		}
	}
	if testMod.Path == "" {
		t.Skip("testdata/app モジュールが見つかりません")
	}

	// サイクル検出のテスト（既にseenマップに含まれている場合）
	seen := map[string]struct{}{
		"testdata/app.main": {},
	}

	result, err := callgraph.CallersTree(ctx, testMod, "testdata/app.main", *modules, 0, seen, 0)
	assert.NoError(t, err, "予期しないエラー")
	require.NotNil(t, result, "結果が nil です")
	assert.True(t, result.Cycled, "サイクル検出が期待されましたが、Cycled フラグが false です")
}

func TestCallersTree_MaxDepthWithContext(t *testing.T) {
	testdataPath := filepath.Join("..", "..", "testdata")
	ctx := context.Background()

	modules, err := gomod.Scan(ctx, testdataPath)
	require.NoError(t, err, "モジュールスキャンに失敗しました")

	var testMod gomod.Module
	for _, mod := range modules.Iter {
		if mod.Path == "testdata/app" {
			testMod = mod
			break
		}
	}
	if testMod.Path == "" {
		t.Skip("testdata/app モジュールが見つかりません")
	}

	// 最大深度のテスト（depth >= maxDepth の場合）
	result, err := callgraph.CallersTree(ctx, testMod, "testdata/app.main", *modules, 1, nil, 1)
	assert.NoError(t, err, "予期しないエラー")
	require.NotNil(t, result, "結果が nil です")
	// 最大深度に到達した場合、callersは空になることを確認
	assert.Empty(t, result.Callers, "最大深度に到達した場合、callersは空であることが期待されます")
}
