.PHONY: test install

# TARGET="github.com/meian/go-rev-callgraph/testdata/foo.SomeStruct\#Method"
TARGET="github.com/meian/go-rev-callgraph/testdata/foo.Target"

install:
	go install

test: test-tree

test-tree: install
	cd testdata \
	    && go-rev-callgraph --format tree $(TARGET)

test-json: install
	cd testdata \
	    && go-rev-callgraph --format json $(TARGET)

test-json-edge: install
	cd testdata \
	    && go-rev-callgraph --format json --json-style edges $(TARGET)