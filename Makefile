SHELL := /bin/bash

.PHONY: build run test lint

build:
	go build -o engramr ./cmd/engramr

run: build
	./engramr --help

test:
	go test ./...

lint:
	@echo "lint: TODO (hook up golangci-lint)"
