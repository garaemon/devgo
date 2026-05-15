package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/garaemon/devgo/pkg/config"
	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
)

func TestExecuteInteractiveShell(t *testing.T) {
	tests := []struct {
		name             string
		containerName    string
		devContainer     *devcontainer.DevContainer
		containers       []container.Summary
		execCreateResp   container.ExecCreateResponse
		execCreateError  error
		execAttachError  error
		inspectResponse  types.ContainerJSON
		expectError      bool
		expectedErrorMsg string
	}{
		{
			name:          "successful shell execution",
			containerName: "test-container",
			devContainer: &devcontainer.DevContainer{
				ContainerUser:   "root",
				WorkspaceFolder: "/workspace",
			},
			containers: []container.Summary{
				{
					ID:    "abc123",
					Names: []string{"/test-container"},
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
			},
			inspectResponse: types.ContainerJSON{
				Config: &container.Config{
					Env: []string{"PATH=/usr/bin"},
				},
			},
			execCreateResp: container.ExecCreateResponse{
				ID: "exec123",
			},
		},
		{
			name:          "container not running",
			containerName: "missing-container",
			devContainer: &devcontainer.DevContainer{
				ContainerUser:   "root",
				WorkspaceFolder: "/workspace",
			},
			containers:       []container.Summary{},
			expectError:      true,
			expectedErrorMsg: "is not running",
		},
		{
			name:          "exec create error",
			containerName: "test-container",
			devContainer: &devcontainer.DevContainer{
				ContainerUser:   "root",
				WorkspaceFolder: "/workspace",
			},
			containers: []container.Summary{
				{
					ID:    "abc123",
					Names: []string{"/test-container"},
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
			},
			inspectResponse: types.ContainerJSON{
				Config: &container.Config{
					Env: []string{"PATH=/usr/bin"},
				},
			},
			execCreateError:  fmt.Errorf("failed to create exec"),
			expectError:      true,
			expectedErrorMsg: "failed to create exec instance",
		},
		{
			name:          "exec attach error",
			containerName: "test-container",
			devContainer: &devcontainer.DevContainer{
				ContainerUser:   "root",
				WorkspaceFolder: "/workspace",
			},
			containers: []container.Summary{
				{
					ID:    "abc123",
					Names: []string{"/test-container"},
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
			},
			execCreateResp: container.ExecCreateResponse{
				ID: "exec123",
			},
			inspectResponse: types.ContainerJSON{
				Config: &container.Config{
					Env: []string{"PATH=/usr/bin"},
				},
			},
			execAttachError:  fmt.Errorf("failed to attach"),
			expectError:      true,
			expectedErrorMsg: "failed to attach to exec instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mockResp types.HijackedResponse
			if tt.name == "successful shell execution" {
				// For successful execution, provide a proper mock response
				buf := &bytes.Buffer{}
				buf.WriteString("root@abc123:/workspace# ")

				mockResp = types.HijackedResponse{
					Conn:   &mockConn{Buffer: &bytes.Buffer{}},
					Reader: bufio.NewReader(buf),
				}
			} else {
				mockResp = createMockHijackedResponse()
			}

			mockClient := &mockExecClient{
				containers:         tt.containers,
				execCreateResponse: tt.execCreateResp,
				execCreateError:    tt.execCreateError,
				execAttachResponse: mockResp,
				execAttachError:    tt.execAttachError,
				inspectResponse:    tt.inspectResponse,
			}

			err := executeInteractiveShell(context.Background(), mockClient, tt.containerName, tt.devContainer, []string{"/bin/bash", "-i"})

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.expectedErrorMsg != "" && !strings.Contains(err.Error(), tt.expectedErrorMsg) {
					t.Errorf("error message %q does not contain %q", err.Error(), tt.expectedErrorMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveShellCommand(t *testing.T) {
	tests := []struct {
		name     string
		override string
		userConfig  *config.UserConfig
		want     []string
	}{
		{
			name:     "default when nothing set",
			override: "",
			userConfig:  nil,
			want:     []string{"/bin/bash", "-i"},
		},
		{
			name:     "user config wins over default",
			override: "",
			userConfig:  &config.UserConfig{Shell: "zsh"},
			want:     []string{"zsh", "-i"},
		},
		{
			name:     "CLI override wins over user config",
			override: "/usr/bin/fish",
			userConfig:  &config.UserConfig{Shell: "zsh"},
			want:     []string{"/usr/bin/fish", "-i"},
		},
		{
			name:     "CLI override with nil user config",
			override: "zsh",
			userConfig:  nil,
			want:     []string{"zsh", "-i"},
		},
		{
			name:     "empty user config Shell falls back to default",
			override: "",
			userConfig:  &config.UserConfig{Shell: ""},
			want:     []string{"/bin/bash", "-i"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveShellCommand(tt.override, tt.userConfig)
			if len(got) != len(tt.want) {
				t.Fatalf("resolveShellCommand() = %v, want %v", got, tt.want)
			}
			for i, v := range tt.want {
				if got[i] != v {
					t.Errorf("resolveShellCommand()[%d] = %q, want %q", i, got[i], v)
				}
			}
		})
	}
}

func TestShellCommand_FallsBackToContainerUser(t *testing.T) {
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "node",
		WorkspaceFolder: "/workspace",
	}
	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}
	baseMockClient := &mockExecClient{
		containers:         containers,
		execCreateResponse: container.ExecCreateResponse{ID: "exec123"},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{Env: []string{"PATH=/usr/bin"}},
		},
	}
	mockClient := &mockShellExecClient{mockExecClient: baseMockClient}

	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer, []string{"/bin/bash", "-i"})

	if mockClient.capturedExecOptions.User != "node" {
		t.Errorf("expected shell to fall back to containerUser %q, got %q", "node", mockClient.capturedExecOptions.User)
	}
}

func TestShellCommand_PrefersRemoteUser(t *testing.T) {
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "root",
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspace",
	}
	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}
	baseMockClient := &mockExecClient{
		containers:         containers,
		execCreateResponse: container.ExecCreateResponse{ID: "exec123"},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{Env: []string{"PATH=/usr/bin"}},
		},
	}
	mockClient := &mockShellExecClient{mockExecClient: baseMockClient}

	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer, []string{"/bin/bash", "-i"})

	if mockClient.capturedExecOptions.User != "vscode" {
		t.Errorf("expected shell to run as remoteUser %q, got %q", "vscode", mockClient.capturedExecOptions.User)
	}
}

func TestShellCommand_UsesResolvedShell(t *testing.T) {
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "testuser",
		WorkspaceFolder: "/workspace",
	}
	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}
	baseMockClient := &mockExecClient{
		containers:         containers,
		execCreateResponse: container.ExecCreateResponse{ID: "exec123"},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{Env: []string{"PATH=/usr/bin"}},
		},
	}
	mockClient := &mockShellExecClient{mockExecClient: baseMockClient}

	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer, []string{"zsh", "-i"})

	got := mockClient.capturedExecOptions.Cmd
	want := []string{"zsh", "-i"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Cmd = %v, want %v", got, want)
	}
}

func TestRunShellCommand_NoArgs(t *testing.T) {
	// Shell command should not require arguments (unlike exec)
	err := runShellCommand([]string{})

	// Should fail due to missing devcontainer config, not due to argument validation
	if err != nil && strings.Contains(err.Error(), "requires at least one argument") {
		t.Errorf("shell command should not require arguments, got: %v", err)
	}

	// Should fail with devcontainer config error instead
	if err == nil {
		t.Errorf("expected error due to missing devcontainer config, got nil")
	} else if !strings.Contains(err.Error(), "devcontainer config") {
		// This is expected - should fail on devcontainer config lookup
		t.Logf("Expected failure due to missing devcontainer config: %v", err)
	}
}

// mockShellExecClient extends mockExecClient to capture exec options
type mockShellExecClient struct {
	*mockExecClient
	capturedExecOptions container.ExecOptions
}

func (m *mockShellExecClient) ContainerExecCreate(ctx context.Context, containerID string, config container.ExecOptions) (container.ExecCreateResponse, error) {
	m.capturedExecOptions = config
	return m.mockExecClient.ContainerExecCreate(ctx, containerID, config)
}

func TestShellCommandExecOptions(t *testing.T) {
	// Test that shell command uses correct exec options
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "testuser",
		WorkspaceFolder: "/test-workspace",
	}

	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}

	baseMockClient := &mockExecClient{
		containers: containers,
		execCreateResponse: container.ExecCreateResponse{
			ID: "exec123",
		},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{
				Env: []string{"PATH=/usr/bin"},
			},
		},
	}

	mockClient := &mockShellExecClient{
		mockExecClient: baseMockClient,
	}

	// This will fail due to terminal handling, but we can still test the exec options
	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer, []string{"/bin/bash", "-i"})

	// Verify exec options are set correctly for shell command
	capturedExecOptions := mockClient.capturedExecOptions
	if capturedExecOptions.User != "testuser" {
		t.Errorf("expected User to be 'testuser', got %q", capturedExecOptions.User)
	}
	if capturedExecOptions.WorkingDir != "/test-workspace" {
		t.Errorf("expected WorkingDir to be '/test-workspace', got %q", capturedExecOptions.WorkingDir)
	}
	if !capturedExecOptions.Tty {
		t.Errorf("expected Tty to be true for shell command")
	}
	if !capturedExecOptions.AttachStdin {
		t.Errorf("expected AttachStdin to be true for shell command")
	}
	if !capturedExecOptions.AttachStdout {
		t.Errorf("expected AttachStdout to be true for shell command")
	}
	if !capturedExecOptions.AttachStderr {
		t.Errorf("expected AttachStderr to be true for shell command")
	}

	// Verify shell command uses /bin/bash -i for interactive shell
	expectedCmd := []string{"/bin/bash", "-i"}
	if len(capturedExecOptions.Cmd) != len(expectedCmd) {
		t.Errorf("expected Cmd to be %v, got %v", expectedCmd, capturedExecOptions.Cmd)
	} else {
		for i, cmd := range expectedCmd {
			if capturedExecOptions.Cmd[i] != cmd {
				t.Errorf("expected Cmd[%d] to be %q, got %q", i, cmd, capturedExecOptions.Cmd[i])
			}
		}
	}
}

func TestShellCommandContainerNameLogic(t *testing.T) {
	// Test that shell command follows the same container naming logic as other commands
	workspaceDir := "/test/workspace"

	tests := []struct {
		name          string
		devContainer  *devcontainer.DevContainer
		containerName string
		expectedName  string
	}{
		{
			name: "uses devcontainer name",
			devContainer: &devcontainer.DevContainer{
				Name: "custom-shell-container",
			},
			expectedName: "custom-shell-container-default-" + GeneratePathHash(workspaceDir),
		},
		{
			name:         "uses workspace directory name",
			devContainer: &devcontainer.DevContainer{},
			expectedName: "workspace-default-" + GeneratePathHash(workspaceDir),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original global variable
			originalContainerName := containerName
			defer func() {
				containerName = originalContainerName
			}()

			containerName = tt.containerName
			result := determineContainerName(tt.devContainer, workspaceDir)

			if result != tt.expectedName {
				t.Errorf("determineContainerName() = %q, want %q", result, tt.expectedName)
			}
		})
	}
}

func TestShellRespectsBashrc(t *testing.T) {
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "testuser",
		WorkspaceFolder: "/workspace",
	}

	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}

	baseMockClient := &mockExecClient{
		containers: containers,
		execCreateResponse: container.ExecCreateResponse{
			ID: "exec123",
		},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{
				Env: []string{"PATH=/usr/bin"},
			},
		},
	}

	mockClient := &mockShellExecClient{
		mockExecClient: baseMockClient,
	}

	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer, []string{"/bin/bash", "-i"})

	capturedExecOptions := mockClient.capturedExecOptions

	// Verify PS1 is NOT set in environment to respect container's .bashrc
	// This aligns with README.md documentation about respecting .bashrc configuration
	for _, env := range capturedExecOptions.Env {
		if strings.HasPrefix(env, "PS1=") {
			t.Errorf("PS1 should not be set in environment to respect .bashrc, but found: %q", env)
		}
	}

	// Verify /bin/bash -i is used for interactive shell
	expectedCmd := []string{"/bin/bash", "-i"}
	if len(capturedExecOptions.Cmd) != len(expectedCmd) {
		t.Errorf("expected Cmd to be %v for proper .bashrc sourcing, got %v", expectedCmd, capturedExecOptions.Cmd)
	} else {
		for i, cmd := range expectedCmd {
			if capturedExecOptions.Cmd[i] != cmd {
				t.Errorf("expected Cmd[%d] to be %q for proper .bashrc sourcing, got %q", i, cmd, capturedExecOptions.Cmd[i])
			}
		}
	}
}
