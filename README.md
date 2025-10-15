# devgo

A Go CLI tool that runs Docker containers based on devcontainer.json configuration files. `devgo` provides compatibility with the official DevContainer CLI, offering a lightweight alternative for managing development containers.

## Features

### ✅ Fully Implemented Commands

- **`devgo up`** - Create and run dev containers with full lifecycle support
- **`devgo build`** - Build dev container images (Dockerfile and Docker Compose)
- **`devgo exec`** - Execute commands in running containers
- **`devgo shell`** - Start interactive shell sessions
- **`devgo stop`** - Stop running containers
- **`devgo down`** - Stop and remove containers
- **`devgo list`** - List all devgo-managed containers

### ✅ Advanced Features

- **Docker Compose Support** - Single and multiple compose files
- **Lifecycle Commands** - Full support for onCreate, updateContent, postCreate, postStart, postAttach
- **initializeCommand** - Host-side command execution before container creation
- **waitFor Support** - Control execution order and dependencies
- **Container Management** - Proper labeling and workspace isolation
- **Interactive TTY** - Full terminal support for shell sessions

### ❌ Not Yet Implemented

- `devgo run-user-commands` - Run user-defined commands in containers
- `devgo read-configuration` - Output workspace configuration

## Installation

### Option 1: Install from GitHub (Recommended)

```bash
# Install the latest version directly from GitHub
go install github.com/garaemon/devgo@latest
```

### Option 2: Build from Source

```bash
# Clone the repository
git clone https://github.com/garaemon/devgo.git
cd devgo

# Build the binary
bazel build //:devgo

# The binary will be at bazel-bin/devgo_/devgo
# You can copy it to your PATH
sudo cp bazel-bin/devgo_/devgo /usr/local/bin/devgo
```

### Verify Installation

```bash
# Check if devgo is installed correctly
devgo --version

# Show help
devgo --help
```

## Quick Start

1. **Navigate to a project with a devcontainer.json**:
   ```bash
   cd /path/to/your/project
   ```

2. **Start the dev container**:
   ```bash
   devgo up
   ```

3. **Execute commands in the container**:
   ```bash
   devgo exec -- npm install
   devgo exec -- go build
   ```

4. **Open an interactive shell**:
   ```bash
   devgo shell
   ```

5. **Stop the container when done**:
   ```bash
   devgo down
   ```

## Command Reference

### `devgo up`

Creates and starts a dev container based on the devcontainer.json configuration.

```bash
devgo up [options]

Options:
  --workspace-folder PATH    Specify workspace directory (default: current directory)
```

**Features:**
- Automatically detects devcontainer.json in `.devcontainer/` or root directory
- Supports both Dockerfile builds and Docker Compose setups
- Executes lifecycle commands in proper order
- Handles container reuse if already running
- Mounts workspace and sets up environment variables

### `devgo build`

Builds a dev container image without starting it.

```bash
devgo build [options] [path]

Options:
  --workspace-folder PATH    Specify workspace directory
  --push                     Push built image to registry
```

**Features:**
- Supports Dockerfile builds with build arguments
- Handles Docker Compose image builds
- Optional registry push functionality

### `devgo exec`

Executes commands inside the running dev container.

```bash
devgo exec [options] -- <command> [args...]

Options:
  --workspace-folder PATH    Specify workspace directory
```

**Examples:**
```bash
devgo exec -- ls -la
devgo exec -- npm test
devgo exec -- bash -c "echo 'Hello from container'"
```

### `devgo shell`

Starts an interactive shell session in the dev container.

```bash
devgo shell [options]

Options:
  --workspace-folder PATH    Specify workspace directory
```

**Features:**
- Full TTY support with proper terminal handling
- Runs as the configured container user
- Sets appropriate working directory
- Handles signal forwarding (Ctrl+C, etc.)

### `devgo list`

Lists all containers managed by devgo.

```bash
devgo list [options]

Options:
  --workspace-folder PATH    Filter by workspace directory
```

**Output includes:**
- Container name and status
- Associated workspace path
- Image information
- Creation timestamp

### `devgo stop`

Stops running dev containers without removing them.

```bash
devgo stop [options]

Options:
  --workspace-folder PATH    Specify workspace directory
```

### `devgo down`

Stops and removes dev containers and associated resources.

```bash
devgo down [options]

Options:
  --workspace-folder PATH    Specify workspace directory
```

**Features:**
- Graceful container shutdown
- Removes containers and associated networks
- Preserves volumes and images

## DevContainer Configuration Support

### Supported Properties

- ✅ **image** - Base container image
- ✅ **dockerFile** - Custom Dockerfile builds
- ✅ **dockerComposeFile** - Docker Compose setups (single/multiple files)
- ✅ **service** - Target service in compose files
- ✅ **runServices** - Additional services to start
- ✅ **workspaceFolder** - Container workspace path
- ✅ **workspaceMount** - Custom workspace mounting
- ✅ **mounts** - Additional volume mounts
- ✅ **containerEnv** - Environment variables
- ✅ **remoteUser** - Container user configuration
- ✅ **initializeCommand** - Host-side initialization
- ✅ **onCreateCommand** - Post-creation commands
- ✅ **updateContentCommand** - Content update commands
- ✅ **postCreateCommand** - Post-creation setup
- ✅ **postStartCommand** - Post-start commands
- ✅ **postAttachCommand** - Post-attach commands
- ✅ **waitFor** - Command execution dependencies

### Lifecycle Command Execution Order

1. **initializeCommand** (on host, before container creation)
2. **onCreateCommand** (first time container is created)
3. **updateContentCommand** (when content changes)
4. **postCreateCommand** (after creation/update)
5. **postStartCommand** (when container starts)
6. **postAttachCommand** (when attaching to container)

## Docker Compose Support

`devgo` fully supports Docker Compose-based dev containers:

```json
{
  "dockerComposeFile": ["docker-compose.yml", "docker-compose.dev.yml"],
  "service": "app",
  "runServices": ["database", "redis"],
  "workspaceFolder": "/workspace"
}
```

**Features:**
- Multiple compose files
- Service dependencies
- Automatic network creation
- Volume management

## Development

```bash
# Build the project
bazel build //:devgo

# Run all tests
bazel test //...

# Run specific package tests
bazel test //cmd:all
bazel test //pkg/config:config_test
bazel test //pkg/devcontainer:devcontainer_test

# Run linter (you need to set up golangci-lint separately)
golangci-lint run

# Update dependencies from go.mod
bazel run //:gazelle-update-repos
```

## Maintenance

### Updating Go Dependencies

When you add, remove, or update Go dependencies in `go.mod`, you need to sync them with Bazel:

```bash
# 1. Update go.mod and go.sum
go get github.com/example/package@v1.2.3

# 2. Run go mod tidy
go mod tidy

# 3. Update Bazel's deps.bzl from go.mod
bazel run //:gazelle-update-repos

# 4. Verify the build still works
bazel build //...
bazel test //...
```

### Adding New Go Files or Packages

When you add new `.go` files or create new packages, update the BUILD files:

```bash
# Option 1: Update all BUILD files automatically (recommended)
bazel run //:gazelle

# Option 2: Update specific package
bazel run //:gazelle -- update pkg/newpackage

# After updating, test your changes
bazel test //...
```

### Updating Bazel Version

The Bazel version is pinned in `.bazelversion`:

```bash
# Edit .bazelversion with the desired version
echo "7.5.0" > .bazelversion

# Test with the new version
bazel build //...
bazel test //...
```

### Updating Bazel Rules (rules_go, gazelle, etc.)

To update Bazel rules, edit `WORKSPACE`:

```python
# Find the current rules_go version
http_archive(
    name = "io_bazel_rules_go",
    sha256 = "...",
    urls = [
        "https://github.com/bazelbuild/rules_go/releases/download/v0.XX.X/rules_go-v0.XX.X.zip",
    ],
)
```

Steps:
1. Check for new versions at https://github.com/bazelbuild/rules_go/releases
2. Update the version, URLs, and sha256 in `WORKSPACE`
3. Update gazelle similarly if needed
4. Test thoroughly: `bazel test //...`

### Regenerating All Bazel Build Files

If you need to completely regenerate BUILD files:

```bash
# This will update all BUILD.bazel files based on your Go code
bazel run //:gazelle

# If you have issues, try cleaning first
bazel clean --expunge
bazel run //:gazelle
bazel build //...
```

### Common Maintenance Tasks

```bash
# Clean Bazel cache (if you have build issues)
bazel clean

# Complete clean (removes all Bazel files)
bazel clean --expunge

# Check for issues with external dependencies
bazel sync

# Run all tests with verbose output
bazel test --test_output=all //...

# Build everything
bazel build //...
```

