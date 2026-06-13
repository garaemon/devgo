package features

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/garaemon/devgo/pkg/devcontainer"
)

// featuresSubdir is the directory (relative to the build context) into which
// feature artifacts are extracted and from which the generated Dockerfile COPYs.
const featuresSubdir = ".devgo-features"

// ApplyInput describes the inputs needed to apply features to a base image.
type ApplyInput struct {
	// BaseImage is the image to layer features on top of.
	BaseImage string
	// Specs are the features to install.
	Specs []devcontainer.FeatureSpec
	// BuildContext is the directory used as the Docker build context. Feature
	// files are written under BuildContext/.devgo-features so that COPY can reach
	// them.
	BuildContext string
}

// ApplyResult describes the generated build artifacts.
type ApplyResult struct {
	// DockerfilePath is the path to the generated wrapper Dockerfile.
	DockerfilePath string
	// CleanupDir is the directory that should be removed after the build.
	CleanupDir string
}

// Cleanup removes the temporary feature directory.
func (r *ApplyResult) Cleanup() {
	if r == nil || r.CleanupDir == "" {
		return
	}
	_ = os.RemoveAll(r.CleanupDir)
}

// ApplyFeatures pulls each feature, extracts it under the build context, and
// generates a wrapper Dockerfile that installs the features on top of BaseImage.
func ApplyFeatures(in ApplyInput) (*ApplyResult, error) {
	if len(in.Specs) == 0 {
		return nil, fmt.Errorf("no features to apply")
	}

	featuresDir := filepath.Join(in.BuildContext, featuresSubdir)
	if err := os.MkdirAll(featuresDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create features directory: %w", err)
	}

	var pulled []*PulledFeature
	optionValues := make(map[string]map[string]string)

	for _, spec := range in.Specs {
		ref, err := ParseOCIRef(spec.Ref)
		if err != nil {
			return nil, err
		}

		pf, err := Pull(ref, featuresDir)
		if err != nil {
			return nil, err
		}

		pulled = append(pulled, pf)
		optionValues[ref.Raw] = pf.Metadata.ResolveOptionValues(spec.Options)
	}

	dockerfile := GenerateWrapperDockerfile(in.BaseImage, pulled, featuresSubdir, optionValues)
	dockerfilePath := filepath.Join(featuresDir, "Dockerfile.devgo")
	//nolint:gosec // generated build artifact, not sensitive
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write generated Dockerfile: %w", err)
	}

	return &ApplyResult{DockerfilePath: dockerfilePath, CleanupDir: featuresDir}, nil
}
