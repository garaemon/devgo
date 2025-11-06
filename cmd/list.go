package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
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

	// Create a new tabwriter with proper column alignment
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print header
	if _, err := fmt.Fprintln(w, "NAME\tSESSION\tSTATUS\tIMAGE\tCREATED\tWORKSPACE"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("-", 20)+"\t"+strings.Repeat("-", 12)+"\t"+
		strings.Repeat("-", 15)+"\t"+strings.Repeat("-", 20)+"\t"+
		strings.Repeat("-", 10)+"\t"+strings.Repeat("-", 20)); err != nil {
		return fmt.Errorf("failed to write separator: %w", err)
	}

	for _, c := range containers {
		name := getContainerName(c.Names)
		session := getSessionFromLabels(c.Labels)
		status := c.Status
		image := c.Image
		created := time.Unix(c.Created, 0).Format("2006-01-02")
		workspace := getWorkspaceFromLabels(c.Labels)

		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", name, session, status, image, created, workspace); err != nil {
			return fmt.Errorf("failed to write container info: %w", err)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %w", err)
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

func getSessionFromLabels(labels map[string]string) string {
	if session, exists := labels[constants.DevgoSessionLabel]; exists {
		return session
	}
	return "<unknown>"
}
