package contextutil

import (
	"context"
	"errors"
)

// IsCanceledOrTimedOut は ctx がキャンセルまたはタイムアウトされていれば true を返します
func IsCanceledOrTimedOut(ctx context.Context) bool {
	err := ctx.Err()
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
