package contextutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/meian/rev-callgraph/internal/contextutil"
	"github.com/stretchr/testify/assert"
)

func TestIsCanceledOrTimedOut(t *testing.T) {
	t.Run("not done", func(t *testing.T) {
		ctx := context.Background()
		assert.False(t, contextutil.IsCanceledOrTimedOut(ctx))
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.True(t, contextutil.IsCanceledOrTimedOut(ctx))
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		// テスト結果が不安定にならないように長めにディレイを取っておく
		time.Sleep(200 * time.Millisecond)
		assert.True(t, contextutil.IsCanceledOrTimedOut(ctx))
	})
}
