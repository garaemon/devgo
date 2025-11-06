package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/garaemon/devgo/pkg/constants"
)

func TestGetContainerName(t *testing.T) {
	tests := []struct {
		name     string
		names    []string
		expected string
	}{
		{
			name:     "normal container name",
			names:    []string{"/devgo-myproject"},
			expected: "devgo-myproject",
		},
		{
			name:     "multiple names",
			names:    []string{"/devgo-myproject", "/alias"},
			expected: "devgo-myproject",
		},
		{
			name:     "empty names",
			names:    []string{},
			expected: "<none>",
		},
		{
			name:     "name without slash",
			names:    []string{"devgo-myproject"},
			expected: "devgo-myproject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getContainerName(tt.names)
			if result != tt.expected {
				t.Errorf("getContainerName(%v) = %q, want %q", tt.names, result, tt.expected)
			}
		})
	}
}

func TestGetWorkspaceFromLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name: "workspace label exists",
			labels: map[string]string{
				constants.DevgoManagedLabel:   constants.DevgoManagedValue,
				constants.DevgoWorkspaceLabel: "/home/user/project",
			},
			expected: "/home/user/project",
		},
		{
			name: "workspace label missing",
			labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
			expected: "<unknown>",
		},
		{
			name:     "empty labels",
			labels:   map[string]string{},
			expected: "<unknown>",
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: "<unknown>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getWorkspaceFromLabels(tt.labels)
			if result != tt.expected {
				t.Errorf("getWorkspaceFromLabels(%v) = %q, want %q", tt.labels, result, tt.expected)
			}
		})
	}
}

func TestListCommandConstants(t *testing.T) {
	if constants.DevgoManagedLabel != "devgo.managed" {
		t.Errorf("DevgoManagedLabel = %q, want %q", constants.DevgoManagedLabel, "devgo.managed")
	}
	if constants.DevgoManagedValue != "true" {
		t.Errorf("DevgoManagedValue = %q, want %q", constants.DevgoManagedValue, "true")
	}
	if constants.DevgoWorkspaceLabel != "devgo.workspace" {
		t.Errorf("DevgoWorkspaceLabel = %q, want %q", constants.DevgoWorkspaceLabel, "devgo.workspace")
	}
}

// mockListClient implements a mock Docker client for testing list functionality
type mockListClient struct {
	containers []container.Summary
	listError  error
}

func (m *mockListClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	if m.listError != nil {
		return nil, m.listError
	}
	return m.containers, nil
}

func (m *mockListClient) Close() error {
	return nil
}

func TestListDevgoContainers(t *testing.T) {
	tests := []struct {
		name              string
		containers        []container.Summary
		listError         error
		expectedInOutput  []string
		shouldNotContain  []string
		expectError       bool
		expectEmptyOutput bool
	}{
		{
			name:             "no containers",
			containers:       []container.Summary{},
			expectedInOutput: []string{"No devgo containers found"},
		},
		{
			name: "single container",
			containers: []container.Summary{
				{
					ID:      "abc123",
					Names:   []string{"/test-container"},
					Image:   "ubuntu:22.04",
					Status:  "Up 2 minutes",
					Created: time.Date(2025, 6, 19, 10, 0, 0, 0, time.UTC).Unix(),
					Labels: map[string]string{
						constants.DevgoManagedLabel:   constants.DevgoManagedValue,
						constants.DevgoWorkspaceLabel: "/home/user/project",
						constants.DevgoSessionLabel:   "default",
					},
				},
			},
			expectedInOutput: []string{
				"NAME", "SESSION", "STATUS", "IMAGE", "CREATED", "WORKSPACE",
				"test-container", "default", "Up 2 minutes", "ubuntu:22.04", "2025-06-19", "/home/user/project",
			},
		},
		{
			name: "multiple containers",
			containers: []container.Summary{
				{
					ID:      "abc123",
					Names:   []string{"/test-container-1"},
					Image:   "ubuntu:22.04",
					Status:  "Up 2 minutes",
					Created: time.Date(2025, 6, 19, 10, 0, 0, 0, time.UTC).Unix(),
					Labels: map[string]string{
						constants.DevgoManagedLabel:   constants.DevgoManagedValue,
						constants.DevgoWorkspaceLabel: "/home/user/project1",
						constants.DevgoSessionLabel:   "session1",
					},
				},
				{
					ID:      "def456",
					Names:   []string{"/test-container-2"},
					Image:   "alpine:latest",
					Status:  "Exited (0) 1 hour ago",
					Created: time.Date(2025, 6, 19, 9, 0, 0, 0, time.UTC).Unix(),
					Labels: map[string]string{
						constants.DevgoManagedLabel:   constants.DevgoManagedValue,
						constants.DevgoWorkspaceLabel: "/home/user/project2",
						constants.DevgoSessionLabel:   "session2",
					},
				},
			},
			expectedInOutput: []string{
				"NAME", "SESSION", "STATUS", "IMAGE", "CREATED", "WORKSPACE",
				"test-container-1", "session1", "Up 2 minutes", "ubuntu:22.04", "2025-06-19", "/home/user/project1",
				"test-container-2", "session2", "Exited (0) 1 hour ago", "alpine:latest", "/home/user/project2",
			},
		},
		{
			name: "container with missing workspace label",
			containers: []container.Summary{
				{
					ID:      "abc123",
					Names:   []string{"/test-container"},
					Image:   "ubuntu:22.04",
					Status:  "Up 2 minutes",
					Created: time.Date(2025, 6, 19, 10, 0, 0, 0, time.UTC).Unix(),
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
			},
			expectedInOutput: []string{
				"NAME", "SESSION", "STATUS", "IMAGE", "CREATED", "WORKSPACE",
				"test-container", "<unknown>", "Up 2 minutes", "ubuntu:22.04", "2025-06-19",
			},
		},
		{
			name:        "docker client error",
			listError:   fmt.Errorf("docker daemon not running"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Create mock client
			mockClient := &mockListClient{
				containers: tt.containers,
				listError:  tt.listError,
			}

			// Test the function
			err := listDevgoContainers(context.Background(), mockClient)

			// Restore stdout and capture output
			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Check results
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

			// Check that all expected strings are in the output
			for _, expected := range tt.expectedInOutput {
				if !strings.Contains(output, expected) {
					t.Errorf("output missing expected string %q\noutput:\n%s", expected, output)
				}
			}

			// Check that unwanted strings are not in the output
			for _, unwanted := range tt.shouldNotContain {
				if strings.Contains(output, unwanted) {
					t.Errorf("output contains unwanted string %q\noutput:\n%s", unwanted, output)
				}
			}
		})
	}
}

func TestRunListCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "list command with no args",
			args:        []string{},
			expectError: false, // May fail if Docker is not available, but that's expected
		},
		{
			name:        "list command with extra args",
			args:        []string{"extra", "args"},
			expectError: false, // Function ignores extra args
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runListCommand(tt.args)

			// The function may fail if Docker is not available, which is acceptable
			// We're mainly testing that the function doesn't panic and handles args correctly
			if err != nil {
				t.Logf("runListCommand returned error (expected if Docker unavailable): %v", err)
				// Check that the error is related to Docker client creation
				if !containsAny(err.Error(), []string{"docker", "client", "connection", "daemon"}) {
					t.Errorf("unexpected error type: %v", err)
				}
			}
		})
	}
}

// containsAny checks if the string contains any of the given substrings
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(strings.ToLower(s), strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
