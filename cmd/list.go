package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/garaemon/devgo/pkg/constants"
)

// DockerListClient interface for Docker container listing operations
type DockerListClient interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	Close() error
}

func runListCommand(args []string) error {
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
	return listDevgoContainers(ctx, cli)
}

func listDevgoContainers(ctx context.Context, cli DockerListClient) error {
	filter := filters.NewArgs()
	filter.Add("label", fmt.Sprintf("%s=%s", constants.DevgoManagedLabel, constants.DevgoManagedValue))

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filter,
	})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Println("No devgo containers found")
		return nil
	}

	fmt.Printf("%-20s %-15s %-20s %-10s %s\n", "NAME", "STATUS", "IMAGE", "CREATED", "WORKSPACE")
	fmt.Println(strings.Repeat("-", 80))

	for _, c := range containers {
		name := getContainerName(c.Names)
		status := c.Status
		image := c.Image
		created := time.Unix(c.Created, 0).Format("2006-01-02")
		workspace := getWorkspaceFromLabels(c.Labels)

		fmt.Printf("%-20s %-15s %-20s %-10s %s\n", name, status, image, created, workspace)
	}

	return nil
}

func getContainerName(names []string) string {
	if len(names) == 0 {
		return "<none>"
	}
	// Docker container names include a leading slash, remove it
	return strings.TrimPrefix(names[0], "/")
}

func getWorkspaceFromLabels(labels map[string]string) string {
	if workspace, exists := labels[constants.DevgoWorkspaceLabel]; exists {
		return workspace
	}
	return "<unknown>"
}