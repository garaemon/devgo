package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectRuntime(t *testing.T) {
	tests := []struct {
		name      string
		flagVal   string
		envVal    string
		configVal string
		want      string
	}{
		{
			name: "defaults to docker when nothing is set",
			want: "docker",
		},
		{
			name:    "flag selects podman",
			flagVal: "podman",
			want:    "podman",
		},
		{
			name:      "flag beats env and config",
			flagVal:   "podman",
			envVal:    "docker",
			configVal: "docker",
			want:      "podman",
		},
		{
			name:      "env beats config",
			envVal:    "podman",
			configVal: "docker",
			want:      "podman",
		},
		{
			name:      "config used when flag and env empty",
			configVal: "podman",
			want:      "podman",
		},
		{
			name:    "value is lowercased and trimmed",
			flagVal: "  PodMan  ",
			want:    "podman",
		},
		{
			name:    "unknown runtime passes through for custom binaries",
			flagVal: "nerdctl",
			want:    "nerdctl",
		},
		{
			name:    "whitespace-only value falls back to default",
			flagVal: "   ",
			want:    "docker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectRuntime(tt.flagVal, tt.envVal, tt.configVal); got != tt.want {
				t.Errorf("selectRuntime(%q, %q, %q) = %q, want %q",
					tt.flagVal, tt.envVal, tt.configVal, got, tt.want)
			}
		})
	}
}

// withCleanRuntimeEnv isolates the runtime-selection inputs so tests do not
// leak into one another or depend on the developer's environment.
func withCleanRuntimeEnv(t *testing.T) {
	t.Helper()
	prev := runtimeOverride
	runtimeOverride = ""
	t.Cleanup(func() { runtimeOverride = prev })

	// Pin the config lookup at an empty temp dir so no real user config is read.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEVGO_CONTAINER_RUNTIME", "")
}

func TestResolveContainerRuntime_DefaultDocker(t *testing.T) {
	withCleanRuntimeEnv(t)

	if got := resolveContainerRuntime(); got != "docker" {
		t.Errorf("resolveContainerRuntime() = %q, want %q", got, "docker")
	}
	if isPodman() {
		t.Errorf("isPodman() = true, want false for default runtime")
	}
}

func TestResolveContainerRuntime_EnvSelectsPodman(t *testing.T) {
	withCleanRuntimeEnv(t)
	t.Setenv("DEVGO_CONTAINER_RUNTIME", "podman")

	if got := resolveContainerRuntime(); got != "podman" {
		t.Errorf("resolveContainerRuntime() = %q, want %q", got, "podman")
	}
	if !isPodman() {
		t.Errorf("isPodman() = false, want true")
	}
}

func TestResolveContainerRuntime_FlagOverridesEnv(t *testing.T) {
	withCleanRuntimeEnv(t)
	t.Setenv("DEVGO_CONTAINER_RUNTIME", "docker")
	runtimeOverride = "podman"

	if got := resolveContainerRuntime(); got != "podman" {
		t.Errorf("resolveContainerRuntime() = %q, want %q", got, "podman")
	}
}

func TestResolveContainerRuntime_UserConfig(t *testing.T) {
	withCleanRuntimeEnv(t)
	dir := filepath.Join(t.TempDir(), "devgo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"),
		[]byte(`{"containerRuntime": "podman"}`), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(dir))

	if got := resolveContainerRuntime(); got != "podman" {
		t.Errorf("resolveContainerRuntime() = %q, want %q", got, "podman")
	}
}

func TestEnsurePodmanHost_RespectsExistingDockerHost(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://example:2375")
	ensurePodmanHost()
	if got := os.Getenv("DOCKER_HOST"); got != "tcp://example:2375" {
		t.Errorf("DOCKER_HOST = %q, want it left unchanged", got)
	}
}

func TestEnsurePodmanHost_UsesContainerHost(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("CONTAINER_HOST", "unix:///tmp/custom-podman.sock")
	ensurePodmanHost()
	if got := os.Getenv("DOCKER_HOST"); got != "unix:///tmp/custom-podman.sock" {
		t.Errorf("DOCKER_HOST = %q, want it derived from CONTAINER_HOST", got)
	}
}

func TestFindPodmanSocket_FallsBackToMachineSocket(t *testing.T) {
	// On macOS the Podman API socket lives under the podman-machine state
	// directory, not under the Linux-style candidate paths. devgo must fall
	// back to the socket path reported by the podman CLI in that case.
	machineSock := filepath.Join(t.TempDir(), "podman-machine-default-api.sock")
	if err := os.WriteFile(machineSock, []byte{}, 0o600); err != nil {
		t.Fatalf("failed to create fake machine socket: %v", err)
	}
	missingCandidates := []string{
		filepath.Join(t.TempDir(), "missing", "podman.sock"),
	}

	got := findPodmanSocket(missingCandidates, func() string { return machineSock })

	if got != machineSock {
		t.Errorf("findPodmanSocket() = %q, want %q", got, machineSock)
	}
}

func TestFindPodmanSocket_PrefersCandidateOverMachineSocket(t *testing.T) {
	candidateSock := filepath.Join(t.TempDir(), "podman.sock")
	if err := os.WriteFile(candidateSock, []byte{}, 0o600); err != nil {
		t.Fatalf("failed to create fake candidate socket: %v", err)
	}

	got := findPodmanSocket([]string{candidateSock}, func() string {
		t.Error("queryMachineSocket should not be called when a candidate exists")
		return ""
	})

	if got != candidateSock {
		t.Errorf("findPodmanSocket() = %q, want %q", got, candidateSock)
	}
}

func TestFindPodmanSocket_ReturnsEmptyWhenNothingExists(t *testing.T) {
	missingCandidates := []string{
		filepath.Join(t.TempDir(), "missing", "podman.sock"),
	}
	missingMachineSock := filepath.Join(t.TempDir(), "missing-machine.sock")

	got := findPodmanSocket(missingCandidates, func() string { return missingMachineSock })

	if got != "" {
		t.Errorf("findPodmanSocket() = %q, want empty string", got)
	}
}

func TestShouldForwardSSHAgent(t *testing.T) {
	tests := []struct {
		name        string
		runtimeName string
		goos        string
		want        bool
	}{
		{
			name:        "docker on linux forwards",
			runtimeName: "docker",
			goos:        "linux",
			want:        true,
		},
		{
			name:        "docker on darwin forwards",
			runtimeName: "docker",
			goos:        "darwin",
			want:        true,
		},
		{
			name:        "podman on linux forwards",
			runtimeName: "podman",
			goos:        "linux",
			want:        true,
		},
		{
			name:        "podman on darwin skips because of the VM boundary",
			runtimeName: "podman",
			goos:        "darwin",
			want:        false,
		},
		{
			name:        "podman on windows skips because of the VM boundary",
			runtimeName: "podman",
			goos:        "windows",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldForwardSSHAgent(tt.runtimeName, tt.goos); got != tt.want {
				t.Errorf("shouldForwardSSHAgent(%q, %q) = %v, want %v",
					tt.runtimeName, tt.goos, got, tt.want)
			}
		})
	}
}

func TestEnsurePodmanHost_DetectsSocketFile(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("CONTAINER_HOST", "")

	runtimeDir := t.TempDir()
	sockDir := filepath.Join(runtimeDir, "podman")
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		t.Fatalf("failed to create socket dir: %v", err)
	}
	sockPath := filepath.Join(sockDir, "podman.sock")
	if err := os.WriteFile(sockPath, []byte{}, 0o600); err != nil {
		t.Fatalf("failed to create fake socket: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	ensurePodmanHost()

	want := "unix://" + sockPath
	if got := os.Getenv("DOCKER_HOST"); got != want {
		t.Errorf("DOCKER_HOST = %q, want %q", got, want)
	}
}
