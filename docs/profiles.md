# Global container profiles in devgo

`devgo` normally discovers a `devcontainer.json` by walking up from the current
directory. That works great for repositories that ship their own dev container,
but many repositories have none — and you may not want to commit one. **Global
profiles** let you keep named container configurations under your home directory
and run them in any directory.

## How it works

A profile is a directory under `~/.config/devgo/profiles/<name>/` containing a
`devcontainer.json` (the same format as a repo-local one). When you pass
`--profile <name>` (or set `DEVGO_PROFILE`), devgo:

1. Loads `~/.config/devgo/profiles/<name>/devcontainer.json` instead of
   searching the working tree.
2. Mounts the **current directory** as the workspace (override with
   `--workspace-folder`).
3. Names the container `<profile>-<dir>-<session>-<hash>` so profile containers
   are easy to spot and never collide with a repo-local container in the same
   directory.

The profiles directory honors `XDG_CONFIG_HOME`; when unset it defaults to
`~/.config/devgo/profiles`.

## Quick start

```bash
# Scaffold a reusable profile (creates ~/.config/devgo/profiles/go/devcontainer.json)
devgo init --profile go

# Edit the generated template to taste
$EDITOR ~/.config/devgo/profiles/go/devcontainer.json

# Use it from ANY directory
cd ~/some/repo/without/devcontainer
devgo up --profile go
devgo shell --profile go
```

Set it once for a shell session and drop the flag:

```bash
export DEVGO_PROFILE=go
devgo up        # uses the "go" profile
devgo shell     # same
```

## Precedence

When resolving which configuration to use, devgo applies this order:

1. `--config <path>` — an explicit file always wins.
2. `--profile <name>` flag.
3. `DEVGO_PROFILE` environment variable.
4. Local discovery (`.devcontainer/devcontainer.json` or `.devcontainer.json`
   walking up from the current directory).

So `--config` overrides any profile, and an explicit `--profile` flag overrides
`DEVGO_PROFILE`.

## Container naming and reuse

Profile containers are **per-workspace**: the workspace path is hashed into the
container name, so running `devgo up --profile go` in two different directories
gives you two independent containers, each mounting its own directory. This
keeps the workspace bind-mount correct and matches how repo-local containers
already behave.

| Mode | Container name |
|------|----------------|
| Local (repo `devcontainer.json`) | `<name>-<session>-<hash(workspace)>` |
| Profile | `<profile>-<name>-<session>-<hash(workspace)>` |

`<name>` is the `name` field from the profile's `devcontainer.json` (the
scaffolded template sets it to the profile name), falling back to the workspace
directory's base name.

## Errors

If you reference a profile that does not exist, devgo lists the profiles you do
have:

```
$ devgo up --profile rust
Error: profile "rust" not found; available profiles: go, node
```

When no profiles exist at all, it tells you how to create one:

```
$ devgo up --profile rust
Error: profile "rust" not found; no profiles exist yet. Create one with:
devgo init --profile rust (profiles live under /home/you/.config/devgo/profiles)
```

## Relationship to dotfiles

Profiles and [dotfiles](dotfiles.md) are complementary:

- A **profile** defines the container itself (image, build, mounts, lifecycle
  commands) for repositories that lack a `devcontainer.json`.
- **Dotfiles** layer your personal shell/editor setup on top of *any* container,
  profile-based or repo-local.

Both live under `~/.config/devgo/`, and dotfiles are applied after the profile's
lifecycle commands just as they are for repo-local containers.
