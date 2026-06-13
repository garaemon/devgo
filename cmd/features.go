package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/garaemon/devgo/pkg/devcontainer"
	"github.com/garaemon/devgo/pkg/features"
)

// applyFeaturesIfNeeded builds an image that layers the devcontainer's features
// on top of baseImage. When no features are declared it returns baseImage
// unchanged. The returned image tag should be used in place of baseImage for
// subsequent container creation.
func applyFeaturesIfNeeded(
	devContainer *devcontainer.DevContainer,
	workspaceDir, devcontainerPath, baseImage string,
) (string, error) {
	if !devContainer.HasFeatures() {
		return baseImage, nil
	}

	if devContainer.HasDockerCompose() {
		warnf("features are not supported with docker compose configurations; skipping")
		return baseImage, nil
	}

	buildContext := filepath.Dir(devcontainerPath)

	result, err := features.ApplyFeatures(features.ApplyInput{
		BaseImage:    baseImage,
		Specs:        devContainer.GetFeatures(),
		BuildContext: buildContext,
	})
	if err != nil {
		return "", fmt.Errorf("failed to prepare features: %w", err)
	}
	defer result.Cleanup()

	featureTag := determineFeatureImageTag(devContainer, workspaceDir)

	buildArgs := []string{"build", "-t", featureTag, "-f", result.DockerfilePath, buildContext}
	debugf("Running: docker %s\n", strings.Join(buildArgs, " "))

	cmd := exec.Command("docker", buildArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker build (features) failed: %w", err)
	}

	debugf("Successfully built feature image: %s\n", featureTag)
	return featureTag, nil
}

// determineFeatureImageTag returns a deterministic tag for the feature-augmented
// image so that re-runs are idempotent and benefit from Docker's layer cache.
func determineFeatureImageTag(devContainer *devcontainer.DevContainer, workspaceDir string) string {
	base := devContainer.Name
	if base == "" {
		base = filepath.Base(workspaceDir)
	}
	return fmt.Sprintf("devgo-%s-features:latest", sanitizeDockerName(base))
}
