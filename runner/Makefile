.PHONY: build run test help

BINARY_NAME=benchmark_runner

build:
	go build -o bin/$(BINARY_NAME) .

run: build
	./bin/$(BINARY_NAME) -help

test:
	go test ./...

help:
	@echo "Available targets:"
	@echo "  build         - Build the runner"
	@echo "  run           - Build and show runner help"
	@echo "  test          - Run tests"
