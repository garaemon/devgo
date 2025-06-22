package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/garaemon/devgo/pkg/devcontainer"
)

// DownDockerClient interface for down command Docker operations
type DownDockerClient interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	Close() error
}

func runDownCommand(args []string) error {
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
	return stopAndRemoveContainer(ctx, cli, containerName)
}

func stopAndRemoveContainer(ctx context.Context, cli DownDockerClient, containerName string) error {
	// Check if container exists
	filter := filters.NewArgs()
	filter.Add("name", containerName)
	
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filter,
	})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}
	
	var found bool
	var containerID string
	var isRunning bool
	
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.TrimPrefix(name, "/") == containerName {
				found = true
				containerID = c.ID
				isRunning = c.State == "running"
				break
			}
		}
		if found {
			break
		}
	}
	
	if !found {
		fmt.Printf("Container '%s' does not exist\n", containerName)
		return nil
	}
	
	// Stop container if it's running
	if isRunning {
		fmt.Printf("Stopping container '%s'\n", containerName)
		err = cli.ContainerStop(ctx, containerID, container.StopOptions{})
		if err != nil {
			return fmt.Errorf("failed to stop container '%s': %w", containerName, err)
		}
		fmt.Printf("Container '%s' stopped\n", containerName)
	}
	
	// Remove container
	fmt.Printf("Removing container '%s'\n", containerName)
	err = cli.ContainerRemove(ctx, containerID, container.RemoveOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove container '%s': %w", containerName, err)
	}
	
	fmt.Printf("Container '%s' removed successfully\n", containerName)
	return nil
}