package submain

import (
	"fmt"

	"github.com/meian/rev-callgraph/testdata/qux"
)

func Caller() {
	fmt.Println("submain.Caller")
	qux.Caller()
}
