package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
	"github.com/garaemon/devgo/pkg/sshagent"
	"github.com/opencontainers/image-spec/specs-go/v1"
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
	ImageExists(ctx context.Context, imageName string) (bool, error)
	PullImage(ctx context.Context, imageName string) error
	Close() error
}

// dockerAPIClient interface wraps the Docker client methods we use
type dockerAPIClient interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error)
	ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error)
	ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
	Close() error
}

// dockerClientFactory is a function type for creating Docker clients
type dockerClientFactory func() (dockerAPIClient, error)

// defaultDockerClientFactory creates a real Docker client
func defaultDockerClientFactory() (dockerAPIClient, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}

// realDockerClient implements DockerClient using Docker SDK
type realDockerClient struct {
	client dockerAPIClient
}

func newRealDockerClient() (DockerClient, error) {
	return newRealDockerClientWithFactory(defaultDockerClientFactory)
}

func newRealDockerClientWithFactory(factory dockerClientFactory) (DockerClient, error) {
	cli, err := factory()
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &realDockerClient{client: cli}, nil
}

func runUpCommand(args []string) error {
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

	if err := executeInitializeCommand(devContainer, workspaceDir); err != nil {
		return fmt.Errorf("failed to execute initialize command: %w", err)
	}

	return startContainerWithDocker(ctx, devContainer, containerName, workspaceDir, dockerClient)
}

func startContainerWithDocker(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string, dockerClient DockerClient) error {
	if devContainer.HasDockerCompose() {
		return startContainerWithDockerCompose(ctx, devContainer, containerName, workspaceDir)
	}

	// Determine the image to use
	imageName := devContainer.Image

	// If no image is specified but build configuration exists, build the image
	if imageName == "" && devContainer.HasBuild() {
		devcontainerPath, err := findDevcontainerConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to find devcontainer config: %w", err)
		}

		fmt.Println("No image specified, building from Dockerfile...")
		if err := buildDevContainer(devContainer, workspaceDir, devcontainerPath); err != nil {
			return fmt.Errorf("failed to build dev container: %w", err)
		}

		// Use the built image
		imageName = determineImageTag(devContainer, workspaceDir)
		devContainer.Image = imageName
	}

	if imageName == "" {
		return fmt.Errorf("devcontainer must specify an image, build configuration, or docker compose configuration")
	}

	// TODO: Add support for --container-name option similar to devcontainer-cli runArgs
	// The official devcontainer-cli doesn't have a direct --name option for the up command,
	// but supports container naming through runArgs in devcontainer.json.
	// We should consider adding a --container-name option for command-line convenience.

	// Check if we need to pull the image
	shouldPullImage := pull
	if !shouldPullImage {
		// Check if image exists locally
		imageExists, err := dockerClient.ImageExists(ctx, devContainer.Image)
		if err != nil {
			return fmt.Errorf("failed to check if image exists: %w", err)
		}
		shouldPullImage = !imageExists
	}

	// Pull image if needed
	if shouldPullImage {
		if pull {
			fmt.Printf("Pulling image '%s'\n", devContainer.Image)
		} else {
			fmt.Printf("Image '%s' not found locally, pulling...\n", devContainer.Image)
		}
		if err := dockerClient.PullImage(ctx, devContainer.Image); err != nil {
			return fmt.Errorf("failed to pull image '%s': %w", devContainer.Image, err)
		}
	}

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

	if err := dockerClient.CreateAndStartContainer(ctx, dockerArgs); err != nil {
		return err
	}

	return executeLifecycleCommands(ctx, devContainer, containerName, workspaceDir)
}

func executeOnCreateCommand(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	onCreateArgs := devContainer.GetOnCreateCommandArgs()
	if len(onCreateArgs) == 0 {
		return nil
	}

	fmt.Printf("Running onCreateCommand: %s\n", strings.Join(onCreateArgs, " "))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for onCreateCommand: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close Docker client: %v\n", closeErr)
		}
	}()

	if err := executeCommandInContainer(ctx, cli, containerName, onCreateArgs, devContainer); err != nil {
		return err
	}

	fmt.Println("Finished onCreateCommand")
	return nil
}

func executeUpdateContentCommand(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	updateContentArgs := devContainer.GetUpdateContentCommandArgs()
	if len(updateContentArgs) == 0 {
		return nil
	}

	fmt.Printf("Running updateContentCommand: %s\n", strings.Join(updateContentArgs, " "))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for updateContentCommand: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close Docker client: %v\n", closeErr)
		}
	}()

	if err := executeCommandInContainer(ctx, cli, containerName, updateContentArgs, devContainer); err != nil {
		return err
	}

	fmt.Println("Finished updateContentCommand")
	return nil
}

func executePostCreateCommand(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	postCreateArgs := devContainer.GetPostCreateCommandArgs()
	if len(postCreateArgs) == 0 {
		return nil
	}

	fmt.Printf("Running postCreateCommand: %s\n", strings.Join(postCreateArgs, " "))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for postCreateCommand: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close Docker client: %v\n", closeErr)
		}
	}()

	if err := executeCommandInContainer(ctx, cli, containerName, postCreateArgs, devContainer); err != nil {
		return err
	}

	fmt.Println("Finished postCreateCommand")
	return nil
}

func executePostStartCommand(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	postStartArgs := devContainer.GetPostStartCommandArgs()
	if len(postStartArgs) == 0 {
		return nil
	}

	fmt.Printf("Running postStartCommand: %s\n", strings.Join(postStartArgs, " "))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for postStartCommand: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close Docker client: %v\n", closeErr)
		}
	}()

	if err := executeCommandInContainer(ctx, cli, containerName, postStartArgs, devContainer); err != nil {
		return err
	}

	fmt.Println("Finished postStartCommand")
	return nil
}

func executePostAttachCommand(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	postAttachArgs := devContainer.GetPostAttachCommandArgs()
	if len(postAttachArgs) == 0 {
		return nil
	}

	fmt.Printf("Running postAttachCommand: %s\n", strings.Join(postAttachArgs, " "))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for postAttachCommand: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close Docker client: %v\n", closeErr)
		}
	}()

	if err := executeCommandInContainer(ctx, cli, containerName, postAttachArgs, devContainer); err != nil {
		return err
	}

	fmt.Println("Finished postAttachCommand")
	return nil
}

func executeInitializeCommand(devContainer *devcontainer.DevContainer, workspaceDir string) error {
	initArgs := devContainer.GetInitializeCommandArgs()
	if len(initArgs) == 0 {
		return nil
	}

	fmt.Printf("Running initializeCommand: %s\n", strings.Join(initArgs, " "))

	cmd := exec.Command(initArgs[0], initArgs[1:]...)
	cmd.Dir = workspaceDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("initializeCommand failed: %w", err)
	}

	fmt.Println("Finished initializeCommand")

	return nil
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

	// Determine session name
	session := sessionName
	if session == "" {
		session = constants.DefaultSessionName
	}

	// Create container configuration with devgo labels
	labels := map[string]string{
		constants.DevgoManagedLabel:   constants.DevgoManagedValue,
		constants.DevgoWorkspaceLabel: args.WorkspaceDir,
		constants.DevgoSessionLabel:   session,
	}

	// Create host configuration with volume mounts
	binds := []string{fmt.Sprintf("%s:%s", args.WorkspaceDir, args.WorkspaceFolder)}

	// Add SSH agent forwarding if available
	if sshagent.IsAvailable() {
		hostSocket, err := sshagent.GetHostSocket()
		if err == nil {
			mount := sshagent.CreateMount(hostSocket)
			binds = append(binds, fmt.Sprintf("%s:%s", mount.Source, mount.Target))

			// Add SSH_AUTH_SOCK environment variable
			sshEnv := sshagent.GetContainerEnv()
			for key, value := range sshEnv {
				env = append(env, fmt.Sprintf("%s=%s", key, value))
			}

			fmt.Printf("SSH agent forwarding enabled: %s -> %s\n",
				hostSocket, mount.Target)
		}
	}

	config := &container.Config{
		Image:  args.Image,
		Cmd:    []string{"sleep", "infinity"},
		Env:    env,
		Labels: labels,
	}

	hostConfig := &container.HostConfig{
		Binds: binds,
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

func (r *realDockerClient) ImageExists(ctx context.Context, imageName string) (bool, error) {
	images, err := r.client.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list images: %w", err)
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == imageName {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *realDockerClient) PullImage(ctx context.Context, imageName string) error {
	resp, err := r.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer func() {
		if closeErr := resp.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close pull response: %v\n", closeErr)
		}
	}()

	// Read the response to ensure the pull completes
	_, err = io.Copy(io.Discard, resp)
	if err != nil {
		return fmt.Errorf("failed to read pull response: %w", err)
	}

	fmt.Printf("Image '%s' pulled successfully\n", imageName)
	return nil
}

func (r *realDockerClient) Close() error {
	return r.client.Close()
}

func updateRemoteUserUID(ctx context.Context, devContainer *devcontainer.DevContainer, containerName string) error {
	// Only applicable on Linux
	if runtime.GOOS != "linux" {
		return nil
	}

	// Check if feature is enabled
	if !devContainer.ShouldUpdateRemoteUserUID() {
		return nil
	}

	// Only supported with image and dockerFile, not dockerComposeFile
	if devContainer.HasDockerCompose() {
		return nil
	}

	targetUser := devContainer.GetTargetUser()
	// Never update root user
	if targetUser == "" || targetUser == "root" {
		return nil
	}

	hostUID := os.Getuid()
	hostGID := os.Getgid()

	fmt.Printf("Updating container user '%s' UID/GID to match host (%d:%d)\n", targetUser, hostUID, hostGID)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close Docker client: %v\n", closeErr)
		}
	}()

	// Commands to update UID/GID
	// We use || true to make commands non-failing
	commands := [][]string{
		// Update user's UID
		{"/bin/sh", "-c", fmt.Sprintf("usermod -u %d %s 2>/dev/null || true", hostUID, targetUser)},
		// Update user's primary group GID
		{"/bin/sh", "-c", fmt.Sprintf("groupmod -g %d %s 2>/dev/null || true", hostGID, targetUser)},
		// Fix ownership of user's home directory
		{"/bin/sh", "-c", fmt.Sprintf("chown -R %d:%d /home/%s 2>/dev/null || true", hostUID, hostGID, targetUser)},
	}

	// Execute commands as root
	tempDevContainer := &devcontainer.DevContainer{
		ContainerUser:   "root",
		WorkspaceFolder: devContainer.GetWorkspaceFolder(),
	}

	for _, cmd := range commands {
		if err := executeCommandInContainer(ctx, cli, containerName, cmd, tempDevContainer); err != nil {
			return fmt.Errorf("failed to execute UID/GID update command: %w", err)
		}
	}

	return nil
}

func executeLifecycleCommands(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	// Update remote user UID/GID before executing lifecycle commands
	if err := updateRemoteUserUID(ctx, devContainer, containerName); err != nil {
		// Only warn, don't fail the entire lifecycle
		fmt.Printf("Warning: failed to update remote user UID/GID: %v\n", err)
	}

	commands := []struct {
		commandType string
		executor    func(context.Context, *devcontainer.DevContainer, string, string) error
	}{
		{devcontainer.WaitForOnCreateCommand, executeOnCreateCommand},
		{devcontainer.WaitForUpdateContentCommand, executeUpdateContentCommand},
		{devcontainer.WaitForPostCreateCommand, executePostCreateCommand},
		{devcontainer.WaitForPostStartCommand, executePostStartCommand},
	}

	waitFor := devContainer.GetWaitFor()
	fmt.Printf("Executing lifecycle commands up to: %s\n", waitFor)

	// Execute commands synchronously until waitFor
	for _, cmd := range commands {
		if devContainer.ShouldWaitForCommand(cmd.commandType) {
			if err := cmd.executor(ctx, devContainer, containerName, workspaceDir); err != nil {
				return fmt.Errorf("failed to execute %s: %w", cmd.commandType, err)
			}
		}
	}

	fmt.Printf("Container is ready for use (waitFor: %s completed)\n", waitFor)

	// Execute remaining commands asynchronously
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, cmd := range commands {
			if !devContainer.ShouldWaitForCommand(cmd.commandType) {
				if err := cmd.executor(ctx, devContainer, containerName, workspaceDir); err != nil {
					fmt.Printf("Background command %s failed: %v\n", cmd.commandType, err)
				}
			}
		}

		// Always execute postAttachCommand last
		if err := executePostAttachCommand(ctx, devContainer, containerName, workspaceDir); err != nil {
			fmt.Printf("Background postAttachCommand failed: %v\n", err)
		}
	}()

	wg.Wait()

	return nil
}

func startContainerWithDockerCompose(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	if devContainer.GetService() == "" {
		return fmt.Errorf("service name is required when using docker compose")
	}

	composeFiles := devContainer.GetDockerComposeFiles()
	if len(composeFiles) == 0 {
		return fmt.Errorf("no docker compose files specified")
	}

	// Build docker compose command arguments
	var composeArgs []string
	for _, file := range composeFiles {
		composeArgs = append(composeArgs, "-f", filepath.Join(workspaceDir, file))
	}

	// Determine which services to run
	runServices := devContainer.GetRunServices()
	if len(runServices) == 0 {
		runServices = []string{devContainer.GetService()}
	}

	// Check if services are already running
	checkCmd := exec.Command("docker", append(append([]string{"compose"}, composeArgs...), "ps", "-q", devContainer.GetService())...)
	checkCmd.Dir = workspaceDir
	output, err := checkCmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		fmt.Printf("Service '%s' is already running\n", devContainer.GetService())
		return executeLifecycleCommands(ctx, devContainer, containerName, workspaceDir)
	}

	// Start docker compose services
	upArgs := append(composeArgs, append([]string{"up", "-d"}, runServices...)...)
	upCmd := exec.Command("docker", append([]string{"compose"}, upArgs...)...)
	upCmd.Dir = workspaceDir
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr

	fmt.Printf("Starting docker compose services: %s\n", strings.Join(runServices, ", "))
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("failed to start docker compose services: %w", err)
	}

	fmt.Printf("Docker compose services started successfully\n")
	return executeLifecycleCommands(ctx, devContainer, containerName, workspaceDir)
}
