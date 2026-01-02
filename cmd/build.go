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

	// Add build arguments
	args := devContainer.GetBuildArgs()
	for key, value := range args {
		buildArgs = append(buildArgs, "--build-arg", fmt.Sprintf("%s=%v", key, value))
	}

	// Add target stage for multi-stage builds
	target := devContainer.GetBuildTarget()
	if target != "" {
		buildArgs = append(buildArgs, "--target", target)
	}

	// Add cache-from images
	cacheFrom := devContainer.GetBuildCacheFrom()
	for _, cache := range cacheFrom {
		buildArgs = append(buildArgs, "--cache-from", cache)
	}

	// Add additional build options
	options := devContainer.GetBuildOptions()
	if options != nil {
		buildArgs = append(buildArgs, options...)
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
	dockerfilePath := devContainer.GetDockerfilePath()
	if dockerfilePath == "" {
		return filepath.Join(filepath.Dir(devcontainerPath), "Dockerfile")
	}

	if filepath.IsAbs(dockerfilePath) {
		return dockerfilePath
	}
	return filepath.Join(filepath.Dir(devcontainerPath), dockerfilePath)
}

func determineBuildContext(devContainer *devcontainer.DevContainer, workspaceDir, devcontainerPath string) string {
	context := devContainer.GetBuildContext()
	if context == "" || context == "." {
		return filepath.Dir(devcontainerPath)
	}

	if filepath.IsAbs(context) {
		return context
	}
	return filepath.Join(filepath.Dir(devcontainerPath), context)
}

func determineImageTag(devContainer *devcontainer.DevContainer, workspaceDir string) string {
	if imageName != "" {
		return imageName
	}

	if devContainer.Name != "" {
		return fmt.Sprintf("devgo-%s:latest", sanitizeDockerName(devContainer.Name))
	}

	return fmt.Sprintf("devgo-%s:latest", sanitizeDockerName(filepath.Base(workspaceDir)))
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
