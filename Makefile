.PHONY: build test lint clean install test-coverage test-integration \
	coverage-report coverage-diff dev ci ci-full help

# Default target
.DEFAULT_GOAL := build

# Base revision for the diff coverage view: make coverage-diff BASE=origin/main
BASE ?= main

# Lines of context shown around each changed line in that view
CONTEXT ?= 3

help:
	@echo "Available targets:"
	@echo "  build            - Build the binary"
	@echo "  test             - Run unit tests"
	@echo "  test-integration - Run integration tests"
	@echo "  test-coverage    - Run tests with coverage report"
	@echo "  coverage-report  - Print markdown coverage summary"
	@echo "  coverage-diff    - HTML coverage for lines changed since BASE (default main),"
	@echo "                     with CONTEXT lines around each change (default 3)"
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
	go run ./tools/covreport -cover coverage.out -format html > coverage.html

# Print a markdown coverage summary (same tool CI uses for PR comments)
coverage-report: test-coverage
	go run ./tools/covreport -cover coverage.out

# Browse coverage for just the lines changed since BASE. The report holds both
# views; toggle "Changed only" / "All files" in the header. Deliberately not
# dependent on test-coverage, which overwrites coverage.html with the full view.
coverage-diff:
	go test -coverprofile=coverage.out ./...
	go run ./tools/covreport -cover coverage.out -format html \
	  -diff-base $(BASE) -diff-context $(CONTEXT) > coverage.html

# Run integration tests
test-integration:
	cd test/integration && go test -v ./...

# Development: build and test
dev: build test

# CI: full pipeline
ci: lint test build

# CI with integration tests: full pipeline including integration tests
ci-full: lint test build test-integration