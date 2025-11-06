package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
)

// mockConn implements a basic net.Conn for testing
type mockConn struct {
	*bytes.Buffer
}

func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

// createMockHijackedResponse creates a mock HijackedResponse for testing
func createMockHijackedResponse() types.HijackedResponse {
	buf := &bytes.Buffer{}
	conn := &mockConn{Buffer: buf}
	reader := bufio.NewReader(bytes.NewReader([]byte("mock output")))

	return types.HijackedResponse{
		Conn:   conn,
		Reader: reader,
	}
}

// mockExecClient implements a mock Docker client for testing exec functionality
type mockExecClient struct {
	containers         []container.Summary
	listError          error
	execCreateResponse container.ExecCreateResponse
	execCreateError    error
	execAttachResponse types.HijackedResponse
	execAttachError    error
}

func (m *mockExecClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	if m.listError != nil {
		return nil, m.listError
	}
	return m.containers, nil
}

func (m *mockExecClient) ContainerExecCreate(ctx context.Context, containerID string, config container.ExecOptions) (container.ExecCreateResponse, error) {
	if m.execCreateError != nil {
		return container.ExecCreateResponse{}, m.execCreateError
	}
	return m.execCreateResponse, nil
}

func (m *mockExecClient) ContainerExecStart(ctx context.Context, execID string, config container.ExecStartOptions) error {
	return nil
}

func (m *mockExecClient) ContainerExecAttach(ctx context.Context, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error) {
	if m.execAttachError != nil {
		return types.HijackedResponse{}, m.execAttachError
	}
	return m.execAttachResponse, nil
}

func (m *mockExecClient) Close() error {
	return nil
}

func TestFindRunningContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		containers    []container.Summary
		listError     error
		expectedID    string
		expectError   bool
	}{
		{
			name:          "container found",
			containerName: "test-container",
			containers: []container.Summary{
				{
					ID:    "abc123",
					Names: []string{"/test-container"},
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
			},
			expectedID: "abc123",
		},
		{
			name:          "container not found",
			containerName: "missing-container",
			containers: []container.Summary{
				{
					ID:    "abc123",
					Names: []string{"/other-container"},
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
			},
			expectedID: "",
		},
		{
			name:          "multiple containers, find correct one",
			containerName: "target-container",
			containers: []container.Summary{
				{
					ID:    "abc123",
					Names: []string{"/other-container"},
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
				{
					ID:    "def456",
					Names: []string{"/target-container"},
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
			},
			expectedID: "def456",
		},
		{
			name:          "docker list error",
			containerName: "test-container",
			listError:     fmt.Errorf("docker daemon not running"),
			expectError:   true,
		},
		{
			name:          "empty containers list",
			containerName: "test-container",
			containers:    []container.Summary{},
			expectedID:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockExecClient{
				containers: tt.containers,
				listError:  tt.listError,
			}

			containerID, err := findRunningContainer(context.Background(), mockClient, tt.containerName)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if containerID != tt.expectedID {
				t.Errorf("findRunningContainer() = %q, want %q", containerID, tt.expectedID)
			}
		})
	}
}

func TestExecuteCommandInContainer(t *testing.T) {
	tests := []struct {
		name             string
		containerName    string
		args             []string
		devContainer     *devcontainer.DevContainer
		containers       []container.Summary
		execCreateResp   container.ExecCreateResponse
		execCreateError  error
		execAttachError  error
		expectError      bool
		expectedErrorMsg string
	}{
		{
			name:          "successful execution",
			containerName: "test-container",
			args:          []string{"bash", "-c", "echo hello"},
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
			args:          []string{"bash"},
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
			args:          []string{"bash"},
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
			args:          []string{"bash"},
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
			if tt.name == "successful execution" {
				// For successful execution, provide a proper mock response that won't panic
				buf := &bytes.Buffer{}
				// Write a simple Docker stream header + content for stdcopy
				// Docker stream format: [stream_type][0][0][0][payload_size][payload]
				buf.WriteByte(1)        // stdout stream type
				buf.WriteByte(0)        // padding
				buf.WriteByte(0)        // padding
				buf.WriteByte(0)        // padding
				buf.WriteByte(0)        // payload size (high byte)
				buf.WriteByte(0)        // payload size
				buf.WriteByte(0)        // payload size
				buf.WriteByte(4)        // payload size (4 bytes: "test")
				buf.WriteString("test") // payload

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

			err := executeCommandInContainer(context.Background(), mockClient, tt.containerName, tt.args, tt.devContainer)

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

func TestRunExecCommand_ArgValidation(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "no arguments",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "single argument",
			args:        []string{"bash"},
			expectError: false, // May fail due to missing devcontainer, but arg validation passes
		},
		{
			name:        "multiple arguments",
			args:        []string{"echo", "hello", "world"},
			expectError: false, // May fail due to missing devcontainer, but arg validation passes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runExecCommand(tt.args)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for empty args, got nil")
				} else if !strings.Contains(err.Error(), "requires at least one argument") {
					t.Errorf("expected 'requires at least one argument' error, got: %v", err)
				}
				return
			}

			// For non-empty args, we expect it to fail due to missing devcontainer or Docker
			// but not due to argument validation
			if err != nil && strings.Contains(err.Error(), "requires at least one argument") {
				t.Errorf("unexpected argument validation error: %v", err)
			}
		})
	}
}

func TestExecCommandContainerNameLogic(t *testing.T) {
	// Test that exec command follows the same container naming logic as other commands
	// This is more of an integration test to ensure consistency

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
				Name: "custom-dev-container",
			},
			expectedName: "custom-dev-container-" + GeneratePathHash(workspaceDir) + "-default",
		},
		{
			name:         "uses workspace directory name",
			devContainer: &devcontainer.DevContainer{},
			expectedName: "workspace-" + GeneratePathHash(workspaceDir) + "-default",
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
