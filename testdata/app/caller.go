package main

import (
	"github.com/meian/rev-callgraph/testdata/app/submain"
)

func caller() {
	println("main.Caller")
	submain.Caller()
}

func Recurse() {
	println("main.Recurse")
	submain.Caller()
	Recurse()
}
