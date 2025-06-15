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
		assert.False(t, contextutil.IsCanceledOrTimedOut(ctx), "キャンセルまたはタイムアウトされた")
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.True(t, contextutil.IsCanceledOrTimedOut(ctx), "キャンセルされていない")
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		assert.Eventually(t, func() bool {
			return contextutil.IsCanceledOrTimedOut(ctx)
		}, 5*time.Second, time.Millisecond, "タイムアウトしなかった")
	})

	t.Run("deadline not exceeded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		assert.False(t, contextutil.IsCanceledOrTimedOut(ctx), "キャンセルまたはタイムアウトされた")
	})
}
