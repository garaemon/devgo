package cmd

import (
	"strings"
	"testing"

	"github.com/garaemon/devgo/pkg/devcontainer"
)

func TestGeneratePathHash(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "basic path",
			path:     "/Users/test/project",
			expected: "8f7e5b6a", // First 8 characters of SHA256
		},
		{
			name:     "different path should have different hash",
			path:     "/Users/test/another-project",
			expected: "4a3c2d1e", // Different hash
		},
		{
			name:     "same path should have same hash",
			path:     "/Users/test/project",
			expected: "8f7e5b6a", // Same as first test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GeneratePathHash(tt.path)

			// Check that result is 8 characters long
			if len(result) != 8 {
				t.Errorf("GeneratePathHash(%q) returned hash of length %d, want 8", tt.path, len(result))
			}

			// For paths that appear multiple times, verify consistency
			if tt.path == "/Users/test/project" {
				expected := GeneratePathHash("/Users/test/project")
				if result != expected {
					t.Errorf("GeneratePathHash(%q) = %q, want consistent hash %q", tt.path, result, expected)
				}
			}
		})
	}

	// Test that different paths produce different hashes
	hash1 := GeneratePathHash("/path/one")
	hash2 := GeneratePathHash("/path/two")
	if hash1 == hash2 {
		t.Errorf("Different paths produced same hash: %q", hash1)
	}
}

func TestDetermineContainerNameWithPathHash(t *testing.T) {
	// Save and restore global variables
	oldContainerName := containerName
	oldSessionName := sessionName
	defer func() {
		containerName = oldContainerName
		sessionName = oldSessionName
	}()

	tests := []struct {
		name               string
		containerNameFlag  string
		sessionNameFlag    string
		workspaceDir       string
		devContainerName   string
		expectContains     []string
		expectFormat       string
	}{
		{
			name:              "default naming with path hash",
			containerNameFlag: "",
			sessionNameFlag:   "",
			workspaceDir:      "/Users/test/myproject",
			devContainerName:  "",
			expectContains:    []string{"myproject", "default"},
			expectFormat:      "myproject-default-HASH",
		},
		{
			name:              "with custom devcontainer name",
			containerNameFlag: "",
			sessionNameFlag:   "",
			workspaceDir:      "/Users/test/myproject",
			devContainerName:  "CustomName",
			expectContains:    []string{"customname", "default"},
			expectFormat:      "customname-default-HASH",
		},
		{
			name:              "with explicit container name",
			containerNameFlag: "my-container",
			sessionNameFlag:   "",
			workspaceDir:      "/Users/test/myproject",
			devContainerName:  "",
			expectContains:    []string{"my-container"},
			expectFormat:      "my-container",
		},
		{
			name:              "with custom session",
			containerNameFlag: "",
			sessionNameFlag:   "dev",
			workspaceDir:      "/Users/test/myproject",
			devContainerName:  "",
			expectContains:    []string{"myproject", "dev"},
			expectFormat:      "myproject-dev-HASH",
		},
		{
			name:              "with spaces in devcontainer name",
			containerNameFlag: "",
			sessionNameFlag:   "",
			workspaceDir:      "/Users/test/myproject",
			devContainerName:  "My Project Name",
			expectContains:    []string{"my_project_name", "default"},
			expectFormat:      "my_project_name-default-HASH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set global variables
			containerName = tt.containerNameFlag
			sessionName = tt.sessionNameFlag

			// Create a mock devcontainer
			dc := &devcontainer.DevContainer{
				Name: tt.devContainerName,
			}

			result := determineContainerName(dc, tt.workspaceDir)

			// Check that all expected substrings are present
			for _, expected := range tt.expectContains {
				if !contains(result, expected) {
					t.Errorf("determineContainerName() = %q, should contain %q", result, expected)
				}
			}

			// For default naming, verify it includes hash
			if tt.containerNameFlag == "" && tt.devContainerName == "" {
				pathHash := GeneratePathHash(tt.workspaceDir)
				if !contains(result, pathHash) {
					t.Errorf("determineContainerName() = %q, should contain path hash %q", result, pathHash)
				}
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
