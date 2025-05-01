package qux

import (
	"fmt"

	"github.com/meian/rev-callgraph/testdata/bar"
)

func Caller() {
	fmt.Println("qux.Caller")
	bar.Caller()
}
