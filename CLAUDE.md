# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`devgo` is a Go CLI tool that runs Docker containers based on devcontainer.json configuration files. It automatically discovers devcontainer configurations and executes commands inside the appropriate container environment.

## Architecture

```
devgo/
├── main.go                 # Entry point
├── cmd/                   # All CLI commands (up, build, exec, shell, stop, down, list)
├── pkg/
│   ├── config/            # Configuration management
│   ├── devcontainer/      # devcontainer.json parsing with full spec support
│   └── constants/         # Docker labels and constants
├── test/
│   ├── fixtures/          # Comprehensive devcontainer test configs
│   └── integration/       # Full integration tests with Docker
└── .github/workflows/     # GitHub Actions CI/CD
```

## Development Commands

```bash
# Build the binary
make build

# Run unit tests
make test

# Run integration tests
make test-integration

# Run tests with coverage
make test-coverage

# Run linter
make lint

# Development cycle (build + test)
make dev

# Full CI pipeline
make ci

# Full CI pipeline with integration tests
make ci-full

# Install to GOPATH/bin
make install

# Show all available targets
make help
```

## Testing Strategy

- **Unit tests**: Test individual packages with mocks
- **Integration tests**: Test with actual Docker containers and sample devcontainer configs
- **CI/CD**: GitHub Actions workflow tests on Go 1.20 and 1.21

## Code Style

- Line length limit: 100 characters
- Uses golangci-lint with standard Go linters
- Standard library preferred over external dependencies where possible

## DevContainer CLI Compatibility

`devgo` aims to provide compatibility with the official devcontainer-cli commands and API. The following commands will be implemented:

### Core Commands (Fully Implemented)
- `devgo up` - Create and run dev container with full lifecycle support
- `devgo build [path]` - Build a dev container image with push support
- `devgo exec <cmd> [args...]` - Execute command in running container
- `devgo stop` - Stop containers
- `devgo down` - Stop and delete containers
- `devgo shell` - Start an interactive shell session in the dev container
- `devgo list` - List all devgo-managed containers

### Extended Commands (Fully Implemented)
- `devgo run-user-commands` - Run user commands in container (✅ implemented)
- `devgo read-configuration` - Output current workspace configuration (✅ implemented)

### Global Options
- `--help` - Show help
- `--version` - Show version number
- `--workspace-folder <path>` - Specify project directory (used across commands)

## Current Implementation Status

### Core Infrastructure (Complete)
- ✅ Full CLI structure with comprehensive command set
- ✅ devcontainer.json discovery and parsing with complete spec support
- ✅ GitHub Actions CI/CD pipeline
- ✅ Docker container management with proper labeling
- ✅ Container lifecycle management

### Commands Implementation
- ✅ **up command** - Full lifecycle support (onCreate, updateContent, postCreate, postStart, postAttach)
- ✅ **build command** - Docker and Dockerfile builds with push support
- ✅ **exec command** - Command execution in containers
- ✅ **shell command** - Interactive TTY sessions
- ✅ **stop/down commands** - Container lifecycle management
- ✅ **list command** - Container inventory management
- ✅ **read-configuration** - Output workspace configuration as JSON
- ✅ **run-user-commands** - Run lifecycle commands in existing container

### Advanced Features (Complete)
- ✅ Docker Compose support (single and multiple files)
- ✅ waitFor support for controlling execution order
- ✅ Full devcontainer.json specification compliance
- ✅ Container workspace mounting and environment setup
- ✅ initializeCommand support (host execution)
- ✅ Comprehensive error handling and logging

### Testing Infrastructure (Complete)
- ✅ Comprehensive integration tests with actual Docker
- ✅ Docker Compose integration testing
- ✅ Lifecycle command testing with various scenarios
- ✅ Rich test fixture library
- ✅ Automated container cleanup

### Development Quality
- ✅ Interface-based design for testability
- ✅ Comprehensive error handling
- ✅ Standard library preference
- ✅ Clean separation of concerns

**Current Status**: devgo is production-ready with 100% of DevContainer CLI core functionality implemented. All essential commands are fully functional and tested.
