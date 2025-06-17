package progress

import (
	"context"
)

type messengerKey struct{}

// WithProgress はコンテキストに Messenger を追加します。
func WithProgress(ctx context.Context, m *Messenger) context.Context {
	return context.WithValue(ctx, messengerKey{}, m)
}

// Msg はコンテキストに紐づけられた Messenger にメッセージを出力します。
// Messenger が存在しない場合は何もしません。
func Msg(ctx context.Context, msg string) {
	m, ok := ctx.Value(messengerKey{}).(*Messenger)
	if !ok || m == nil {
		return
	}
	m.Msg(msg)
}

// Msgf はコンテキストに紐づけられた Messenger にフォーマットされたメッセージを出力します。
// Messenger が存在しない場合は何もしません。
func Msgf(ctx context.Context, format string, args ...any) {
	m, ok := ctx.Value(messengerKey{}).(*Messenger)
	if !ok || m == nil {
		return
	}
	m.Msgf(format, args...)
}
