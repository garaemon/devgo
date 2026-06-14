# Personal dotfiles support in devgo

`devgo` supports cloning a personal dotfiles repository into the dev container
and running an install script after the container is ready. This lets each
user keep their own shell setup, aliases, prompt, and editor config separate
from the team's `devcontainer.json` (which is shared via git).

## Background

`devcontainer.json` is committed to the repository and shared with the whole
team, so it is the wrong place to put personal customizations such as zsh
configuration, individual aliases, or personal CLI tools. VS Code, GitHub
Codespaces, and DevPod all solve this problem with a *dotfiles repository*
mechanism that is **not part of the official [devcontainer
spec](https://containers.dev/implementors/json_reference/)** but has become a
de-facto convention.

`devgo` follows the same convention and intentionally uses the same three
configuration keys VS Code uses (`repository`, `targetPath`,
`installCommand`), so users coming from VS Code do not need to learn anything
new.

## Configuration

### Persistent configuration file

Personal settings live in `~/.config/devgo/config.json`. The file is loaded
automatically when `devgo up` runs.

```json
{
  "dotfiles": {
    "repository": "https://github.com/your-user/dotfiles",
    "targetPath": "~/dotfiles",
    "installCommand": "install.sh"
  }
}
```

| Key | Required | Default | Description |
|-----|----------|---------|-------------|
| `repository` | yes | — | Git URL of the dotfiles repository. Any URL `git clone` accepts (https, ssh, git protocol). |
| `targetPath` | no | `~/dotfiles` | Directory inside the container where the repository is cloned. A leading `~` or `~/` is expanded to the target user's `$HOME` (resolved inside the container). The target should not be the home directory itself, since `git clone` refuses to write into a non-empty directory. |
| `installCommand` | no | (auto) | Install command to execute after cloning. The first token is the script path (relative to `targetPath`); any remaining tokens are passed to the script as arguments, e.g. `install.sh --tool`. When empty, devgo searches for a known script (see below). |

### Auto-detected install script

When `installCommand` is empty, devgo looks for the following scripts in the
cloned repository in order, and runs the first one it finds:

```
install.sh
install
bootstrap.sh
bootstrap
setup.sh
setup
```

If none are found, the dotfiles are left in `targetPath` without further
processing (clone-only mode).

### Command-line overrides

Every key can be overridden on the command line for a single `devgo up`
invocation:

```
--dotfiles-repository <url>          Override repository URL
--dotfiles-target-path <path>        Override target path
--dotfiles-install-command <script>  Override install script
--no-dotfiles                        Disable dotfiles entirely (e.g. on CI)
--force-dotfiles                     Re-clone even if targetPath already exists
```

The CLI flags take precedence over `~/.config/devgo/config.json`.

## Execution model

* Dotfiles are processed **after all lifecycle commands have completed**
  (`onCreate`, `updateContent`, `postCreate`, `postStart`, `postAttach`). Team
  setup always wins, then the personal layer goes on top. This means `devgo
  up` waits for the full lifecycle even when `waitFor` is set to an earlier
  step; the dotfiles step runs synchronously after `wg.Wait()` returns. If
  you want `up` to return faster, run with `--no-dotfiles` and apply your
  dotfiles manually later.
* `git clone` runs **inside the container**, as the target user (`remoteUser`
  if set, otherwise `containerUser`, otherwise `root`). The same user is used
  by `devgo exec` and `devgo shell`, so the cloned dotfiles end up under the
  same `$HOME` you land in interactively. SSH agent forwarding configured by
  devgo is reused, so private repositories work transparently when the host
  has the right keys loaded.
* If `targetPath` already exists, devgo **skips both clone and install** to
  avoid trampling over user changes. Use `--force-dotfiles` to override.
* The install command runs as a single `sh -c "cd <targetPath> && <installCommand>"`.
  Both the `cd` and the command execute inside the same shell, so any
  environment variables exported by the script affect only that shell. The
  command is not shell-quoted, so arguments in `installCommand` (and other
  shell syntax) are honored. Because dotfiles repositories are trusted, this
  is intentional; do not point `installCommand` at untrusted content.
* The install script must be **executable** (`chmod +x install.sh`) and have
  a shebang the container can resolve. Non-zero exit codes 126 (not
  executable) and 127 (interpreter missing) trigger an explicit hint in the
  resulting error message.
* Failures during dotfiles processing are **logged but do not fail
  `devgo up`**. A broken personal setup must never block access to the
  container.

## Examples

### Minimal: configure once, all projects use it

```
mkdir -p ~/.config/devgo
cat > ~/.config/devgo/config.json <<'EOF'
{
  "dotfiles": {
    "repository": "https://github.com/your-user/dotfiles"
  }
}
EOF

cd some-project-with-devcontainer
devgo up
```

### One-off override for a single container

```
devgo up --dotfiles-repository https://github.com/other-user/dotfiles
```

### Disable dotfiles (CI environments)

```
devgo up --no-dotfiles
```

### Re-apply dotfiles after editing the upstream repository

```
devgo up --force-dotfiles
```

## Other personal settings in the same config file

The user config file (`~/.config/devgo/config.json`) is shared by every
personal customization devgo supports. In addition to `dotfiles`, the
following top-level keys are recognized:

| Key | Default | Description |
|-----|---------|-------------|
| `shell` | `/bin/bash` | Program launched by `devgo shell`. Useful for users who prefer `zsh`, `fish`, etc. Always invoked with `-i`. |

Example with both dotfiles and a custom shell:

```json
{
  "shell": "zsh",
  "dotfiles": {
    "repository": "https://github.com/your-user/dotfiles"
  }
}
```

The `--shell` CLI flag overrides the value for a single `devgo shell`
invocation.

## What devgo intentionally does *not* do

* **Overlay the team's `devcontainer.json` itself.** Adding personal mounts,
  environment variables, or lifecycle commands is out of scope for the
  dotfiles feature. A separate `devcontainer.local.json` overlay mechanism
  may be added in a future release.
* **Manage the dotfiles repository for you.** devgo only clones and runs the
  install script. Anything else (symlinking, package installation,
  shell-switch logic) belongs in the install script inside the dotfiles
  repository.
