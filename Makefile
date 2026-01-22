SHELL := /bin/bash

.PHONY: build run test test-all coverage coverage-html integration deps-up deps-down lint

build:
	go build -o bin/engramr ./cmd/engramr

run: build
	bin/engramr --help

test:
	go test ./...

lint:
	@echo "lint: TODO (hook up golangci-lint)"

# Runs unit + integration tests (requires Qdrant + Ollama running)
test-all: deps-up
	go test -tags=integration ./...

# Starts dependencies in background
deps-up:
	docker compose up -d

deps-down:
	docker compose down

# Coverage for unit tests only (fast, CI-friendly)
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Coverage including integration tests (requires deps)
coverage-all: deps-up
	go test -tags=integration -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

coverage-html:
	go tool cover -html=coverage.out

# Run only integration tests (useful for iteration)
integration: deps-up
	go test -tags=integration ./... -run Integration

# Optional: pull embedding model (do once, or when you change model)
model-pull: deps-up
	docker compose exec ollama ollama pull nomic-embed-text
