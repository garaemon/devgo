package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/garaemon/devgo/pkg/config"
	"github.com/garaemon/devgo/pkg/devcontainer"
	"golang.org/x/term"
)

// DefaultShell is used when neither --shell nor user config provides a value.
const DefaultShell = "/bin/bash"

// resolveShellCommand returns the command to run for `devgo shell`. The
// resolution order is: --shell flag > user config > DefaultShell. The shell
// is always launched with -i for interactive mode.
func resolveShellCommand(override string, userConfig *config.UserConfig) []string {
	shell := DefaultShell
	if userConfig != nil && userConfig.Shell != "" {
		shell = userConfig.Shell
	}
	if override != "" {
		shell = override
	}
	return []string{shell, "-i"}
}

func runShellCommand(args []string) error {
	devcontainerPath, err := findDevcontainerConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to find devcontainer config: %w", err)
	}

	workspaceDir := determineWorkspaceFolder(devcontainerPath)

	devContainer, err := devcontainer.Parse(devcontainerPath)
	if err != nil {
		return fmt.Errorf("failed to parse devcontainer.json: %w", err)
	}

	containerName := determineContainerName(devContainer, workspaceDir)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			warnf("failed to close Docker client: %v", closeErr)
		}
	}()

	userConfig, err := config.LoadUserConfig()
	if err != nil {
		warnf("failed to load user config: %v", err)
		userConfig = &config.UserConfig{}
	}
	shellCommand := resolveShellCommand(shellOverride, userConfig)

	ctx := context.Background()
	return executeInteractiveShell(ctx, cli, containerName, devContainer, shellCommand, shellEnvVars)
}

// resolveEnvVars parses --env/-e entries into a map of variables. A single
// entry may contain several newline-separated assignments, and a leading
// "export " on each line is ignored, so the output of
// `aws configure export-credentials --format env` can be passed verbatim:
//
//	devgo shell --env "$(aws configure export-credentials --format env)"
//
// Each line is one of:
//
//	KEY=VALUE   set an explicit value
//	KEY         inherit the value from the host environment (skipped if unset)
//	PREFIX*     inherit every host variable whose name starts with PREFIX
//
// For the KEY=VALUE form the value is preserved exactly (including embedded and
// trailing whitespace); only the key side is trimmed, since environment
// variable names never contain whitespace. Later assignments to the same key
// win.
func resolveEnvVars(entries []string) map[string]string {
	resolved := make(map[string]string)
	for _, entry := range entries {
		for _, line := range strings.Split(entry, "\n") {
			// Drop a trailing CR so CRLF input is handled, but otherwise keep
			// the line intact so values retain their internal/trailing spaces.
			line = strings.TrimSuffix(line, "\r")

			if idx := strings.IndexByte(line, '='); idx >= 0 {
				key := strings.TrimSpace(line[:idx])
				key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
				if key != "" {
					resolved[key] = line[idx+1:]
				}
				continue
			}

			// No '=': an inherit (KEY) or wildcard (PREFIX*) form. These are
			// typed by the user, so trimming surrounding whitespace is safe.
			e := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
			switch {
			case e == "":
				continue
			case strings.HasSuffix(e, "*"):
				prefix := strings.TrimSuffix(e, "*")
				for _, kv := range os.Environ() {
					idx := strings.IndexByte(kv, '=')
					if idx < 0 {
						continue
					}
					if key := kv[:idx]; strings.HasPrefix(key, prefix) {
						resolved[key] = kv[idx+1:]
					}
				}
			default:
				if val, ok := os.LookupEnv(e); ok {
					resolved[e] = val
				}
			}
		}
	}
	return resolved
}

// buildShellEnv builds the environment slice passed to the shell exec. It
// starts from the container's resolved environment, defaults TERM to
// xterm-256color, then overlays user-supplied --env entries (which override
// container values).
func buildShellEnv(expandedEnv map[string]string, extraEnv []string) []string {
	merged := map[string]string{"TERM": "xterm-256color"}
	for k, v := range expandedEnv {
		merged[k] = v
	}
	for k, v := range resolveEnvVars(extraEnv) {
		merged[k] = v
	}

	env := make([]string, 0, len(merged))
	for k, v := range merged {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

func executeInteractiveShell(ctx context.Context, cli DockerExecClient, containerName string, devContainer *devcontainer.DevContainer, shellCommand []string, extraEnv []string) error {
	containerID, err := findRunningContainer(ctx, cli, containerName)
	if err != nil {
		return fmt.Errorf("failed to find running container: %w", err)
	}

	if containerID == "" {
		return fmt.Errorf("container '%s' is not running. Use 'devgo up' to start it first", containerName)
	}

	// Get base environment variables from running container
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	baseEnv := make(map[string]string)
	for _, e := range inspect.Config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			baseEnv[parts[0]] = parts[1]
		}
	}

	expandedEnv := devContainer.GetContainerEnv(baseEnv)
	// TERM defaults to xterm-256color; user-supplied --env entries override
	// container values.
	env := buildShellEnv(expandedEnv, extraEnv)

	user := devContainer.GetTargetUser()
	workspaceFolder := devContainer.GetWorkspaceFolder()

	// Get terminal size before creating exec
	stdinFd := int(os.Stdin.Fd())
	var consoleSize *[2]uint
	if term.IsTerminal(stdinFd) {
		width, height, err := term.GetSize(stdinFd)
		if err == nil {
			consoleSize = &[2]uint{uint(height), uint(width)}
			debugf("Terminal size: %dx%d (cols x rows)\n", width, height)
		}
	}

	execConfig := container.ExecOptions{
		User:         user,
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          shellCommand,
		WorkingDir:   workspaceFolder,
		Env:          env,
		ConsoleSize:  consoleSize,
		DetachKeys:   "ctrl-@", // Use ctrl-@ instead of default ctrl-p,ctrl-q to allow ctrl-p for history
	}

	debugln("Creating exec instance with config:")
	debugf("  User: %s\n", user)
	debugf("  Tty: %v\n", execConfig.Tty)
	debugf("  AttachStdin: %v\n", execConfig.AttachStdin)
	debugf("  AttachStdout: %v\n", execConfig.AttachStdout)
	debugf("  AttachStderr: %v\n", execConfig.AttachStderr)
	debugf("  Cmd: %v\n", execConfig.Cmd)
	debugf("  Env: %v\n", execConfig.Env)
	debugf("  DetachKeys: %s (allows ctrl-p for history)\n", execConfig.DetachKeys)
	if consoleSize != nil {
		debugf("  ConsoleSize: %dx%d\n", consoleSize[1], consoleSize[0])
	}
	debugf("  WorkingDir: %s\n", execConfig.WorkingDir)
	debugf("  Container ID: %s\n", containerID)

	execCreateResp, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec instance: %w", err)
	}

	debugf("Exec instance created with ID: %s\n", execCreateResp.ID)

	// Check if stdin is a terminal and set raw mode
	var oldState *term.State
	if term.IsTerminal(stdinFd) {
		debugf("Setting terminal to raw mode (fd: %d)\n", stdinFd)
		oldState, err = term.MakeRaw(stdinFd)
		if err != nil {
			return fmt.Errorf("failed to set terminal to raw mode: %w", err)
		}
		debugln("Terminal set to raw mode successfully")
		defer func() {
			if restoreErr := term.Restore(stdinFd, oldState); restoreErr != nil {
				warnf("failed to restore terminal: %v", restoreErr)
			}
		}()
	} else {
		debugf("Warning: stdin is not a terminal (fd: %d)\n", stdinFd)
	}

	// Handle signals to restore terminal state
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		if oldState != nil {
			_ = term.Restore(stdinFd, oldState)
		}
		os.Exit(0)
	}()

	// Attach to the exec instance to get HijackedResponse
	debugf("Attaching to exec instance %s\n", execCreateResp.ID)
	execAttachResp, err := cli.ContainerExecAttach(ctx, execCreateResp.ID, container.ExecAttachOptions{
		Tty: true,
	})
	if err != nil {
		return fmt.Errorf("failed to attach to exec instance: %w", err)
	}
	defer execAttachResp.Close()
	debugln("Successfully attached to exec instance")

	// Start the exec instance in a separate goroutine
	// This must be done AFTER attach and runs concurrently with I/O
	debugln("Starting exec instance in background")
	go func() {
		startErr := cli.ContainerExecStart(ctx, execCreateResp.ID, container.ExecStartOptions{
			Tty: true,
		})
		if startErr != nil {
			debugf("ExecStart error: %v\n", startErr)
		} else {
			debugln("ExecStart completed")
		}
	}()

	// Handle TTY I/O
	debugln("Starting I/O operations")

	// Copy stdin to container in background
	go func() {
		debugln("Starting stdin -> container copy")
		_, _ = io.Copy(execAttachResp.Conn, os.Stdin)
		debugln("Stdin copy completed")
	}()

	// Copy container output to stdout (blocks until exec finishes)
	debugln("Starting container -> stdout copy (blocking)")
	_, err = io.Copy(os.Stdout, execAttachResp.Reader)
	debugf("Stdout copy completed: err=%v\n", err)

	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to handle interactive session: %w", err)
	}

	debugln("Shell session completed successfully")

	return nil
}
