.PHONY: build test lint clean install test-coverage test-integration coverage-report dev ci ci-full help

# Default target
.DEFAULT_GOAL := build

help:
	@echo "Available targets:"
	@echo "  build            - Build the binary"
	@echo "  test             - Run unit tests"
	@echo "  test-integration - Run integration tests"
	@echo "  test-coverage    - Run tests with coverage report"
	@echo "  coverage-report  - Print markdown coverage summary"
	@echo "  lint             - Run linter"
	@echo "  clean            - Clean build artifacts"
	@echo "  install          - Install binary to GOPATH/bin"
	@echo "  dev              - Development: build and test"
	@echo "  ci               - CI: lint, test, and build"
	@echo "  ci-full          - CI with integration tests: full pipeline"

# Build the binary
build:
	go build -o devgo .

# Run tests
test:
	go test -v ./...

# Run linter
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -f devgo

# Install binary to GOPATH/bin
install:
	go install .

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Print a markdown coverage summary (same tool CI uses for PR comments)
coverage-report: test-coverage
	go run ./tools/covreport -cover coverage.out

# Run integration tests
test-integration:
	cd test/integration && go test -v ./...

# Development: build and test
dev: build test

# CI: full pipeline
ci: lint test build

# CI with integration tests: full pipeline including integration tests
ci-full: lint test build test-integration