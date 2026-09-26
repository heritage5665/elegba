# Makefile

.PHONY: build test lint run clean

CONFIG ?= examples/elegba.yaml

build:
	go build -o elegba ./cmd/elegba

test:
	go test -race -cover ./...

lint:
	golangci-lint run

run: build
	./elegba --config "$(CONFIG)"

clean:
	go clean
	rm -f elegba