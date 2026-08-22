package features

import (
	"fmt"
	"os/exec"
	"strings"
)

// ImageUserResolver reports the user an image is configured to run as, i.e. the
// value of its USER instruction. An empty string means the image does not set
// one and therefore runs as root.
type ImageUserResolver func(image string) (string, error)

// CommandImageUser returns a resolver backed by the given container runtime
// binary, e.g. "docker" or "podman".
func CommandImageUser(runtimeBinary string) ImageUserResolver {
	return func(image string) (string, error) {
		//nolint:gosec // runtimeBinary is devgo's configured container runtime
		out, err := exec.Command(runtimeBinary, "image", "inspect",
			"--format", "{{.Config.User}}", image).Output()
		if err != nil {
			return "", fmt.Errorf("failed to inspect image %q: %w", image, err)
		}
		return strings.TrimSpace(string(out)), nil
	}
}
