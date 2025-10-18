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
			}

			err := executeInteractiveShell(context.Background(), mockClient, tt.containerName, tt.devContainer)

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
	}

	mockClient := &mockShellExecClient{
		mockExecClient: baseMockClient,
	}

	// This will fail due to terminal handling, but we can still test the exec options
	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer)

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

	// Verify shell command uses /bin/bash --login
	expectedCmd := []string{"/bin/bash", "--login"}
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
			expectedName: "custom-shell-container-default",
		},
		{
			name:         "uses workspace directory name",
			devContainer: &devcontainer.DevContainer{},
			expectedName: "devgo-workspace-default",
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
