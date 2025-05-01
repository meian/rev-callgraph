package format

import (
	"fmt"
	"strings"

	"github.com/meian/rev-callgraph/internal/symbol"
)

// treePrinter はツリー形式でコールグラフを出力するプリンタです。
type treePrinter struct{}

func init() {
	printers["tree"] = func(string) Printer {
		return &treePrinter{}
	}
}

// Print はツリー形式でコールグラフを出力します。
func (p *treePrinter) Print(n *symbol.CallNode) error {
	if n == nil {
		return nil
	}
	printTree(n, 0)
	return nil
}

// printTree はCallNodeを再帰的にツリー表示します。
func printTree(n *symbol.CallNode, indent int) {
	if n == nil {
		return
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", indent))
	b.WriteString(n.Name)
	if n.Main {
		b.WriteString(" [main]")
	}
	if n.Cycled {
		b.WriteString(" (cycled)")
	}
	fmt.Println(b.String())
	for _, c := range n.Callers {
		printTree(c, indent+2)
	}
}
