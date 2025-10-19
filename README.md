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
make build

# Install to GOPATH/bin
make install
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

**Shell Prompt Behavior:**

Following the official DevContainer specification, `devgo shell` respects the container's `.bashrc` configuration and does not override the `PS1` environment variable. This allows:

- Custom prompts defined in `.bashrc` to work properly
- Git branch display and status indicators (if configured in the container)
- Full compatibility with official DevContainer images that use `__bash_prompt` or similar prompt functions

The shell is launched with `/bin/bash --login`, which ensures that `.bashrc` and other shell initialization files are properly sourced. This behavior aligns with the official DevContainer CLI's `userEnvProbe` approach.

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
- ✅ **updateRemoteUserUID** - Automatic UID/GID synchronization (Linux only)
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

## UID/GID Synchronization (Linux)

On Linux hosts, `devgo` automatically synchronizes the container user's UID/GID with your host user to prevent file ownership and permission issues when using bind mounts. This feature is critical for avoiding problems like:

- Git operations failing with "unsafe repository" errors
- File permission denied errors when creating/modifying files
- Ownership mismatches between host and container

### How It Works

When `devgo up` creates a container on Linux:

1. **User Detection**: Identifies the target container user from `remoteUser` or `containerUser` (defaults to `root`)
2. **UID/GID Retrieval**: Gets the host user's UID/GID (e.g., 1000:1000)
3. **Container Update**: Updates the container user's UID/GID to match the host user
4. **Permission Fix**: Updates ownership of the user's home directory

This happens automatically before any lifecycle commands (`onCreate`, `postCreate`, etc.) are executed, ensuring all subsequent operations have correct permissions.

### Configuration

The `updateRemoteUserUID` property controls this behavior:

```json
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "remoteUser": "vscode",
  "updateRemoteUserUID": true  // Default on Linux, no-op on Windows/macOS
}
```

**Default behavior:**
- **Linux**: `true` (enabled by default)
- **Windows/macOS**: `false` (not needed due to VM layer handling permissions)

**Disabling the Feature:**

You can disable UID/GID synchronization by setting `updateRemoteUserUID` to `false`:

```json
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "remoteUser": "vscode",
  "updateRemoteUserUID": false
}
```

**When to disable:**
- You are using a container that already handles UID/GID mapping internally
- You have custom UID/GID management scripts in your lifecycle commands
- You are intentionally using a different UID/GID inside the container

**Important considerations when disabled:**
- You may encounter "unsafe repository" errors when running git commands
- File permission issues may occur when creating or modifying files in bind-mounted directories
- You will need to manually handle file ownership and permission issues
- Consider alternative solutions like adding `git config --global --add safe.directory /workspace` in your lifecycle commands

### Important Notes

- **Root user**: UID/GID updates are never applied to the `root` user (UID must stay 0)
- **Platform-specific**: This feature only activates on Linux hosts
- **Docker Compose limitation**: Currently only supported with `image` and `dockerFile` properties, not with `dockerComposeFile`
- **Timing**: Updates occur after container creation but before any lifecycle commands

### Example Scenarios

**Scenario 1: Git repository access**
```json
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "remoteUser": "vscode"
}
```
The `vscode` user's UID/GID will be updated to match your host user, allowing seamless git operations without "unsafe repository" errors.

**Scenario 2: Node.js development**
```json
{
  "image": "node:20",
  "containerUser": "node"
}
```
The `node` user will have the correct permissions to install npm packages and access workspace files.

## Development

```bash
# Run tests
make test

# Run linter
make lint

# Development cycle
make dev

# Full CI pipeline
make ci
```

