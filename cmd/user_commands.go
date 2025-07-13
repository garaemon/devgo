package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
)

func runUserCommandsCommand(args []string) error {
	devcontainerPath, err := findDevcontainerConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to find devcontainer config: %w", err)
	}

	devContainer, err := devcontainer.Parse(devcontainerPath)
	if err != nil {
		return fmt.Errorf("failed to parse devcontainer config: %w", err)
	}

	ctx := context.Background()
	containerName, err := findRunningDevContainer(ctx, devContainer)
	if err != nil {
		return fmt.Errorf("failed to find running devcontainer: %w", err)
	}

	workspaceDir := filepath.Dir(devcontainerPath)
	if filepath.Base(workspaceDir) == ".devcontainer" {
		workspaceDir = filepath.Dir(workspaceDir)
	}

	// Execute lifecycle commands
	if err := runLifecycleCommands(ctx, devContainer, containerName, workspaceDir); err != nil {
		return fmt.Errorf("failed to run user commands: %w", err)
	}

	return nil
}

func findRunningDevContainer(ctx context.Context, devContainer *devcontainer.DevContainer) (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close Docker client: %v\n", closeErr)
		}
	}()

	filter := filters.NewArgs()
	filter.Add("label", fmt.Sprintf("%s=%s", constants.DevgoManagedLabel, constants.DevgoManagedValue))

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     false, // Only running containers
		Filters: filter,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		return "", fmt.Errorf("no running devgo containers found")
	}

	// If multiple containers found, use the first one or find the one matching current workspace
	for _, container := range containers {
		// Check if container has the workspace label matching current directory
		if workspaceLabel, exists := container.Labels[constants.DevgoWorkspaceLabel]; exists {
			currentDir, err := os.Getwd()
			if err == nil && workspaceLabel == currentDir {
				return container.Names[0][1:], nil // Remove leading '/'
			}
		}
	}

	// If no exact match, return the first container
	return containers[0].Names[0][1:], nil // Remove leading '/'
}

func runLifecycleCommands(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	waitFor := devContainer.GetWaitFor()

	// Execute commands according to waitFor setting
	if devContainer.ShouldWaitForCommand(devcontainer.WaitForOnCreateCommand) {
		if err := executeOnCreateCommand(ctx, devContainer, containerName, workspaceDir); err != nil {
			return fmt.Errorf("onCreateCommand failed: %w", err)
		}
	}

	if devContainer.ShouldWaitForCommand(devcontainer.WaitForUpdateContentCommand) {
		if err := executeUpdateContentCommand(ctx, devContainer, containerName, workspaceDir); err != nil {
			return fmt.Errorf("updateContentCommand failed: %w", err)
		}
	}

	if devContainer.ShouldWaitForCommand(devcontainer.WaitForPostCreateCommand) {
		if err := executePostCreateCommand(ctx, devContainer, containerName, workspaceDir); err != nil {
			return fmt.Errorf("postCreateCommand failed: %w", err)
		}
	}

	if devContainer.ShouldWaitForCommand(devcontainer.WaitForPostStartCommand) {
		if err := executePostStartCommand(ctx, devContainer, containerName, workspaceDir); err != nil {
			return fmt.Errorf("postStartCommand failed: %w", err)
		}
	}

	// Always run postAttachCommand if it exists
	if err := executePostAttachCommand(ctx, devContainer, containerName, workspaceDir); err != nil {
		return fmt.Errorf("postAttachCommand failed: %w", err)
	}

	fmt.Printf("Successfully executed user commands up to %s\n", waitFor)
	return nil
}
