package main

import "github.com/meian/rev-callgraph/testdata/app/submain"

func main() {
	caller()
	submain.Caller()
}
