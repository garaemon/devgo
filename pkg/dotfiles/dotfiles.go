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
}

// Logf is the progress-logging callback Apply uses for informational
// messages (clone start, install completion, "already present" skips). It
// is invoked exactly like fmt.Printf — no trailing newline is added. The
// cmd layer passes a --debug-gated function so the noisy default `devgo
// up` output goes away; pass nil (or [Discard]) to silence dotfiles
// progress entirely.
type Logf func(format string, args ...any)

// Discard is a no-op [Logf]. Callers that do not need progress output (or
// tests that want silent runs) pass it to Apply so the body can call the
// logger unconditionally without nil checks.
func Discard(string, ...any) {}

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
//
// logf receives human-readable progress messages. Pass [Discard] to suppress
// them; a nil callback is treated the same way.
func Apply(ctx context.Context, exec Executor, user string, cfg *Config, force bool, logf Logf) error {
	if cfg == nil {
		return nil
	}
	if cfg.Repository == "" {
		return fmt.Errorf("dotfiles repository is empty")
	}
	if logf == nil {
		logf = Discard
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
			logf("dotfiles target %s already exists, skipping (use --force-dotfiles to overwrite)\n", target)
			return nil
		}
		if err := removePath(ctx, exec, user, target); err != nil {
			return fmt.Errorf("failed to remove existing target %s: %w", target, err)
		}
	}

	logf("Applying dotfiles from %s into %s\n", SanitizeRepoURL(cfg.Repository), target)
	if err := cloneRepo(ctx, exec, user, cfg.Repository, target); err != nil {
		return fmt.Errorf("failed to clone dotfiles: %w", err)
	}

	script, err := resolveInstallScript(ctx, exec, user, target, cfg.InstallCommand)
	if err != nil {
		return fmt.Errorf("failed to resolve install script: %w", err)
	}
	if script == "" {
		logf("dotfiles cloned; no install script found, skipping install step\n")
		return nil
	}

	if err := runInstallScript(ctx, exec, user, target, script); err != nil {
		return fmt.Errorf("install script %s failed: %w", script, err)
	}

	logf("dotfiles installed from %s\n", SanitizeRepoURL(cfg.Repository))
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
		// installCommand may carry arguments (e.g. "install.sh --tool"), so
		// only the first token is the script path; the rest are passed
		// through to the script verbatim. Probe just the script's existence
		// to surface a clear "missing" error rather than letting the eventual
		// `sh -c './script ...'` fail with exit 127 mixed into other paths.
		scriptName := firstToken(explicit)
		probe := scriptName
		if !strings.HasPrefix(probe, "/") {
			probe = path.Join(targetPath, strings.TrimPrefix(scriptName, "./"))
		}
		exists, err := pathExists(ctx, exec, user, probe)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("configured installCommand %q does not exist in dotfiles repo at %s", scriptName, targetPath)
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

func runInstallScript(ctx context.Context, exec Executor, user, targetPath, command string) error {
	// command is the resolved installCommand, which may include arguments
	// (e.g. "install.sh --tool"). It is run as a shell command line so the
	// arguments reach the script. Only the first token (the script path) is
	// adjusted with a leading "./" when it is a bare relative name. The line
	// is intentionally NOT shell-quoted: dotfiles repos are trusted, and
	// quoting the whole string would collapse arguments into one filename.
	scriptName := firstToken(command)
	invocation := command
	if !strings.HasPrefix(scriptName, "/") && !strings.HasPrefix(scriptName, "./") {
		invocation = "./" + command
	}
	cmd := []string{"sh", "-c", fmt.Sprintf("cd %s && %s", shellQuote(targetPath), invocation)}
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

// firstToken returns the first whitespace-separated field of s, which for an
// installCommand is the script path (the remainder being its arguments). It
// returns the empty string when s is blank or whitespace-only.
func firstToken(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// shellQuote wraps a value in single quotes for safe inclusion in `sh -c`.
// Single quotes inside the value are handled by closing the quote, escaping
// the literal quote, and reopening.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
