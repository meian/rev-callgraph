module github.com/meian/go-rev-callgraph/testdata/app

go 1.24.1

require github.com/meian/go-rev-callgraph/testdata/qux v0.0.0-20250501155140-0b00b296e431

require (
	github.com/meian/go-rev-callgraph/testdata/bar v0.0.0-20250427081351-f7bd913afbae // indirect
	github.com/meian/go-rev-callgraph/testdata/foo v0.0.0-20250427081040-1b56eebcb9ff // indirect
)
