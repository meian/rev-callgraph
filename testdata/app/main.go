package main

import "github.com/meian/go-rev-callgraph/testdata/app/submain"

func main() {
	caller()
	submain.Caller()
}
