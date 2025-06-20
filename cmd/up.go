package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
)

// DockerRunArgs represents arguments for docker run command
type DockerRunArgs struct {
	Name            string
	Image           string
	WorkspaceDir    string
	WorkspaceFolder string
	Env             map[string]string
}

// DockerClient interface for Docker operations
type DockerClient interface {
	ContainerExists(ctx context.Context, name string) (bool, error)
	IsContainerRunning(ctx context.Context, name string) (bool, error)
	StartExistingContainer(ctx context.Context, name string) error
	CreateAndStartContainer(ctx context.Context, args DockerRunArgs) error
	Close() error
}

// realDockerClient implements DockerClient using Docker SDK
type realDockerClient struct {
	client *client.Client
}

func newRealDockerClient() (DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &realDockerClient{client: cli}, nil
}

func runUpCommand(args []string) error {
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
	dockerClient, err := newRealDockerClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer func() {
		if closeErr := dockerClient.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close Docker client: %v\n", closeErr)
		}
	}()
	
	ctx := context.Background()
	return startContainerWithDocker(ctx, devContainer, containerName, workspaceDir, dockerClient)
}

func determineWorkspaceFolder() string {
	if workspaceFolder != "" {
		return workspaceFolder
	}
	cwd, _ := os.Getwd()
	return cwd
}

func determineContainerName(devContainer *devcontainer.DevContainer, workspaceDir string) string {
	if containerName != "" {
		return containerName
	}
	if devContainer.Name != "" {
		return devContainer.Name
	}
	return fmt.Sprintf("devgo-%s", filepath.Base(workspaceDir))
}

func startContainerWithDocker(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string, dockerClient DockerClient) error {
	if !devContainer.HasImage() {
		return fmt.Errorf("devcontainer must specify an image")
	}

	// TODO: Add support for --container-name option similar to devcontainer-cli runArgs
	// The official devcontainer-cli doesn't have a direct --name option for the up command,
	// but supports container naming through runArgs in devcontainer.json.
	// We should consider adding a --container-name option for command-line convenience.

	// Check if container already exists
	exists, err := dockerClient.ContainerExists(ctx, containerName)
	if err != nil {
		return fmt.Errorf("failed to check if container exists: %w", err)
	}

	if exists {
		running, err := dockerClient.IsContainerRunning(ctx, containerName)
		if err != nil {
			return fmt.Errorf("failed to check if container is running: %w", err)
		}
		if running {
			return fmt.Errorf("container '%s' is already running", containerName)
		}
		fmt.Printf("Container '%s' exists but is stopped, starting it\n", containerName)
		return dockerClient.StartExistingContainer(ctx, containerName)
	}

	fmt.Printf("Creating and starting container '%s' with image '%s'\n", containerName, devContainer.Image)

	dockerArgs := DockerRunArgs{
		Name:            containerName,
		Image:           devContainer.Image,
		WorkspaceDir:    workspaceDir,
		WorkspaceFolder: devContainer.GetWorkspaceFolder(),
		Env:             devContainer.ContainerEnv,
	}

	return dockerClient.CreateAndStartContainer(ctx, dockerArgs)
}

// realDockerClient methods
func (r *realDockerClient) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	filter := filters.NewArgs()
	filter.Add("name", containerName)
	
	containers, err := r.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filter,
	})
	if err != nil {
		return false, fmt.Errorf("failed to list containers: %w", err)
	}
	
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.TrimPrefix(name, "/") == containerName {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *realDockerClient) IsContainerRunning(ctx context.Context, containerName string) (bool, error) {
	filter := filters.NewArgs()
	filter.Add("name", containerName)
	filter.Add("status", "running")
	
	containers, err := r.client.ContainerList(ctx, container.ListOptions{
		Filters: filter,
	})
	if err != nil {
		return false, fmt.Errorf("failed to list running containers: %w", err)
	}
	
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.TrimPrefix(name, "/") == containerName {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *realDockerClient) StartExistingContainer(ctx context.Context, containerName string) error {
	err := r.client.ContainerStart(ctx, containerName, container.StartOptions{})
	if err != nil {
		return fmt.Errorf("failed to start existing container: %w", err)
	}
	fmt.Printf("Container '%s' started successfully\n", containerName)
	return nil
}

func (r *realDockerClient) CreateAndStartContainer(ctx context.Context, args DockerRunArgs) error {
	// Prepare environment variables
	var env []string
	if args.Env != nil {
		for key, value := range args.Env {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Create container configuration with devgo labels
	labels := map[string]string{
		constants.DevgoManagedLabel:   constants.DevgoManagedValue,
		constants.DevgoWorkspaceLabel: args.WorkspaceDir,
	}
	
	config := &container.Config{
		Image:  args.Image,
		Cmd:    []string{"sleep", "infinity"},
		Env:    env,
		Labels: labels,
	}

	// Create host configuration with volume mounts
	hostConfig := &container.HostConfig{
		Binds: []string{fmt.Sprintf("%s:%s", args.WorkspaceDir, args.WorkspaceFolder)},
	}

	// Create the container
	resp, err := r.client.ContainerCreate(ctx, config, hostConfig, nil, nil, args.Name)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Start the container
	err = r.client.ContainerStart(ctx, resp.ID, container.StartOptions{})
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	fmt.Printf("Container '%s' started successfully\n", args.Name)
	return nil
}

func (r *realDockerClient) Close() error {
	return r.client.Close()
}