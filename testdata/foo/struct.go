package foo

import "fmt"

type SomeStruct struct{}

func (s *SomeStruct) Method() {
	fmt.Println("foo.SomeStruct#Target")
	Target()
}

func CallMethod() {
	fmt.Println("foo.CallMethod")
	var s SomeStruct
	s.Method()
}
