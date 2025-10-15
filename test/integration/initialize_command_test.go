package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitializeCommandIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	tests := []struct {
		name                string
		devcontainerContent string
		setupFiles          func(workspaceDir string) error
		expectError         bool
		validateResults     func(t *testing.T, workspaceDir string, output string)
	}{
		{
			name: "initialize command writes to workspace",
			devcontainerContent: `{
  "image": "alpine:3.19",
  "initializeCommand": "echo 'Initialized' > init-output.txt"
}`,
			setupFiles: func(workspaceDir string) error {
				return nil
			},
			expectError: false,
			validateResults: func(t *testing.T, workspaceDir string, output string) {
				outputFile := filepath.Join(workspaceDir, "init-output.txt")
				if _, err := os.Stat(outputFile); os.IsNotExist(err) {
					t.Error("Expected output file to be created in workspace")
				} else if err == nil {
					content, readErr := os.ReadFile(outputFile)
					if readErr != nil {
						t.Errorf("Failed to read output file: %v", readErr)
					} else if strings.TrimSpace(string(content)) != "Initialized" {
						t.Errorf("Expected 'Initialized', got '%s'", strings.TrimSpace(string(content)))
					}
				}
			},
		},
		{
			name: "no initialize command runs normally",
			devcontainerContent: `{
  "image": "alpine:3.19"
}`,
			setupFiles: func(workspaceDir string) error {
				return nil
			},
			expectError: false,
			validateResults: func(t *testing.T, workspaceDir string, output string) {
				// Should not contain initialize command output
				if strings.Contains(output, "Running initializeCommand") {
					t.Error("Should not run initialize command when none specified")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary workspace
			tempDir := t.TempDir()

			// Create .devcontainer directory
			devcontainerDir := filepath.Join(tempDir, ".devcontainer")
			if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
				t.Fatalf("Failed to create .devcontainer directory: %v", err)
			}

			// Write devcontainer.json
			devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
			if err := os.WriteFile(devcontainerPath, []byte(tt.devcontainerContent), 0644); err != nil {
				t.Fatalf("Failed to create devcontainer.json: %v", err)
			}

			// Setup additional files if needed
			if err := tt.setupFiles(tempDir); err != nil {
				t.Fatalf("Failed to setup test files: %v", err)
			}

			// Build devgo binary
			devgoBinary := buildDevgoBinary(t)
			// Only remove if it's a temporary binary (not a Bazel pre-built binary)
			if !strings.Contains(devgoBinary, "bazel") {
				defer os.Remove(devgoBinary)
			}

			// Pre-cleanup any existing containers
			containerName := "devgo-" + filepath.Base(tempDir)
			cleanupContainer(t, containerName)
			defer cleanupContainer(t, containerName)

			// Change to working directory
			originalDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("Failed to get current directory: %v", err)
			}
			defer os.Chdir(originalDir)

			if err := os.Chdir(tempDir); err != nil {
				t.Fatalf("Failed to change to working directory: %v", err)
			}

			// Run devgo up command
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, devgoBinary, "up")
			output, err := cmd.CombinedOutput()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none. Output: %s", string(output))
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v. Output: %s", err, string(output))
				return
			}

			// Validate results
			if tt.validateResults != nil {
				tt.validateResults(t, tempDir, string(output))
			}
		})
	}
}
