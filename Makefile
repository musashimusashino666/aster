.PHONY: test build vet fmt fmt-check check examples clean

BIN ?= bin/aster
GO_FILES = $(shell find . -type f -name '*.go' -not -path './.git/*')

test:
	go test ./...

build:
	mkdir -p bin
	go build -trimpath -o $(BIN) ./cmd/aster

vet:
	go vet ./...

fmt:
	gofmt -w $(GO_FILES)

fmt-check:
	@unformatted="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$unformatted" ]; then \
		printf 'Go files need formatting:\n%s\n' "$$unformatted"; \
		gofmt -d $(GO_FILES); \
		exit 1; \
	fi
check: fmt-check test vet build

examples: build
	./$(BIN) run examples/hello.ast
	./$(BIN) run examples/fibonacci.ast
	./$(BIN) run examples/guards.ast
	./$(BIN) run examples/arrays.ast
	./$(BIN) run examples/data_processing.ast
	./$(BIN) run examples/numeric.ast

clean:
	rm -rf bin
