package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
)

// DockerExecClient interface for Docker exec operations
type DockerExecClient interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerExecCreate(ctx context.Context, containerID string, config container.ExecOptions) (container.ExecCreateResponse, error)
	ContainerExecStart(ctx context.Context, execID string, config container.ExecStartOptions) error
	ContainerExecAttach(ctx context.Context, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error)
	Close() error
}

func runExecCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("exec command requires at least one argument")
	}

	workspaceDir := determineWorkspaceFolder()
	
	devcontainerPath, err := findDevcontainerConfig("")
	if err != nil {
		return fmt.Errorf("failed to find devcontainer config: %w", err)
	}

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
	return executeCommandInContainer(ctx, cli, containerName, args, devContainer)
}

func executeCommandInContainer(ctx context.Context, cli DockerExecClient, containerName string, args []string, devContainer *devcontainer.DevContainer) error {
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
		Tty:          false, // Disable TTY for simpler output handling
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          args,
		WorkingDir:   workspaceFolder,
	}

	execCreateResp, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec instance: %w", err)
	}

	execAttachResp, err := cli.ContainerExecAttach(ctx, execCreateResp.ID, container.ExecAttachOptions{
		Tty: false,
	})
	if err != nil {
		return fmt.Errorf("failed to attach to exec instance: %w", err)
	}
	defer execAttachResp.Close()

	// Start the exec instance
	err = cli.ContainerExecStart(ctx, execCreateResp.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("failed to start exec instance: %w", err)
	}

	// Demultiplex the output stream (Docker uses multiplexed stdout/stderr)
	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, execAttachResp.Reader)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to copy output: %w", err)
	}

	return nil
}

func findRunningContainer(ctx context.Context, cli DockerExecClient, containerName string) (string, error) {
	filter := filters.NewArgs()
	filter.Add("name", containerName)
	filter.Add("status", "running")
	filter.Add("label", fmt.Sprintf("%s=%s", constants.DevgoManagedLabel, constants.DevgoManagedValue))

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		Filters: filter,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %w", err)
	}

	for _, c := range containers {
		for _, name := range c.Names {
			if strings.TrimPrefix(name, "/") == containerName {
				return c.ID, nil
			}
		}
	}

	return "", nil
}

