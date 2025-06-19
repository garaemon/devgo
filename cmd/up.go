package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	ContainerExists(name string) bool
	IsContainerRunning(name string) bool
	StartExistingContainer(name string) error
	CreateAndStartContainer(args DockerRunArgs) error
}

// realDockerClient implements DockerClient using actual docker commands
type realDockerClient struct{}

func newRealDockerClient() DockerClient {
	return &realDockerClient{}
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
	dockerClient := newRealDockerClient()
	
	return startContainerWithDocker(devContainer, containerName, workspaceDir, dockerClient)
}

func determineWorkspaceFolder() string {
	if *workspaceFolder != "" {
		return *workspaceFolder
	}
	cwd, _ := os.Getwd()
	return cwd
}

func determineContainerName(devContainer *devcontainer.DevContainer, workspaceDir string) string {
	if *containerName != "" {
		return *containerName
	}
	if devContainer.Name != "" {
		return devContainer.Name
	}
	return fmt.Sprintf("devgo-%s", filepath.Base(workspaceDir))
}

func startContainerWithDocker(devContainer *devcontainer.DevContainer, containerName, workspaceDir string, dockerClient DockerClient) error {
	if !devContainer.HasImage() {
		return fmt.Errorf("devcontainer must specify an image")
	}

	// TODO: Add support for --container-name option similar to devcontainer-cli runArgs
	// The official devcontainer-cli doesn't have a direct --name option for the up command,
	// but supports container naming through runArgs in devcontainer.json.
	// We should consider adding a --container-name option for command-line convenience.

	// Check if container already exists
	if dockerClient.ContainerExists(containerName) {
		if dockerClient.IsContainerRunning(containerName) {
			return fmt.Errorf("container '%s' is already running", containerName)
		}
		fmt.Printf("Container '%s' exists but is stopped, starting it\n", containerName)
		return dockerClient.StartExistingContainer(containerName)
	}

	fmt.Printf("Creating and starting container '%s' with image '%s'\n", containerName, devContainer.Image)

	dockerArgs := DockerRunArgs{
		Name:            containerName,
		Image:           devContainer.Image,
		WorkspaceDir:    workspaceDir,
		WorkspaceFolder: devContainer.GetWorkspaceFolder(),
		Env:             devContainer.ContainerEnv,
	}

	return dockerClient.CreateAndStartContainer(dockerArgs)
}

// realDockerClient methods
func (r *realDockerClient) ContainerExists(containerName string) bool {
	cmd := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("name=^%s$", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == containerName
}

func (r *realDockerClient) IsContainerRunning(containerName string) bool {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=^%s$", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == containerName
}

func (r *realDockerClient) StartExistingContainer(containerName string) error {
	cmd := exec.Command("docker", "start", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start existing container: %w\nOutput: %s", err, string(output))
	}
	fmt.Printf("Container '%s' started successfully\n", containerName)
	return nil
}

func (r *realDockerClient) CreateAndStartContainer(args DockerRunArgs) error {
	dockerArgs := []string{
		"run", "-d",
		"--name", args.Name,
		"-v", fmt.Sprintf("%s:%s", args.WorkspaceDir, args.WorkspaceFolder),
	}

	if args.Env != nil {
		for key, value := range args.Env {
			dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", key, value))
		}
	}

	dockerArgs = append(dockerArgs, args.Image, "sleep", "infinity")

	cmd := exec.Command("docker", dockerArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start container: %w\nOutput: %s", err, string(output))
	}

	fmt.Printf("Container '%s' started successfully\n", args.Name)
	return nil
}