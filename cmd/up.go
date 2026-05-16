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
	"github.com/garaemon/devgo/pkg/config"
	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
	"github.com/garaemon/devgo/pkg/dotfiles"
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
			warnf("failed to close Docker client: %v", closeErr)
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

		debugln("No image specified, building from Dockerfile...")
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
			debugf("Pulling image '%s'\n", devContainer.Image)
		} else {
			debugf("Image '%s' not found locally, pulling...\n", devContainer.Image)
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
		debugf("Container '%s' exists but is stopped, removing and recreating it to apply configuration changes\n", containerName)
		
		// Use raw docker client to remove the container
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err == nil {
			_ = cli.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true})
			_ = cli.Close()
		}
	}

	// Get base environment variables from image for expansion
	baseEnv, err := getImageEnv(ctx, devContainer.Image)
	if err != nil {
		warnf("failed to get base environment variables from image: %v", err)
		baseEnv = make(map[string]string)
	}

	expandedEnv := devContainer.GetContainerEnv(baseEnv)

	debugf("Creating and starting container '%s' with image '%s'\n", containerName, devContainer.Image)

	dockerArgs := DockerRunArgs{
		Name:            containerName,
		Image:           devContainer.Image,
		WorkspaceDir:    workspaceDir,
		WorkspaceFolder: devContainer.GetWorkspaceFolder(),
		Env:             expandedEnv,
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

	debugf("Running onCreateCommand: %s\n", strings.Join(onCreateArgs, " "))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for onCreateCommand: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			warnf("failed to close Docker client: %v", closeErr)
		}
	}()

	if err := executeCommandInContainer(ctx, cli, containerName, onCreateArgs, devContainer); err != nil {
		return err
	}

	debugln("Finished onCreateCommand")
	return nil
}

func executeUpdateContentCommand(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	updateContentArgs := devContainer.GetUpdateContentCommandArgs()
	if len(updateContentArgs) == 0 {
		return nil
	}

	debugf("Running updateContentCommand: %s\n", strings.Join(updateContentArgs, " "))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for updateContentCommand: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			warnf("failed to close Docker client: %v", closeErr)
		}
	}()

	if err := executeCommandInContainer(ctx, cli, containerName, updateContentArgs, devContainer); err != nil {
		return err
	}

	debugln("Finished updateContentCommand")
	return nil
}

func executePostCreateCommand(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	postCreateArgs := devContainer.GetPostCreateCommandArgs()
	if len(postCreateArgs) == 0 {
		return nil
	}

	debugf("Running postCreateCommand: %s\n", strings.Join(postCreateArgs, " "))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for postCreateCommand: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			warnf("failed to close Docker client: %v", closeErr)
		}
	}()

	if err := executeCommandInContainer(ctx, cli, containerName, postCreateArgs, devContainer); err != nil {
		return err
	}

	debugln("Finished postCreateCommand")
	return nil
}

func executePostStartCommand(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	postStartArgs := devContainer.GetPostStartCommandArgs()
	if len(postStartArgs) == 0 {
		return nil
	}

	debugf("Running postStartCommand: %s\n", strings.Join(postStartArgs, " "))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for postStartCommand: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			warnf("failed to close Docker client: %v", closeErr)
		}
	}()

	if err := executeCommandInContainer(ctx, cli, containerName, postStartArgs, devContainer); err != nil {
		return err
	}

	debugln("Finished postStartCommand")
	return nil
}

func executePostAttachCommand(ctx context.Context, devContainer *devcontainer.DevContainer, containerName, workspaceDir string) error {
	postAttachArgs := devContainer.GetPostAttachCommandArgs()
	if len(postAttachArgs) == 0 {
		return nil
	}

	debugf("Running postAttachCommand: %s\n", strings.Join(postAttachArgs, " "))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for postAttachCommand: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			warnf("failed to close Docker client: %v", closeErr)
		}
	}()

	if err := executeCommandInContainer(ctx, cli, containerName, postAttachArgs, devContainer); err != nil {
		return err
	}

	debugln("Finished postAttachCommand")
	return nil
}

func executeInitializeCommand(devContainer *devcontainer.DevContainer, workspaceDir string) error {
	initArgs := devContainer.GetInitializeCommandArgs()
	if len(initArgs) == 0 {
		return nil
	}

	debugf("Running initializeCommand: %s\n", strings.Join(initArgs, " "))

	cmd := exec.Command(initArgs[0], initArgs[1:]...)
	cmd.Dir = workspaceDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("initializeCommand failed: %w", err)
	}

	debugln("Finished initializeCommand")

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
	debugf("Container '%s' started successfully\n", containerName)
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

			debugf("SSH agent forwarding enabled: %s -> %s\n",
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

	debugf("Container '%s' started successfully\n", args.Name)
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
			warnf("failed to close pull response: %v", closeErr)
		}
	}()

	// Read the response to ensure the pull completes
	_, err = io.Copy(io.Discard, resp)
	if err != nil {
		return fmt.Errorf("failed to read pull response: %w", err)
	}

	debugf("Image '%s' pulled successfully\n", imageName)
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

	debugf("Updating container user '%s' UID/GID to match host (%d:%d)\n", targetUser, hostUID, hostGID)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer func() {
		_ = cli.Close()
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
		warnf("failed to update remote user UID/GID: %v", err)
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
	debugf("Executing lifecycle commands up to: %s\n", waitFor)

	// Execute commands synchronously until waitFor
	for _, cmd := range commands {
		if devContainer.ShouldWaitForCommand(cmd.commandType) {
			if err := cmd.executor(ctx, devContainer, containerName, workspaceDir); err != nil {
				return fmt.Errorf("failed to execute %s: %w", cmd.commandType, err)
			}
		}
	}

	debugf("Container is ready for use (waitFor: %s completed)\n", waitFor)

	// Execute remaining commands asynchronously
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, cmd := range commands {
			if !devContainer.ShouldWaitForCommand(cmd.commandType) {
				if err := cmd.executor(ctx, devContainer, containerName, workspaceDir); err != nil {
					warnf("background command %s failed: %v", cmd.commandType, err)
				}
			}
		}

		// Always execute postAttachCommand last
		if err := executePostAttachCommand(ctx, devContainer, containerName, workspaceDir); err != nil {
			warnf("background postAttachCommand failed: %v", err)
		}
	}()

	wg.Wait()

	// Personal dotfiles run after every team-defined lifecycle command so
	// that team setup always completes first. Failures are logged but do
	// not fail the up command.
	if err := applyDotfiles(ctx, devContainer, containerName); err != nil {
		warnf("dotfiles step failed for container %s: %v", containerName, err)
	}

	return nil
}

// applyDotfiles loads the user's persistent dotfiles config, merges CLI
// overrides, and runs the clone/install workflow inside the container. It
// returns nil when dotfiles are disabled or unconfigured. All other errors
// (config load, missing container, clone, install) are returned to the
// caller, which logs them but continues so a broken personal setup never
// blocks the container from being usable.
func applyDotfiles(ctx context.Context, devContainer *devcontainer.DevContainer, containerName string) error {
	userConfig, err := config.LoadUserConfig()
	if err != nil {
		return fmt.Errorf("failed to load user config: %w", err)
	}

	override := dotfiles.Override{
		Repository:     dotfilesRepository,
		TargetPath:     dotfilesTargetPath,
		InstallCommand: dotfilesInstallCommand,
	}
	cfg := dotfiles.Resolve(userConfig.Dotfiles, override, noDotfiles)
	if cfg == nil {
		return nil
	}
	cfg.Logger = debugf

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client for dotfiles: %w", err)
	}
	defer func() {
		if closeErr := cli.Close(); closeErr != nil {
			warnf("failed to close Docker client: %v", closeErr)
		}
	}()

	containerID, err := findRunningContainer(ctx, cli, containerName)
	if err != nil {
		return fmt.Errorf("failed to find running container for dotfiles: %w", err)
	}
	if containerID == "" {
		return fmt.Errorf("container %s is not running, cannot apply dotfiles", containerName)
	}

	executor := newDotfilesExecutor(cli, containerID)
	user := devContainer.GetTargetUser()
	debugf("Checking dotfiles for container %s as user %s\n", containerName, user)
	return dotfiles.Apply(ctx, executor, user, cfg, forceDotfiles)
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

	// Create override file for containerEnv if needed
	if len(devContainer.ContainerEnv) > 0 {
		// Get base environment variables for expansion
		baseEnv, err := getComposeServiceEnv(workspaceDir, composeFiles, devContainer.GetService())
		if err != nil {
			warnf("failed to get base environment variables for compose service: %v", err)
			baseEnv = make(map[string]string)
		}

		expandedEnv := devContainer.GetContainerEnv(baseEnv)
		overrideFile, err := createComposeOverrideFile(devContainer.GetService(), expandedEnv)
		if err != nil {
			return fmt.Errorf("failed to create compose override file: %w", err)
		}
		if overrideFile != "" {
			defer func() {
				if err := os.Remove(overrideFile); err != nil {
					warnf("failed to remove temporary override file: %v", err)
				}
			}()
			composeArgs = append(composeArgs, "-f", overrideFile)
		}
	}

	// Determine which services to run
	runServices := devContainer.GetRunServices()
	if len(runServices) == 0 {
		runServices = []string{devContainer.GetService()}
	}

	// Start docker compose services
	upArgs := append(composeArgs, append([]string{"up", "-d"}, runServices...)...)
	upCmd := exec.Command("docker", append([]string{"compose"}, upArgs...)...)
	upCmd.Dir = workspaceDir
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr

	debugf("Starting docker compose services: %s\n", strings.Join(runServices, ", "))
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("failed to start docker compose services: %w", err)
	}

	debugf("Docker compose services started successfully\n")
	return executeLifecycleCommands(ctx, devContainer, containerName, workspaceDir)
}

func getImageEnv(ctx context.Context, imageName string) (map[string]string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = cli.Close()
	}()

	inspect, _, err := cli.ImageInspectWithRaw(ctx, imageName) //nolint:staticcheck // ImageInspectWithRaw is deprecated
	if err != nil {
		return nil, err
	}

	env := make(map[string]string)
	for _, e := range inspect.Config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env, nil
}

func getComposeServiceEnv(workspaceDir string, composeFiles []string, service string) (map[string]string, error) {
	// Use docker compose config to get the environment
	var args []string
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	args = append(args, "config", "--format", "json")

	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir = workspaceDir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Note: We could parse the JSON here to be more robust, but docker compose config
	// mainly shows what's in the YAML. To get the actual environment including
	// image defaults, we'd need to inspect the image.
	// For now, let's try to get the image name from the config and inspect it.

	// Simple heuristic to find image name for the service in the config output
	// A proper JSON parser would be better but let's keep it simple for now
	outputStr := string(output)
	imageMarker := fmt.Sprintf("\"%s\":", service)
	idx := strings.Index(outputStr, imageMarker)
	if idx == -1 {
		return nil, fmt.Errorf("service %s not found in compose config", service)
	}

	imageIdx := strings.Index(outputStr[idx:], "\"image\":")
	if imageIdx == -1 {
		return nil, fmt.Errorf("image not found for service %s", service)
	}

	start := idx + imageIdx + len("\"image\":")
	quoteStart := strings.Index(outputStr[start:], "\"")
	if quoteStart == -1 {
		return nil, fmt.Errorf("failed to parse image name")
	}
	quoteEnd := strings.Index(outputStr[start+quoteStart+1:], "\"")
	if quoteEnd == -1 {
		return nil, fmt.Errorf("failed to parse image name")
	}

	imageName := outputStr[start+quoteStart+1 : start+quoteStart+1+quoteEnd]
	return getImageEnv(context.Background(), imageName)
}

func createComposeOverrideFile(service string, env map[string]string) (string, error) {
	if len(env) == 0 {
		return "", nil
	}

	file, err := os.CreateTemp("", "devgo-compose-override-*.yml")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	var content strings.Builder
	content.WriteString("services:\n")
	fmt.Fprintf(&content, "  %s:\n", service)
	content.WriteString("    environment:\n")

	for k, v := range env {
		escapedVal := strings.ReplaceAll(v, "\\", "\\\\")
		escapedVal = strings.ReplaceAll(escapedVal, "\"", "\\\"")
		fmt.Fprintf(&content, "      %s: \"%s\"\n", k, escapedVal)
	}

	if _, err := file.WriteString(content.String()); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}

	return file.Name(), nil
}
