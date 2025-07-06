package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/garaemon/devgo/pkg/devcontainer"
)

func runBuildCommand(args []string) error {
	workspaceDir, err := getWorkspaceDirectory()
	if err != nil {
		return fmt.Errorf("failed to get workspace directory: %w", err)
	}

	devcontainerPath, err := findDevcontainerConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to find devcontainer config: %w", err)
	}

	if verbose {
		fmt.Printf("Using devcontainer config: %s\n", devcontainerPath)
	}

	devContainer, err := devcontainer.Parse(devcontainerPath)
	if err != nil {
		return fmt.Errorf("failed to parse devcontainer.json: %w", err)
	}

	if !devContainer.HasBuild() {
		return fmt.Errorf("devcontainer.json does not have build configuration")
	}

	return buildDevContainer(devContainer, workspaceDir, devcontainerPath)
}

func buildDevContainer(devContainer *devcontainer.DevContainer, workspaceDir, devcontainerPath string) error {
	dockerfilePath := determineDockerfilePath(devContainer, devcontainerPath)
	buildContext := determineBuildContext(devContainer, workspaceDir, devcontainerPath)

	imageTag := determineImageTag(devContainer, workspaceDir)

	if verbose {
		fmt.Printf("Building image: %s\n", imageTag)
		fmt.Printf("Dockerfile: %s\n", dockerfilePath)
		fmt.Printf("Build context: %s\n", buildContext)
	}

	buildArgs := []string{"build", "-t", imageTag, "-f", dockerfilePath}

	if devContainer.Build.Args != nil {
		for key, value := range devContainer.Build.Args {
			buildArgs = append(buildArgs, "--build-arg", fmt.Sprintf("%s=%v", key, value))
		}
	}

	buildArgs = append(buildArgs, buildContext)

	cmd := exec.Command("docker", buildArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if verbose {
		fmt.Printf("Running: docker %s\n", strings.Join(buildArgs, " "))
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	fmt.Printf("Successfully built image: %s\n", imageTag)

	if push {
		return pushImage(imageTag)
	}

	return nil
}

func determineDockerfilePath(devContainer *devcontainer.DevContainer, devcontainerPath string) string {
	if devContainer.Build.Dockerfile != "" {
		if filepath.IsAbs(devContainer.Build.Dockerfile) {
			return devContainer.Build.Dockerfile
		}
		return filepath.Join(filepath.Dir(devcontainerPath), devContainer.Build.Dockerfile)
	}
	return filepath.Join(filepath.Dir(devcontainerPath), "Dockerfile")
}

func determineBuildContext(devContainer *devcontainer.DevContainer, workspaceDir, devcontainerPath string) string {
	if devContainer.Build.Context != "" {
		if filepath.IsAbs(devContainer.Build.Context) {
			return devContainer.Build.Context
		}
		return filepath.Join(filepath.Dir(devcontainerPath), devContainer.Build.Context)
	}
	return workspaceDir
}

func determineImageTag(devContainer *devcontainer.DevContainer, workspaceDir string) string {
	if imageName != "" {
		return imageName
	}

	if devContainer.Name != "" {
		return fmt.Sprintf("devgo-%s:latest", devContainer.Name)
	}

	return fmt.Sprintf("devgo-%s:latest", filepath.Base(workspaceDir))
}

func pushImage(imageTag string) error {
	if verbose {
		fmt.Printf("Pushing image: %s\n", imageTag)
	}

	cmd := exec.Command("docker", "push", imageTag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker push failed: %w", err)
	}

	fmt.Printf("Successfully pushed image: %s\n", imageTag)
	return nil
}

func getWorkspaceDirectory() (string, error) {
	if workspaceFolder != "" {
		return workspaceFolder, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	return cwd, nil
}
