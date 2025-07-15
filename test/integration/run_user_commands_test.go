package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunUserCommandsIntegration tests the run-user-commands command with actual Docker
func TestRunUserCommandsIntegration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	tests := []struct {
		name                string
		devcontainerContent string
		expectedOutput      []string
		expectError         bool
	}{
		{
			name: "run lifecycle commands with waitFor default",
			devcontainerContent: `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "onCreateCommand": "echo 'onCreate executed'",
  "updateContentCommand": "echo 'updateContent executed'",
  "postCreateCommand": "echo 'postCreate executed'"
}`,
			expectedOutput: []string{
				"Running onCreateCommand:",
				"Running updateContentCommand:",
				"Running postCreateCommand:",
				"Successfully executed user commands up to updateContentCommand",
			},
			expectError: false,
		},
		{
			name: "run lifecycle commands with waitFor postCreateCommand",
			devcontainerContent: `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "onCreateCommand": "echo 'onCreate for postCreate'",
  "updateContentCommand": "echo 'updateContent for postCreate'",
  "postCreateCommand": "echo 'postCreate for postCreate'",
  "postStartCommand": "echo 'postStart should not run'",
  "waitFor": "postCreateCommand"
}`,
			expectedOutput: []string{
				"Running onCreateCommand:",
				"Running updateContentCommand:",
				"Running postCreateCommand:",
				"Successfully executed user commands up to postCreateCommand",
			},
			expectError: false,
		},
		{
			name: "run with postAttachCommand",
			devcontainerContent: `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "postCreateCommand": "echo 'postCreate with attach'",
  "postAttachCommand": "echo 'postAttach executed'"
}`,
			expectedOutput: []string{
				"Running postCreateCommand:",
				"Running postAttachCommand:",
				"Successfully executed user commands up to updateContentCommand",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary workspace for testing
			tempDir := t.TempDir()
			
			// Setup devcontainer
			setupDevcontainerForRunUserCommands(t, tempDir, tt.devcontainerContent)

			// Build devgo binary
			devgoBinary := buildDevgoBinary(t)
			defer os.Remove(devgoBinary)

			// Determine container name (based on directory name)
			containerName := filepath.Base(tempDir)

			// Clean up any existing container with this name
			cleanupContainer(t, containerName)
			defer cleanupContainer(t, containerName)

			// First, create and start the container using up command
			upCmd := exec.Command(devgoBinary, "up")
			upCmd.Dir = tempDir
			upOutput, err := upCmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Failed to run up command: %v. Output: %s", err, string(upOutput))
			}

			// Wait a bit for container to be fully ready
			time.Sleep(2 * time.Second)

			// Verify container is running
			if !isContainerRunning(t, containerName) {
				t.Fatalf("Container %s is not running after up command", containerName)
			}

			// Now run the run-user-commands command
			runCmd := exec.Command(devgoBinary, "run-user-commands")
			runCmd.Dir = tempDir

			output, err := runCmd.CombinedOutput()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but command succeeded. Output: %s", string(output))
				}
				return
			}

			if err != nil {
				t.Errorf("Command failed with error: %v. Output: %s", err, string(output))
				return
			}

			// Log output for debugging
			t.Logf("run-user-commands output: %s", string(output))

			// Verify expected output strings are present
			outputStr := string(output)
			for _, expected := range tt.expectedOutput {
				if !strings.Contains(outputStr, expected) {
					t.Errorf("Expected output to contain '%s', got: %s", expected, outputStr)
				}
			}
		})
	}
}

// TestRunUserCommandsWithoutRunningContainer tests error when no container is running
func TestRunUserCommandsWithoutRunningContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	// Create a temporary workspace for testing
	tempDir := t.TempDir()
	
	// Setup devcontainer
	devcontainerContent := `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "postCreateCommand": "echo 'should not run'"
}`
	setupDevcontainerForRunUserCommands(t, tempDir, devcontainerContent)

	// Build devgo binary
	devgoBinary := buildDevgoBinary(t)
	defer os.Remove(devgoBinary)

	// Try to run run-user-commands without starting container first
	runCmd := exec.Command(devgoBinary, "run-user-commands")
	runCmd.Dir = tempDir

	output, err := runCmd.CombinedOutput()

	// Should fail because no container is running
	if err == nil {
		t.Errorf("Expected error but command succeeded. Output: %s", string(output))
		return
	}

	// Verify error message
	expectedError := "no running devgo containers found"
	if !strings.Contains(string(output), expectedError) {
		t.Errorf("Expected error containing '%s', got: %s", expectedError, string(output))
	}

	t.Logf("Expected error occurred: %s", string(output))
}

// TestRunUserCommandsWithMultipleContainers tests workspace matching with multiple containers
func TestRunUserCommandsWithMultipleContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	// Create two temporary workspaces
	tempDir1 := t.TempDir()
	tempDir2 := t.TempDir()
	
	// Setup first devcontainer
	devcontainerContent1 := `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "postCreateCommand": "echo 'workspace1 command'"
}`
	setupDevcontainerForRunUserCommands(t, tempDir1, devcontainerContent1)

	// Setup second devcontainer
	devcontainerContent2 := `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "postCreateCommand": "echo 'workspace2 command'"
}`
	setupDevcontainerForRunUserCommands(t, tempDir2, devcontainerContent2)

	// Build devgo binary
	devgoBinary := buildDevgoBinary(t)
	defer os.Remove(devgoBinary)

	// Get container names
	containerName1 := filepath.Base(tempDir1)
	containerName2 := filepath.Base(tempDir2)

	// Clean up any existing containers
	cleanupContainer(t, containerName1)
	cleanupContainer(t, containerName2)
	defer cleanupContainer(t, containerName1)
	defer cleanupContainer(t, containerName2)

	// Start both containers
	upCmd1 := exec.Command(devgoBinary, "up")
	upCmd1.Dir = tempDir1
	if output, err := upCmd1.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run up command for workspace1: %v. Output: %s", err, string(output))
	}

	upCmd2 := exec.Command(devgoBinary, "up")
	upCmd2.Dir = tempDir2
	if output, err := upCmd2.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run up command for workspace2: %v. Output: %s", err, string(output))
	}

	// Wait for containers to be ready
	time.Sleep(2 * time.Second)

	// Verify both containers are running
	if !isContainerRunning(t, containerName1) {
		t.Fatalf("Container %s is not running", containerName1)
	}
	if !isContainerRunning(t, containerName2) {
		t.Fatalf("Container %s is not running", containerName2)
	}

	// Run run-user-commands from workspace1 - should target workspace1's container
	runCmd := exec.Command(devgoBinary, "run-user-commands")
	runCmd.Dir = tempDir1

	output, err := runCmd.CombinedOutput()
	if err != nil {
		t.Errorf("Command failed with error: %v. Output: %s", err, string(output))
		return
	}

	// Should contain workspace1 specific output
	if !strings.Contains(string(output), "workspace1 command") {
		t.Errorf("Expected output to target workspace1, got: %s", string(output))
	}

	t.Logf("run-user-commands correctly targeted workspace1: %s", string(output))
}

// Helper functions - using existing helpers from up_test.go

func setupDevcontainerForRunUserCommands(t *testing.T, tempDir, content string) {
	t.Helper()

	devcontainerDir := filepath.Join(tempDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer directory: %v", err)
	}

	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create devcontainer.json: %v", err)
	}
}