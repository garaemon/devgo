# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`devgo` is a Go CLI tool that runs Docker containers based on devcontainer.json configuration files. It automatically discovers devcontainer configurations and executes commands inside the appropriate container environment.

## Architecture

```
devgo/
├── main.go                 # Entry point
├── cmd/root.go            # CLI implementation using standard flag package
├── pkg/
│   ├── config/            # Configuration management
│   ├── devcontainer/      # devcontainer.json parsing (TODO)
│   └── docker/            # Docker container operations (TODO)
├── test/
│   ├── fixtures/          # Sample devcontainer configs (TODO)
│   └── integration/       # Integration tests (TODO)
└── .github/workflows/     # GitHub Actions CI/CD
```

## Development Commands

```bash
# Build the binary
make build

# Run tests
make test

# Run linter
make lint

# Development cycle (build + test)
make dev

# Full CI pipeline
make ci

# Install to GOPATH/bin
make install
```

## Testing Strategy

- **Unit tests**: Test individual packages with mocks
- **Integration tests**: Test with actual Docker containers and sample devcontainer configs
- **CI/CD**: GitHub Actions workflow tests on Go 1.20 and 1.21

## Code Style

- Line length limit: 100 characters
- Uses golangci-lint with standard Go linters
- Standard library preferred over external dependencies where possible

## Current Implementation Status

- ✅ Basic CLI structure with flag parsing
- ✅ devcontainer.json discovery logic
- ✅ GitHub Actions CI/CD pipeline
- ✅ devcontainer.json parser with comprehensive test coverage
- 🚧 Docker container runner (pending)
- 🚧 Integration tests (pending)
