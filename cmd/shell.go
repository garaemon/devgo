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
	"github.com/garaemon/devgo/pkg/devcontainer"
	"golang.org/x/term"
)

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
			fmt.Printf("Warning: failed to close Docker client: %v\n", closeErr)
		}
	}()

	ctx := context.Background()
	return executeInteractiveShell(ctx, cli, containerName, devContainer)
}

func executeInteractiveShell(ctx context.Context, cli DockerExecClient, containerName string, devContainer *devcontainer.DevContainer) error {
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
	var env []string
	// TERM should be set based on the current terminal, but xterm-256color is a safe default
	env = append(env, "TERM=xterm-256color")
	for k, v := range expandedEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	user := devContainer.GetContainerUser()
	workspaceFolder := devContainer.GetWorkspaceFolder()

	// Get terminal size before creating exec
	stdinFd := int(os.Stdin.Fd())
	var consoleSize *[2]uint
	if term.IsTerminal(stdinFd) {
		width, height, err := term.GetSize(stdinFd)
		if err == nil {
			consoleSize = &[2]uint{uint(height), uint(width)}
			if verbose {
				fmt.Printf("Terminal size: %dx%d (cols x rows)\n", width, height)
			}
		}
	}

	execConfig := container.ExecOptions{
		User:         user,
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/bash", "-i", "-l"},
		WorkingDir:   workspaceFolder,
		Env:          env,
		ConsoleSize:  consoleSize,
		DetachKeys:   "ctrl-@", // Use ctrl-@ instead of default ctrl-p,ctrl-q to allow ctrl-p for history
	}

	if verbose {
		fmt.Printf("Creating exec instance with config:\n")
		fmt.Printf("  User: %s\n", user)
		fmt.Printf("  Tty: %v\n", execConfig.Tty)
		fmt.Printf("  AttachStdin: %v\n", execConfig.AttachStdin)
		fmt.Printf("  AttachStdout: %v\n", execConfig.AttachStdout)
		fmt.Printf("  AttachStderr: %v\n", execConfig.AttachStderr)
		fmt.Printf("  Cmd: %v\n", execConfig.Cmd)
		fmt.Printf("  Env: %v\n", execConfig.Env)
		fmt.Printf("  DetachKeys: %s (allows ctrl-p for history)\n", execConfig.DetachKeys)
		if consoleSize != nil {
			fmt.Printf("  ConsoleSize: %dx%d\n", consoleSize[1], consoleSize[0])
		}
		fmt.Printf("  WorkingDir: %s\n", execConfig.WorkingDir)
		fmt.Printf("  Container ID: %s\n", containerID)
	}

	execCreateResp, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec instance: %w", err)
	}

	if verbose {
		fmt.Printf("Exec instance created with ID: %s\n", execCreateResp.ID)
	}

	// Check if stdin is a terminal and set raw mode
	var oldState *term.State
	if term.IsTerminal(stdinFd) {
		if verbose {
			fmt.Printf("Setting terminal to raw mode (fd: %d)\n", stdinFd)
		}
		oldState, err = term.MakeRaw(stdinFd)
		if err != nil {
			return fmt.Errorf("failed to set terminal to raw mode: %w", err)
		}
		if verbose {
			fmt.Printf("Terminal set to raw mode successfully\n")
		}
		defer func() {
			if restoreErr := term.Restore(stdinFd, oldState); restoreErr != nil {
				fmt.Printf("Warning: failed to restore terminal: %v\n", restoreErr)
			}
		}()
	} else if verbose {
		fmt.Printf("Warning: stdin is not a terminal (fd: %d)\n", stdinFd)
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
	if verbose {
		fmt.Printf("Attaching to exec instance %s\n", execCreateResp.ID)
	}
	execAttachResp, err := cli.ContainerExecAttach(ctx, execCreateResp.ID, container.ExecAttachOptions{
		Tty: true,
	})
	if err != nil {
		return fmt.Errorf("failed to attach to exec instance: %w", err)
	}
	defer execAttachResp.Close()
	if verbose {
		fmt.Printf("Successfully attached to exec instance\n")
	}

	// Start the exec instance in a separate goroutine
	// This must be done AFTER attach and runs concurrently with I/O
	if verbose {
		fmt.Printf("Starting exec instance in background\n")
	}
	go func() {
		startErr := cli.ContainerExecStart(ctx, execCreateResp.ID, container.ExecStartOptions{
			Tty: true,
		})
		if verbose {
			if startErr != nil {
				fmt.Printf("ExecStart error: %v\n", startErr)
			} else {
				fmt.Printf("ExecStart completed\n")
			}
		}
	}()

	// Handle TTY I/O
	if verbose {
		fmt.Printf("Starting I/O operations\n")
	}

	// Copy stdin to container in background
	go func() {
		if verbose {
			fmt.Printf("Starting stdin -> container copy\n")
		}
		_, _ = io.Copy(execAttachResp.Conn, os.Stdin)
		if verbose {
			fmt.Printf("Stdin copy completed\n")
		}
	}()

	// Copy container output to stdout (blocks until exec finishes)
	if verbose {
		fmt.Printf("Starting container -> stdout copy (blocking)\n")
	}
	_, err = io.Copy(os.Stdout, execAttachResp.Reader)
	if verbose {
		fmt.Printf("Stdout copy completed: err=%v\n", err)
	}

	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to handle interactive session: %w", err)
	}

	if verbose {
		fmt.Printf("Shell session completed successfully\n")
	}

	return nil
}
