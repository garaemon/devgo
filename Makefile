.PHONY: build test lint clean install test-coverage test-integration coverage-report coverage-diff run-coverage-tests dev ci ci-full help

# Default target
.DEFAULT_GOAL := build

# Base revision for the diff coverage view: make coverage-diff BASE=origin/main
BASE ?= main

help:
	@echo "Available targets:"
	@echo "  build            - Build the binary"
	@echo "  test             - Run unit tests"
	@echo "  test-integration - Run integration tests"
	@echo "  test-coverage    - Run tests with coverage report"
	@echo "  coverage-report  - Print markdown coverage summary"
	@echo "  coverage-diff    - HTML coverage for lines changed since BASE (default main)"
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
	rm -f devgo coverage.out coverage.html coverage.tmp.html base-coverage.out

# Install binary to GOPATH/bin
install:
	go install .

# Shared by every coverage target so the test invocation lives in one place.
run-coverage-tests:
	go test -v -coverprofile=coverage.out ./...

# Run tests with coverage. The report is written to a temporary file first, so
# a failing run leaves the previous coverage.html readable.
test-coverage: run-coverage-tests
	go run ./tools/covreport -cover coverage.out -format html > coverage.tmp.html
	mv coverage.tmp.html coverage.html

# Print a markdown coverage summary (same tool CI uses for PR comments).
# Independent of test-coverage so that printing a summary never replaces a
# coverage.html the reader is still looking at.
coverage-report: run-coverage-tests
	go run ./tools/covreport -cover coverage.out

# Browse coverage for just the lines changed since BASE. The report holds both
# views; toggle "Changed only" / "All files" in the header. Deliberately not
# dependent on test-coverage, which overwrites coverage.html with the full view.
coverage-diff: run-coverage-tests
	go run ./tools/covreport -cover coverage.out -format html \
	  -diff-base '$(BASE)' > coverage.tmp.html
	mv coverage.tmp.html coverage.html

# Run integration tests
test-integration:
	cd test/integration && go test -v ./...

# Development: build and test
dev: build test

# CI: full pipeline
ci: lint test build

# CI with integration tests: full pipeline including integration tests
ci-full: lint test build test-integration