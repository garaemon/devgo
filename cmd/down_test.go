package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
)

// mockDownDockerClient implements a mock Docker client for down command testing
type mockDownDockerClient struct {
	containers        []container.Summary
	listError         error
	stopError         error
	removeError       error
	closeError        error
	stoppedContainers []string
	removedContainers []string
}

func (m *mockDownDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	if m.listError != nil {
		return nil, m.listError
	}
	return m.containers, nil
}

func (m *mockDownDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	if m.stopError != nil {
		return m.stopError
	}
	m.stoppedContainers = append(m.stoppedContainers, containerID)
	return nil
}

func (m *mockDownDockerClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	if m.removeError != nil {
		return m.removeError
	}
	m.removedContainers = append(m.removedContainers, containerID)
	return nil
}

func (m *mockDownDockerClient) Close() error {
	return m.closeError
}

func TestRunDownCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "down command with no args",
			args: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runDownCommand(tt.args)

			// The command may succeed if devcontainer.json exists (locally)
			// or fail if devcontainer.json doesn't exist (CI environment)
			// Both are acceptable behaviors for this test
			if err != nil {
				// If it fails, it should be due to devcontainer config issue
				if !containsSubstringDown(err.Error(), "failed to find devcontainer config") {
					t.Errorf("unexpected error: %v", err)
				}
			}
			// If err is nil, the command succeeded (found devcontainer, container doesn't exist)
		})
	}
}

func TestStopAndRemoveContainer(t *testing.T) {
	tests := []struct {
		name            string
		containerName   string
		containers      []container.Summary
		listError       error
		stopError       error
		removeError     error
		expectError     bool
		expectedStopped []string
		expectedRemoved []string
		errorContains   string
	}{
		{
			name:          "container does not exist",
			containerName: "test-container",
			containers:    []container.Summary{},
			expectError:   false,
		},
		{
			name:          "container exists and is running - stop and remove",
			containerName: "test-container",
			containers: []container.Summary{
				{
					ID:    "container123",
					Names: []string{"/test-container"},
					State: "running",
				},
			},
			expectError:     false,
			expectedStopped: []string{"container123"},
			expectedRemoved: []string{"container123"},
		},
		{
			name:          "container exists but is stopped - only remove",
			containerName: "test-container",
			containers: []container.Summary{
				{
					ID:    "container456",
					Names: []string{"/test-container"},
					State: "exited",
				},
			},
			expectError:     false,
			expectedStopped: []string{}, // Should not stop already stopped container
			expectedRemoved: []string{"container456"},
		},
		{
			name:          "container with multiple names",
			containerName: "test-container",
			containers: []container.Summary{
				{
					ID:    "container789",
					Names: []string{"/other-name", "/test-container", "/another-name"},
					State: "running",
				},
			},
			expectError:     false,
			expectedStopped: []string{"container789"},
			expectedRemoved: []string{"container789"},
		},
		{
			name:          "docker list error",
			containerName: "test-container",
			listError:     errors.New("docker daemon not available"),
			expectError:   true,
			errorContains: "failed to list containers",
		},
		{
			name:          "docker stop error",
			containerName: "test-container",
			containers: []container.Summary{
				{
					ID:    "container123",
					Names: []string{"/test-container"},
					State: "running",
				},
			},
			stopError:     errors.New("failed to stop"),
			expectError:   true,
			errorContains: "failed to stop container",
		},
		{
			name:          "docker remove error",
			containerName: "test-container",
			containers: []container.Summary{
				{
					ID:    "container123",
					Names: []string{"/test-container"},
					State: "exited",
				},
			},
			removeError:   errors.New("failed to remove"),
			expectError:   true,
			errorContains: "failed to remove container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockDownDockerClient{
				containers:        tt.containers,
				listError:         tt.listError,
				stopError:         tt.stopError,
				removeError:       tt.removeError,
				stoppedContainers: []string{},
				removedContainers: []string{},
			}

			ctx := context.Background()
			err := stopAndRemoveContainer(ctx, mockClient, tt.containerName)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !containsSubstringDown(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain '%s' but got '%s'", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			// Verify stop operations
			if len(mockClient.stoppedContainers) != len(tt.expectedStopped) {
				t.Errorf("expected %d containers to be stopped, got %d", len(tt.expectedStopped), len(mockClient.stoppedContainers))
			} else {
				for i, expected := range tt.expectedStopped {
					if mockClient.stoppedContainers[i] != expected {
						t.Errorf("expected container %s to be stopped, got %s", expected, mockClient.stoppedContainers[i])
					}
				}
			}

			// Verify remove operations
			if len(mockClient.removedContainers) != len(tt.expectedRemoved) {
				t.Errorf("expected %d containers to be removed, got %d", len(tt.expectedRemoved), len(mockClient.removedContainers))
			} else {
				for i, expected := range tt.expectedRemoved {
					if mockClient.removedContainers[i] != expected {
						t.Errorf("expected container %s to be removed, got %s", expected, mockClient.removedContainers[i])
					}
				}
			}
		})
	}
}

// Helper function to check if a string contains another string
func containsSubstringDown(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
