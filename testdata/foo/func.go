package foo

import "fmt"

func Target() {
	fmt.Println("foo.Target")
}

func CallTarget() {
	fmt.Println("foo.CallTarget")
	Target()
}
