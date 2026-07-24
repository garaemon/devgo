package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/garaemon/devgo/pkg/config"
)

// runtimeOverride is populated by the global --runtime flag. When empty the
// runtime is resolved from the environment and user config instead.
var runtimeOverride string

// selectRuntime applies devgo's runtime-selection precedence to explicit
// inputs. It is kept separate from resolveContainerRuntime so the precedence
// rules can be unit-tested without touching global flag state or the
// filesystem. Precedence, highest first: flag, environment variable, user
// config, then the "docker" default.
func selectRuntime(flagVal, envVal, configVal string) string {
	raw := flagVal
	if raw == "" {
		raw = envVal
	}
	if raw == "" {
		raw = configVal
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return "docker"
	}
	return raw
}

// resolveContainerRuntime returns the container runtime devgo should drive,
// e.g. "docker" or "podman". It reads, in order of decreasing precedence, the
// --runtime flag, the DEVGO_CONTAINER_RUNTIME environment variable, and the
// containerRuntime field in ~/.config/devgo/config.json.
func resolveContainerRuntime() string {
	configVal := ""
	if cfg, err := config.LoadUserConfig(); err == nil && cfg != nil {
		configVal = cfg.ContainerRuntime
	}
	return selectRuntime(runtimeOverride, os.Getenv("DEVGO_CONTAINER_RUNTIME"), configVal)
}

// containerRuntimeBinary returns the executable name used for CLI-level
// operations (image build/push and compose). It is the resolved runtime name,
// so a value of "podman" runs the podman binary while "docker" runs docker.
func containerRuntimeBinary() string {
	return resolveContainerRuntime()
}

// isPodman reports whether the selected runtime is Podman.
func isPodman() bool {
	return resolveContainerRuntime() == "podman"
}

// ensurePodmanHost points the Docker SDK at Podman's API socket when the user
// selected the podman runtime but hasn't told the SDK where to connect. Podman
// exposes a Docker-compatible API over this socket, so once DOCKER_HOST refers
// to it the existing client.FromEnv calls work against Podman unchanged. An
// explicit DOCKER_HOST (or Podman's own CONTAINER_HOST) is always respected and
// left untouched.
func ensurePodmanHost() {
	if os.Getenv("DOCKER_HOST") != "" {
		return
	}
	if host := os.Getenv("CONTAINER_HOST"); host != "" {
		if err := os.Setenv("DOCKER_HOST", host); err != nil {
			warnf("failed to set DOCKER_HOST from CONTAINER_HOST: %v", err)
		}
		return
	}
	if sock := findPodmanSocket(podmanSocketCandidates(), queryPodmanMachineSocket); sock != "" {
		host := "unix://" + sock
		if err := os.Setenv("DOCKER_HOST", host); err != nil {
			warnf("failed to set DOCKER_HOST for Podman: %v", err)
			return
		}
		debugf("Using Podman socket %s\n", sock)
		return
	}
	debugln("No Podman API socket found; relying on the SDK default connection." +
		" Run 'podman system service' or 'systemctl --user start podman.socket' if connections fail.")
}

// findPodmanSocket returns the first existing socket among the well-known
// candidate paths, falling back to the socket reported by queryMachineSocket.
// The fallback covers macOS and Windows, where Podman runs inside a VM and
// exposes its API through a per-machine socket outside the candidate paths.
func findPodmanSocket(candidates []string, queryMachineSocket func() string) string {
	for _, sock := range candidates {
		if _, err := os.Stat(sock); err == nil {
			return sock
		}
	}
	if sock := queryMachineSocket(); sock != "" {
		if _, err := os.Stat(sock); err == nil {
			return sock
		}
	}
	return ""
}

// queryPodmanMachineSocket asks the podman CLI for the host-side API socket of
// the default podman machine. The socket lives under an unpredictable per-user
// temporary directory, so it cannot be probed as a static candidate path. It
// returns an empty string when no machine is configured or podman is missing.
func queryPodmanMachineSocket() string {
	out, err := exec.Command(containerRuntimeBinary(), "machine", "inspect",
		"--format", "{{.ConnectionInfo.PodmanSocket.Path}}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// podmanSocketCandidates lists the well-known locations of the Podman API
// socket, most specific first: the rootless per-user socket (via
// XDG_RUNTIME_DIR or the numeric uid) followed by the rootful system socket.
func podmanSocketCandidates() []string {
	var candidates []string
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "podman", "podman.sock"))
	}
	if uid := os.Getuid(); uid > 0 {
		candidates = append(candidates, fmt.Sprintf("/run/user/%d/podman/podman.sock", uid))
	}
	candidates = append(candidates, "/run/podman/podman.sock")
	return candidates
}
