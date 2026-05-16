// Package dotfiles applies a personal dotfiles repository inside a running
// dev container. The configuration follows VS Code's de-facto convention:
// repository, targetPath, installCommand. See docs/dotfiles.md for details.
package dotfiles

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/garaemon/devgo/pkg/config"
)

// credentialURLPattern matches the userinfo portion of a URL of the form
// `scheme://user[:password]@host...`. Used by SanitizeRepoURL to strip
// embedded credentials before they reach logs or error messages.
var credentialURLPattern = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+\-.]*://)[^/@\s]+@`)

// DefaultTargetPath is used when neither the persistent config nor a CLI
// override specifies targetPath. The repo is cloned into a subdirectory of
// $HOME (rather than $HOME itself) because git clone refuses non-empty
// targets, and the user's home is rarely empty.
const DefaultTargetPath = "~/dotfiles"

// DefaultInstallScripts is the ordered list of script names devgo searches
// for when installCommand is unset. Mirrors the convention used by VS Code's
// dev containers extension.
var DefaultInstallScripts = []string{
	"install.sh",
	"install",
	"bootstrap.sh",
	"bootstrap",
	"setup.sh",
	"setup",
}

// Config is the resolved per-invocation dotfiles configuration.
type Config struct {
	Repository     string
	TargetPath     string
	InstallCommand string
	// Logger receives informational progress messages (clone start,
	// install completion, "already present" skips). It is invoked exactly
	// like fmt.Printf — no trailing newline is added. When nil, dotfiles
	// processing stays silent; the cmd layer wires this to a --debug-gated
	// logger so the noisy default `devgo up` output goes away.
	Logger func(format string, args ...any)
}

func (c *Config) logf(format string, args ...any) {
	if c == nil || c.Logger == nil {
		return
	}
	c.Logger(format, args...)
}

// Override holds CLI-supplied values. Empty fields fall back to the user
// config file. Strings are used (not pointers) since dotfiles values are all
// non-empty when set.
type Override struct {
	Repository     string
	TargetPath     string
	InstallCommand string
}

// Executor runs a command inside the target container as the given user. The
// implementation is provided by the caller (typically the cmd package backed
// by the Docker exec API). Tests inject a fake.
type Executor interface {
	Exec(ctx context.Context, user string, cmd []string) (stdout string, stderr string, exitCode int, err error)
}

// Resolve combines the persistent user config with CLI overrides. It returns
// nil when dotfiles processing should be skipped (disabled flag, or no
// repository configured anywhere).
func Resolve(fileCfg *config.DotfilesConfig, override Override, disabled bool) *Config {
	if disabled {
		return nil
	}

	cfg := &Config{}
	if fileCfg != nil {
		cfg.Repository = fileCfg.Repository
		cfg.TargetPath = fileCfg.TargetPath
		cfg.InstallCommand = fileCfg.InstallCommand
	}
	if override.Repository != "" {
		cfg.Repository = override.Repository
	}
	if override.TargetPath != "" {
		cfg.TargetPath = override.TargetPath
	}
	if override.InstallCommand != "" {
		cfg.InstallCommand = override.InstallCommand
	}

	if cfg.Repository == "" {
		return nil
	}
	if cfg.TargetPath == "" {
		cfg.TargetPath = DefaultTargetPath
	}
	return cfg
}

// Apply clones the dotfiles repository inside the container and runs the
// install script. It is fail-soft from the caller's perspective: errors are
// returned but the caller (cmd/up.go) is expected to log and continue.
//
// Behavior:
//   - If targetPath already exists and force is false, the function logs
//     "already present" and returns nil.
//   - If force is true and targetPath exists, the existing path is removed
//     before cloning.
//   - If force is true and targetPath does not exist, force has no effect
//     and the clone proceeds as it would otherwise.
//   - When InstallCommand is empty, devgo probes DefaultInstallScripts in
//     order and runs the first match. If none match, the clone is left in
//     place (clone-only mode).
func Apply(ctx context.Context, exec Executor, user string, cfg *Config, force bool) error {
	if cfg == nil {
		return nil
	}
	if cfg.Repository == "" {
		return fmt.Errorf("dotfiles repository is empty")
	}

	target, err := resolveHome(ctx, exec, user, cfg.TargetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target path %s: %w", cfg.TargetPath, err)
	}

	exists, err := pathExists(ctx, exec, user, target)
	if err != nil {
		return fmt.Errorf("failed to check target path %s: %w", target, err)
	}

	if exists {
		if !force {
			cfg.logf("dotfiles target %s already exists, skipping (use --force-dotfiles to overwrite)\n", target)
			return nil
		}
		if err := removePath(ctx, exec, user, target); err != nil {
			return fmt.Errorf("failed to remove existing target %s: %w", target, err)
		}
	}

	cfg.logf("Applying dotfiles from %s into %s\n", SanitizeRepoURL(cfg.Repository), target)
	if err := cloneRepo(ctx, exec, user, cfg.Repository, target); err != nil {
		return fmt.Errorf("failed to clone dotfiles: %w", err)
	}

	script, err := resolveInstallScript(ctx, exec, user, target, cfg.InstallCommand)
	if err != nil {
		return fmt.Errorf("failed to resolve install script: %w", err)
	}
	if script == "" {
		cfg.logf("dotfiles cloned; no install script found, skipping install step\n")
		return nil
	}

	if err := runInstallScript(ctx, exec, user, target, script); err != nil {
		return fmt.Errorf("install script %s failed: %w", script, err)
	}

	cfg.logf("dotfiles installed from %s\n", SanitizeRepoURL(cfg.Repository))
	return nil
}

// resolveHome replaces a leading "~" or "~/" in p with the target user's
// $HOME, queried inside the container. Paths that do not start with "~" are
// returned unchanged. The "~user" form (other-user expansion) is rejected
// explicitly so it does not silently fall through and become a literal
// directory name.
func resolveHome(ctx context.Context, exec Executor, user, p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		if strings.HasPrefix(p, "~") {
			return "", fmt.Errorf("targetPath %q uses unsupported ~user form; only ~ and ~/ are expanded", p)
		}
		return p, nil
	}
	stdout, stderr, exitCode, err := exec.Exec(ctx, user, []string{"sh", "-c", `printf %s "$HOME"`})
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return "", fmt.Errorf("failed to read $HOME: exit %d: %s", exitCode, stderr)
	}
	home := strings.TrimSpace(stdout)
	if home == "" {
		return "", fmt.Errorf("$HOME is empty for user %s", user)
	}
	if p == "~" {
		return home, nil
	}
	return home + p[1:], nil
}

func pathExists(ctx context.Context, exec Executor, user, p string) (bool, error) {
	cmd := []string{"sh", "-c", fmt.Sprintf("[ -e %s ]", shellQuote(p))}
	_, _, exitCode, err := exec.Exec(ctx, user, cmd)
	if err != nil {
		return false, err
	}
	return exitCode == 0, nil
}

func removePath(ctx context.Context, exec Executor, user, p string) error {
	cmd := []string{"sh", "-c", fmt.Sprintf("rm -rf %s", shellQuote(p))}
	_, stderr, exitCode, err := exec.Exec(ctx, user, cmd)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("rm exited with %d: %s", exitCode, stderr)
	}
	return nil
}

func cloneRepo(ctx context.Context, exec Executor, user, repo, target string) error {
	cmd := []string{"sh", "-c", fmt.Sprintf("git clone %s %s", shellQuote(repo), shellQuote(target))}
	stdout, stderr, exitCode, err := exec.Exec(ctx, user, cmd)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		// Mask credentials: git's stderr can echo back the URL on auth failure.
		safe := SanitizeRepoURL(repo)
		cleanStdout := strings.ReplaceAll(stdout, repo, safe)
		cleanStderr := strings.ReplaceAll(stderr, repo, safe)
		return fmt.Errorf("git clone %s exited with %d: stdout=%q stderr=%q", safe, exitCode, cleanStdout, cleanStderr)
	}
	return nil
}

func resolveInstallScript(ctx context.Context, exec Executor, user, targetPath, explicit string) (string, error) {
	if explicit != "" {
		// When the user pinned a script name, surface a clear "missing"
		// error rather than letting the eventual `sh -c './script'` fail
		// with exit 127 mixed into other install error paths.
		probe := explicit
		if !strings.HasPrefix(probe, "/") {
			probe = path.Join(targetPath, strings.TrimPrefix(explicit, "./"))
		}
		exists, err := pathExists(ctx, exec, user, probe)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("configured installCommand %q does not exist in dotfiles repo at %s", explicit, targetPath)
		}
		return explicit, nil
	}
	for _, candidate := range DefaultInstallScripts {
		full := path.Join(targetPath, candidate)
		exists, err := pathExists(ctx, exec, user, full)
		if err != nil {
			return "", err
		}
		if exists {
			return candidate, nil
		}
	}
	return "", nil
}

func runInstallScript(ctx context.Context, exec Executor, user, targetPath, script string) error {
	invocation := script
	if !strings.HasPrefix(script, "/") && !strings.HasPrefix(script, "./") {
		invocation = "./" + script
	}
	// Quote the script path so a value like "install.sh; rm -rf ~" is treated
	// as a single filename argument rather than a shell command sequence.
	// Anything more elaborate (shell pipelines, args) belongs inside the
	// dotfiles repo's own install script.
	cmd := []string{"sh", "-c", fmt.Sprintf("cd %s && %s", shellQuote(targetPath), shellQuote(invocation))}
	stdout, stderr, exitCode, err := exec.Exec(ctx, user, cmd)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		// Exit 126 means "found but not executable" and 127 means "not
		// found / interpreter missing". Both usually point at a missing
		// chmod +x on the script, which is the most common surprise for
		// dotfiles repos checked out from Windows.
		hint := ""
		if exitCode == 126 || exitCode == 127 {
			hint = " (hint: ensure the script is executable and its shebang interpreter exists in the container)"
		}
		return fmt.Errorf("install script exited with %d: stdout=%q stderr=%q%s", exitCode, stdout, stderr, hint)
	}
	return nil
}

// SanitizeRepoURL strips embedded credentials from an https-style URL so it
// can be safely written to logs or error messages. Non-URL strings (ssh
// shorthand, scp-style refs, local paths) are returned unchanged. The
// password (if present) is also stripped along with the username.
func SanitizeRepoURL(repo string) string {
	return credentialURLPattern.ReplaceAllString(repo, "${1}***@")
}

// shellQuote wraps a value in single quotes for safe inclusion in `sh -c`.
// Single quotes inside the value are handled by closing the quote, escaping
// the literal quote, and reopening.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
