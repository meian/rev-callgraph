package callgraph_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/meian/rev-callgraph/internal/callgraph"
	"github.com/meian/rev-callgraph/internal/gomod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallersTree_ContextCancellation(t *testing.T) {
	ctx := context.Background()
	testMod, modules, err := scanTestModules(ctx)
	require.NoError(t, err)

	// キャンセルされたコンテキストを作成
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	// CallersTree を実行（キャンセルされたコンテキストで）
	target := "github.com/meian/rev-callgraph/testdata/app.main"
	_, err = callgraph.CallersTree(cancelCtx, *testMod, target, *modules, 0, nil, 0)

	// キャンセルエラーが返されることを確認
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"context.DeadlineExceeded または context.Canceled が期待されますが、実際: %v", err)
}

func TestCallersTree_CycleDetection(t *testing.T) {
	ctx := context.Background()
	testMod, modules, err := scanTestModules(ctx)
	require.NoError(t, err)

	// サイクル検出のテスト（既にseenマップに含まれている場合）
	seen := map[string]struct{}{
		"github.com/meian/rev-callgraph/testdata/app.main": {},
	}

	result, err := callgraph.CallersTree(ctx, *testMod, "github.com/meian/rev-callgraph/testdata/app.main", *modules, 0, seen, 0)
	assert.NoError(t, err, "予期しないエラー")
	require.NotNil(t, result, "結果が nil です")
	assert.True(t, result.Cycled, "サイクル検出が期待されましたが、Cycled フラグが false です")
}

func TestCallersTree_MaxDepth(t *testing.T) {
	ctx := context.Background()
	testMod, modules, err := scanTestModules(ctx)
	require.NoError(t, err)

	// 最大深度のテスト（depth >= maxDepth の場合）
	result, err := callgraph.CallersTree(ctx, *testMod, "github.com/meian/rev-callgraph/testdata/app.main", *modules, 1, nil, 1)
	assert.NoError(t, err, "予期しないエラー")
	require.NotNil(t, result, "結果が nil です")
	// 最大深度に到達した場合、callersは空になることを確認
	assert.Empty(t, result.Callers, "最大深度に到達した場合、callersは空であることが期待されます")
}

// scanTestModules は testdata 配下の モジュールをスキャンし、テスト用の app モジュールとModuleMapを返す
func scanTestModules(ctx context.Context) (*gomod.Module, *gomod.ModuleMap, error) {
	testdataPath := filepath.Join("..", "..", "testdata")
	modPath := "github.com/meian/rev-callgraph/testdata/app"
	modules, err := gomod.Scan(ctx, testdataPath)
	if err != nil {
		return nil, nil, err
	}
	for _, mod := range modules.Iter {
		if mod.Path == modPath {
			return &mod, modules, nil
		}
	}
	return nil, modules, fmt.Errorf("%s モジュールが見つかりません", modPath)
}
