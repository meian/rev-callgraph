package analysis

import "testing"

func TestControlStatementScopesDoNotShadowLaterImports(t *testing.T) {
	model := parseSourceForTest(t, `package app
import lib "example.com/lib"
type Local struct{}
func (Local) Target() {}
func If() {
	if lib := (Local{}); true { lib.Target() } else { lib.Target() }
	lib.Target()
}
func For() {
	for lib := (Local{}); false; { lib.Target() }
	lib.Target()
}
func Switch() {
	switch lib := (Local{}); { case true: lib.Target(); default: lib.Target() }
	lib.Target()
}
func TypeSwitch() {
	switch lib := any(Local{}).(type) { case Local: lib.Target() }
	lib.Target()
}
func Range() {
	for lib := range []Local{{}} { lib.Target() }
	lib.Target()
}
func SwitchCase() {
	switch { case true: lib := Local{}; lib.Target(); default: lib.Target() }
}
func SelectCase(ch <-chan Local) {
	select { case lib := <-ch: lib.Target(); default: lib.Target() }
}
`)
	for _, name := range []string{"If", "For", "Switch", "TypeSwitch", "Range", "SwitchCase", "SelectCase"} {
		var external, externalLine, lastLine, callCount int
		for _, fn := range model.Functions {
			if fn.Name != name {
				continue
			}
			for _, call := range fn.Calls {
				callCount++
				if call.Location.Line > lastLine {
					lastLine = call.Location.Line
				}
				if call.Name != "Target" {
					continue
				}
				if call.Package == "example.com/lib" {
					external++
					externalLine = call.Location.Line
				}
			}
		}
		if external != 1 || externalLine != lastLine || callCount < 2 {
			t.Errorf("%s: external=%d externalLine=%d lastLine=%d calls=%d; want only the final call to use the import", name, external, externalLine, lastLine, callCount)
		}
	}
}

func TestControlStatementScopesPreserveReverseEdges(t *testing.T) {
	for _, tc := range []struct{ name, statement string }{
		{"if-else", "if lib := (Local{}); true { lib.Target() } else { lib.Target() }"},
		{"nested-if", "if lib := (Local{}); true { if lib := (Local{}); true {lib.Target()}; lib.Target() }"},
		{"for", "for lib := (Local{}); false; { lib.Target() }"},
		{"switch", "switch lib := (Local{}); {case true: lib.Target()}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runFixture(t, map[string]string{
				"go.mod":     "module example.com/p\ngo 1.26\n",
				"lib/lib.go": "package lib\nfunc Target(){}",
				"p.go":       "package p\nimport \"example.com/p/lib\"\ntype Local struct{}\nfunc(Local) Target(){}\nfunc Caller(){" + tc.statement + ";lib.Target()}\nfunc Above(){Caller()}",
			}, "example.com/p/lib.Target", Options{})
			regressionEdge(t, result, "example.com/p.Caller", "example.com/p/lib.Target")
			regressionEdge(t, result, "example.com/p.Above", "example.com/p.Caller")
			if len(result.Edges) != 2 {
				t.Fatalf("local receiver mistaken for import: %+v", result.Edges)
			}
		})
	}
}
