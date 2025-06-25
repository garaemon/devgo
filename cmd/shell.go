package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/garaemon/devgo/pkg/devcontainer"
	"golang.org/x/term"
)

func runShellCommand(args []string) error {
	devcontainerPath, err := findDevcontainerConfig("")
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

	user := devContainer.GetContainerUser()
	workspaceFolder := devContainer.GetWorkspaceFolder()

	execConfig := container.ExecOptions{
		User:         user,
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/bash"},
		WorkingDir:   workspaceFolder,
	}

	execCreateResp, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec instance: %w", err)
	}

	execAttachResp, err := cli.ContainerExecAttach(ctx, execCreateResp.ID, container.ExecAttachOptions{
		Tty: true,
	})
	if err != nil {
		return fmt.Errorf("failed to attach to exec instance: %w", err)
	}
	defer execAttachResp.Close()

	// Check if stdin is a terminal and set raw mode
	stdinFd := int(os.Stdin.Fd())
	var oldState *term.State
	if term.IsTerminal(stdinFd) {
		oldState, err = term.MakeRaw(stdinFd)
		if err != nil {
			return fmt.Errorf("failed to set terminal to raw mode: %w", err)
		}
		defer func() {
			if restoreErr := term.Restore(stdinFd, oldState); restoreErr != nil {
				fmt.Printf("Warning: failed to restore terminal: %v\n", restoreErr)
			}
		}()
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

	// Start the exec instance
	err = cli.ContainerExecStart(ctx, execCreateResp.ID, container.ExecStartOptions{
		Tty: true,
	})
	if err != nil {
		return fmt.Errorf("failed to start exec instance: %w", err)
	}

	// Handle TTY I/O like docker exec -it does
	go func() {
		defer func() {
			if closeErr := execAttachResp.CloseWrite(); closeErr != nil && oldState != nil {
				_ = term.Restore(stdinFd, oldState)
			}
		}()
		_, _ = io.Copy(execAttachResp.Conn, os.Stdin)
	}()

	// For TTY mode, output is not multiplexed
	_, err = io.Copy(os.Stdout, execAttachResp.Reader)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to handle interactive session: %w", err)
	}

	return nil
}