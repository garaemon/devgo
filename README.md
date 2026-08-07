# devgo

A Go CLI tool that runs Docker or Podman containers based on devcontainer.json configuration files. `devgo` provides compatibility with the official DevContainer CLI, offering a lightweight alternative for managing development containers.

## Features

### ✅ Fully Implemented Commands

- **`devgo init`** - Initialize a new devcontainer.json template
- **`devgo up`** - Create and run dev containers with full lifecycle support
- **`devgo build`** - Build dev container images (Dockerfile and Docker Compose)
- **`devgo exec`** - Execute commands in running containers
- **`devgo shell`** - Start interactive shell sessions
- **`devgo stop`** - Stop running containers
- **`devgo down`** - Stop and remove containers
- **`devgo list`** - List all devgo-managed containers

### ✅ Advanced Features

- **Docker & Podman Runtimes** - Use Docker (default) or Podman via `--runtime`, `DEVGO_CONTAINER_RUNTIME`, or user config (see [Container Runtime](#container-runtime-docker--podman))
- **Docker Compose Support** - Single and multiple compose files
- **Lifecycle Commands** - Full support for onCreate, updateContent, postCreate, postStart, postAttach
- **initializeCommand** - Host-side command execution before container creation
- **waitFor Support** - Control execution order and dependencies
- **Container Management** - Proper labeling and workspace isolation
- **Interactive TTY** - Full terminal support for shell sessions
- **Personal customization** - Per-user dotfiles repository and shell override that stay out of the team's `devcontainer.json` (see [docs/dotfiles.md](docs/dotfiles.md))

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

### Starting from Scratch

1. **Initialize a new devcontainer configuration**:
   ```bash
   cd /path/to/your/project
   devgo init
   ```

2. **Edit the generated `.devcontainer/devcontainer.json` as needed**

3. **Start the dev container**:
   ```bash
   devgo up
   ```

### Using an Existing devcontainer.json

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

### `devgo init`

Initializes a new devcontainer.json template in the project.

```bash
devgo init [directory]

Arguments:
  directory    Target directory (optional, defaults to git root or current directory)
```

**Features:**
- Creates `.devcontainer/` directory if it doesn't exist
- Generates a basic `devcontainer.json` template
- Uses `ghcr.io/garaemon/ubuntu-noble:latest` as the default image
- Defaults to git repository root, or current directory if not in a git repo
- Optionally accepts a custom directory path

**Examples:**
```bash
# Initialize in git root directory
devgo init

# Initialize in specific directory
devgo init /path/to/project

# Initialize in current directory (when not in git repo)
devgo init .
```

**Template Contents:**

The generated template includes:
- Default Ubuntu Noble base image
- Basic VSCode customization structure
- Common configuration properties
- Ready-to-customize sections for features and extensions

### `devgo up`

Creates and starts a dev container based on the devcontainer.json configuration.

```bash
devgo up [options]

Options:
  --workspace-folder PATH                    Specify workspace directory (default: current directory)
  --dotfiles-repository URL                  Override the personal dotfiles repository for this run
  --dotfiles-target-path PATH                Override the in-container clone target (default "~/dotfiles")
  --dotfiles-install-command SCRIPT          Override the install script to run after clone
  --no-dotfiles                              Skip the dotfiles step entirely
  --force-dotfiles                           Re-clone dotfiles even if the target path already exists
```

**Features:**
- Automatically detects devcontainer.json in `.devcontainer/` or root directory
- Supports both Dockerfile builds and Docker Compose setups
- Executes lifecycle commands in proper order
- Handles container reuse if already running
- Mounts workspace and sets up environment variables
- Applies the user's personal dotfiles repository (configured in `~/.config/devgo/config.json`) after team lifecycle commands complete; see [docs/dotfiles.md](docs/dotfiles.md) for details

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
  --shell PROGRAM            Program to launch (default /bin/bash). Overrides the
                             "shell" setting in ~/.config/devgo/config.json.
  --env, -e KEY=VALUE        Set an environment variable in the shell session.
                             KEY=VALUE sets an explicit value, KEY inherits it
                             from the host environment, and PREFIX* inherits
                             every host variable starting with PREFIX.
                             May be repeated.
```

**Features:**
- Full TTY support with proper terminal handling
- Runs as the dev container's `remoteUser` (falls back to `containerUser`, then `root`), so the user matches the lifecycle commands and personal dotfiles
- Default shell is `/bin/bash`; configure a personal default in `~/.config/devgo/config.json` (`"shell": "zsh"`) or override per invocation with `--shell`
- Sets appropriate working directory
- Handles signal forwarding (Ctrl+C, etc.)
- Custom detach keys to preserve readline functionality

**Passing environment variables:**

Use `--env`/`-e` to inject environment variables into the shell session. Three forms are supported:

```bash
# Set an explicit value
devgo shell --env FOO=bar

# Inherit a value from the host environment (KEY with no '=')
devgo shell -e MY_TOKEN

# Inherit every host variable matching a prefix (PREFIX*)
devgo shell -e 'AWS_*'

# Repeat the flag to pass several variables
devgo shell -e FOO=bar -e BAZ=qux
```

Values you pass override variables already defined in the container (`containerEnv`).

**AWS SSO profile:**

When you authenticate to AWS with a named profile via SSO, the AWS CLI can export the resolved credentials as environment variables. A single `--env` value may contain several newline-separated assignments (a leading `export ` is ignored), so you can pass the command output directly:

```bash
aws sso login --profile my-sso-profile
devgo shell --env "$(aws configure export-credentials --profile my-sso-profile --format env)"
```

`aws configure export-credentials` turns the cached SSO login into temporary `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_SESSION_TOKEN` values, so the AWS CLI inside the container works without mounting `~/.aws`.

Alternatively, export the credentials into your current shell and forward them all at once with the `AWS_*` wildcard, adding `-e AWS_REGION` if you need the region:

```bash
eval "$(aws configure export-credentials --profile my-sso-profile --format env)"
devgo shell -e 'AWS_*' -e AWS_REGION
```

**Detach Keys:**

By default, Docker uses `Ctrl+P, Ctrl+Q` as detach keys, which conflicts with bash's readline command history navigation (`Ctrl+P` for previous command). To resolve this, `devgo shell` uses `Ctrl+@` (null character) as the detach key instead, allowing:

- **`Ctrl+P`** - Navigate to previous command in history (works correctly)
- **`Ctrl+N`** - Navigate to next command in history
- **`Ctrl+R`** - Reverse search through command history
- **Arrow keys** - Full cursor movement and history navigation
- **Tab** - Command and filename completion

If you need to detach from the shell session, you can press `Ctrl+@` (which typically doesn't produce a visible character on most terminals). However, since `devgo shell` is designed for interactive use, it's recommended to simply type `exit` to leave the shell session normally.

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

## Container Runtime (Docker / Podman)

`devgo` drives Docker by default but can use [Podman](https://podman.io/) instead. Podman
exposes a Docker-compatible API and CLI, so the same devcontainer.json configurations work
with either runtime.

### Selecting the runtime

The runtime is resolved in the following order of precedence (highest first):

1. The `--runtime` flag: `devgo --runtime podman up`
2. The `DEVGO_CONTAINER_RUNTIME` environment variable: `export DEVGO_CONTAINER_RUNTIME=podman`
3. The `containerRuntime` field in `~/.config/devgo/config.json`:

   ```json
   {
     "containerRuntime": "podman"
   }
   ```

4. The default, `docker`.

Accepted values are `docker` and `podman`.

### How it works

- **CLI operations** (`devgo build`, image push, and Docker Compose) invoke the selected
  binary directly (`podman build`, `podman compose`, ...).
- **API operations** (creating, starting, exec-ing, and listing containers) go through the
  Docker SDK. When Podman is selected, `devgo` automatically points the SDK at Podman's
  API socket if `DOCKER_HOST` is not already set, checking `CONTAINER_HOST`, the rootless
  socket (`$XDG_RUNTIME_DIR/podman/podman.sock` or `/run/user/<uid>/podman/podman.sock`),
  and the rootful socket (`/run/podman/podman.sock`).

### Enabling the Podman socket

The Podman API service must be running for the API operations to connect. For a rootless
setup, enable the user socket once:

```bash
systemctl --user enable --now podman.socket
```

Or start it on demand:

```bash
podman system service --time=0 &
```

If your socket lives elsewhere, point `devgo` at it explicitly with `DOCKER_HOST` (or
Podman's `CONTAINER_HOST`), which always takes precedence over auto-detection:

```bash
export DOCKER_HOST="unix:///run/user/$(id -u)/podman/podman.sock"
devgo --runtime podman up
```

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

### Test Coverage

Coverage is computed in-repo — there is no external coverage service.
`tools/covreport` reads a `go test -coverprofile` profile and renders it as
either a markdown summary or a self-contained HTML report.

```bash
# Run tests and write an HTML coverage browser to coverage.html
make test-coverage

# Print the same markdown summary CI posts on pull requests
make coverage-report

# HTML report focused on the lines changed since a revision (default: main)
make coverage-diff BASE=origin/main
```

The HTML report has a per-file sidebar and marks covered and uncovered lines
with tinted backgrounds, replacing the hard-to-read default theme of
`go tool cover -html`.

`make coverage-diff` renders only the changed hunks plus three lines of
context; the page holds both views, so the header toggle switches between
"Changed only" and "All files" without regenerating anything.

#### What the numbers mean

Both percentages are over Go *statements*, as reported by the coverage
profile — not over lines or branches.

- **Project coverage** — covered statements ÷ total statements across the
  whole module. On pull requests it is shown alongside the baseline value and
  the delta.
- **Patch coverage** — the same ratio restricted to statements on lines the
  pull request *adds*. Lines that are only deleted or moved do not count, and
  a change with no coverable added lines reports `n/a`.

Files that are not production code are excluded from both: `*_test.go`, and
anything under `test/` or `tools/`.

#### How CI reports it

On every pull request the `coverage` job runs the coverage tests on the head
commit, writes the report to the job summary, and maintains a **single sticky
comment** on the PR that is updated in place on each push. The job is
informational — it never fails on a coverage percentage. Pull requests from
forks get the job summary but no comment, since they have no write token.

#### The `coverage-data` branch

Baselines live on a dedicated orphan branch named **`coverage-data`**, which
holds nothing but coverage profiles at `profiles/<commit-sha>.out`. It carries
no project source and is never merged into `main`.

- On every push to `main`, the `record-coverage` job runs the coverage tests
  once and commits that commit's profile to `coverage-data`. Concurrent pushes
  are serialized so they cannot clobber each other, and the branch is created
  as an orphan on the first run.
- Pull request runs *read* that branch to get their baseline, so they only
  ever test their own head — the base branch is never re-tested. If the
  merge-base has no stored profile yet (for example its record run is still in
  flight), the job walks back along first-parent history up to 50 commits to
  find the nearest recorded one, and the report labels which commit it used.
- If no baseline is found at all, the report says "Baseline unavailable" and
  omits the project delta; patch coverage is unaffected.

