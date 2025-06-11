.PHONY: build test lint clean install

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

# Development: build and test
dev: build test

# CI: full pipeline
ci: lint test build