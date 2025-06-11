package progress

import (
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"
)

const textColor = color.FgYellow

var (
	print   = color.New(textColor).FprintFunc()
	println = color.New(textColor).FprintlnFunc()
)

// Messenger は進行過程のメッセージを出力するための構造体です。
type Messenger struct {
	w io.Writer
}

func NewMessenger(w io.Writer) *Messenger {
	return &Messenger{w: w}
}

// Msg はメッセージを出力します。
func (m *Messenger) Msg(msg string) {
	if len(msg) == 0 {
		return
	}
	msg = fmt.Sprintf("[%s]%s", time.Now().Format("15:04:05"), msg)
	if msg[len(msg)-1] != '\n' {
		println(m.w, msg)
		return
	}
	print(m.w, msg)
}

// Msgf はフォーマットされたメッセージを出力します。
func (m *Messenger) Msgf(format string, args ...any) {
	if len(format) == 0 {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	m.Msg(msg)
}
