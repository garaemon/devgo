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

func runStopCommand(args []string) error {
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
	return stopContainer(ctx, cli, containerName)
}

func stopContainer(ctx context.Context, cli *client.Client, containerName string) error {
	// Check if container exists and is running
	filter := filters.NewArgs()
	filter.Add("name", containerName)
	filter.Add("status", "running")

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		Filters: filter,
	})
	if err != nil {
		return fmt.Errorf("failed to list running containers: %w", err)
	}

	var found bool
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.TrimPrefix(name, "/") == containerName {
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		fmt.Printf("Container '%s' is not running\n", containerName)
		return nil
	}

	fmt.Printf("Stopping container '%s'\n", containerName)
	err = cli.ContainerStop(ctx, containerName, container.StopOptions{})
	if err != nil {
		return fmt.Errorf("failed to stop container '%s': %w", containerName, err)
	}

	fmt.Printf("Container '%s' stopped successfully\n", containerName)
	return nil
}
