.PHONY: test build vet fmt check examples clean

BIN ?= bin/aster

test:
	go test ./...

build:
	mkdir -p bin
	go build -trimpath -o $(BIN) ./cmd/aster

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

check: fmt test vet build

examples: build
	./$(BIN) run examples/hello.ast
	./$(BIN) run examples/fibonacci.ast
	./$(BIN) run examples/guards.ast
	./$(BIN) run examples/arrays.ast
	./$(BIN) run examples/data_processing.ast
	./$(BIN) run examples/numeric.ast

clean:
	rm -rf bin
