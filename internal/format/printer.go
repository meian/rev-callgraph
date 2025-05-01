package format

import (
	"fmt"

	"github.com/meian/rev-callgraph/internal/symbol"
)

// Printer はコールグラフ出力の共通インターフェイスです。
type Printer interface {
	// Print はコールグラフを出力します。
	Print(root *symbol.CallNode) error
}

type printerGen func(jsonStyle string) Printer

var printers = map[string]printerGen{}

// NewPrinter はformatに応じたPrinterを返します。
func NewPrinter(format, jsonStyle string) (Printer, error) {
	gen, ok := printers[format]
	if !ok {
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
	return gen(jsonStyle), nil
}

// type jsonPrinter struct{}
// type dotPrinter struct{}

// func (p *jsonPrinter) Print(n *symbol.CallNode) error {
// 	fmt.Println("[json出力は未実装]")
// 	return nil
// }
// func (p *dotPrinter) Print(n *symbol.CallNode) error {
// 	fmt.Println("[dot出力は未実装]")
// 	return nil
// }
