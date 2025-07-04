.PHONY: test install

# TARGET="github.com/meian/rev-callgraph/testdata/foo.SomeStruct\#Method"
TARGET="github.com/meian/rev-callgraph/testdata/foo.Target"

install:
	go install

test: test-tree

test-tree: install
	cd testdata \
	    && rev-callgraph --format tree "$(TARGET)"

test-tree-progress: install
	cd testdata \
	    && rev-callgraph --format tree "$(TARGET)" --progress

test-json: install
	cd testdata \
	    && rev-callgraph --format json "$(TARGET)"

test-json-edge: install
	cd testdata \
	    && rev-callgraph --format json --json-style edges "$(TARGET)"