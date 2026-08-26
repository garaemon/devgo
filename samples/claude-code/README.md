# Running Claude Code in a devgo container

This sample shows how to run [Claude Code](https://claude.com/claude-code)
inside a container managed by devgo. The container reuses the Claude Code
credentials from your host, so you do not need to log in again inside the
container.

## Prerequisites

- devgo and Docker (or Podman) installed on the host.
- Claude Code installed and logged in on the host. The login stores an
  OAuth credential in `~/.claude/.credentials.json`.

## Setup

1. Copy this directory to your project, or use it as-is for a quick trial.

2. Edit `.devcontainer/devcontainer.json` and replace `/home/YOUR_USER`
   in the mount source with your host home directory. devgo does not
   expand `${localEnv:HOME}` in mounts yet.

3. Start the container from this directory:

   ```bash
   devgo up
   ```

   The `postCreateCommand` installs Claude Code with npm and prepares the
   configuration for headless use.

## Usage

Run a one-shot prompt:

```bash
devgo exec claude -p "Explain this project"
```

Or work interactively:

```bash
devgo shell
# inside the container
claude
```

Stop and remove the container when done:

```bash
devgo down
```

## How credential sharing works

The sample bind-mounts the host `~/.claude` directory to `/root/.claude`
in the container. Claude Code reads `.credentials.json` from that
directory, so the container reuses the host login session.

Two caveats to keep in mind:

- The mount is read-write. Claude Code running in the container can
  update files under your host `~/.claude` (history, settings, and
  refreshed credentials).
- The credential file grants access to your Claude account. Only use
  this setup with containers and images you trust.

If you prefer to keep the host directory untouched, copy only the
credential file into a separate directory and mount that instead:

```bash
mkdir -p ~/claude-container-home
cp ~/.claude/.credentials.json ~/claude-container-home/
```

Then point the mount source at `~/claude-container-home` (as an absolute
path) instead of `~/.claude`.

## Notes

- The `node:22` image runs as root, so the mount target is
  `/root/.claude`. If your image uses a non-root user, change the target
  to that user's home directory (e.g. `/home/node/.claude`).
- Requires devgo with support for applying `mounts` from
  devcontainer.json.
