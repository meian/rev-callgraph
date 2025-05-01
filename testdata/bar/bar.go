package bar

import (
	"fmt"

	"github.com/meian/rev-callgraph/testdata/foo"
)

func Caller() {
	fmt.Println("bar.Caller")
	foo.Target()
}

func ExpressionCaller() {
	fmt.Println("bar.ExpressionCaller")
	var s foo.SomeStruct
	(*foo.SomeStruct).Method(&s)
}
