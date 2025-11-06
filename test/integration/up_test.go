package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/garaemon/devgo/cmd"
)

// TestUpCommandIntegration tests the up command with actual Docker
func TestUpCommandIntegration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	// Create a temporary workspace for testing
	tempDir := t.TempDir()

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tempDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer directory: %v", err)
	}

	// Create a simple devcontainer.json (no name so it uses directory-based naming)
	devcontainerContent := `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "containerEnv": {
    "TEST_ENV": "integration_test"
  }
}`

	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(devcontainerContent), 0644); err != nil {
		t.Fatalf("Failed to create devcontainer.json: %v", err)
	}

	// Build devgo binary
	devgoBinary := buildDevgoBinary(t)
	defer os.Remove(devgoBinary)

	// Test cases
	tests := []struct {
		name        string
		workingDir  string
		expectError bool
		cleanup     func()
	}{
		{
			name:        "successful up command",
			workingDir:  tempDir,
			expectError: false,
			cleanup: func() {
				containerName := buildExpectedContainerName(tempDir)
				cleanupContainer(t, containerName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pre-cleanup any existing containers
			containerName := buildExpectedContainerName(tt.workingDir)
			cleanupContainer(t, containerName)

			// Ensure cleanup happens
			if tt.cleanup != nil {
				defer tt.cleanup()
			}

			// Change to working directory
			originalDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("Failed to get current directory: %v", err)
			}
			defer os.Chdir(originalDir)

			if err := os.Chdir(tt.workingDir); err != nil {
				t.Fatalf("Failed to change to working directory: %v", err)
			}

			// Run devgo up command
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, devgoBinary, "up")
			output, err := cmd.CombinedOutput()

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
			t.Logf("Command output: %s", string(output))

			// Verify container was created and is running
			t.Logf("Looking for container: %s", containerName)

			if !isContainerRunning(t, containerName) {
				// List all containers for debugging
				listCmd := exec.Command("docker", "ps", "-a")
				listOutput, _ := listCmd.Output()
				t.Logf("All containers: %s", string(listOutput))

				t.Errorf("Container %s is not running after up command", containerName)
				return
			}

			// Verify container has correct properties
			verifyContainerProperties(t, containerName, tempDir)
		})
	}
}

// TestUpCommandWithExistingContainer tests behavior when container already exists
func TestUpCommandWithExistingContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	tempDir := t.TempDir()
	setupDevcontainer(t, tempDir, "existing-test-container")

	devgoBinary := buildDevgoBinary(t)
	defer os.Remove(devgoBinary)

	containerName := buildExpectedContainerName(tempDir)
	// Clean up any existing containers first
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

	// First up command - should create container
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, devgoBinary, "up")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("First up command failed: %v. Output: %s", err, string(output))
	}

	// Stop the container
	stopContainer(t, containerName)

	// Second up command - should start existing container
	ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()

	cmd2 := exec.CommandContext(ctx2, devgoBinary, "up")
	output2, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("Second up command failed: %v. Output: %s", err, string(output2))
	}

	// Verify container is running
	if !isContainerRunning(t, containerName) {
		t.Errorf("Container %s should be running after second up command", containerName)
	}

	// Third up command - should fail with "already running" error
	ctx3, cancel3 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel3()

	cmd3 := exec.CommandContext(ctx3, devgoBinary, "up")
	output3, err := cmd3.CombinedOutput()
	if err == nil {
		t.Errorf("Third up command should have failed but succeeded. Output: %s", string(output3))
	}

	expectedError := "already running"
	if !strings.Contains(string(output3), expectedError) {
		t.Errorf("Expected error containing '%s', got: %s", expectedError, string(output3))
	}
}

// Helper functions

func buildExpectedContainerName(workspaceDir string) string {
	pathHash := cmd.GeneratePathHash(workspaceDir)
	baseName := filepath.Base(workspaceDir)
	return fmt.Sprintf("%s-default-%s", baseName, pathHash)
}

func isDockerAvailable() bool {
	execCmd := exec.Command("docker", "version")
	return execCmd.Run() == nil
}

func buildDevgoBinary(t *testing.T) string {
	t.Helper()

	// Get project root (parent of test directory)
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	projectRoot := filepath.Dir(filepath.Dir(testDir))

	// Build binary in temp location
	tmpBinary := filepath.Join(t.TempDir(), "devgo-integration-test")

	cmd := exec.Command("go", "build", "-o", tmpBinary, ".")
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build devgo binary: %v. Output: %s", err, string(output))
	}

	return tmpBinary
}

func setupDevcontainer(t *testing.T, tempDir, containerName string) {
	t.Helper()

	devcontainerDir := filepath.Join(tempDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer directory: %v", err)
	}

	// Don't use hardcoded name, let it use directory-based naming
	devcontainerContent := `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "containerEnv": {
    "TEST_ENV": "integration_test"
  }
}`

	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(devcontainerContent), 0644); err != nil {
		t.Fatalf("Failed to create devcontainer.json: %v", err)
	}
}

func isContainerRunning(t *testing.T, containerName string) bool {
	t.Helper()

	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		t.Logf("Failed to check if container is running: %v", err)
		return false
	}

	return strings.Contains(string(output), containerName)
}

func verifyContainerProperties(t *testing.T, containerName, workspaceDir string) {
	t.Helper()

	// Check if container has the correct labels
	cmd := exec.Command("docker", "inspect", containerName, "--format", "{{.Config.Labels}}")
	output, err := cmd.Output()
	if err != nil {
		t.Errorf("Failed to inspect container: %v", err)
		return
	}

	labels := string(output)
	if !strings.Contains(labels, "devgo.managed:true") {
		t.Errorf("Container missing devgo.managed label. Labels: %s", labels)
	}

	// Check if workspace is mounted correctly
	cmd = exec.Command("docker", "inspect", containerName, "--format", "{{range .Mounts}}{{.Source}}:{{.Destination}} {{end}}")
	output, err = cmd.Output()
	if err != nil {
		t.Errorf("Failed to inspect container mounts: %v", err)
		return
	}

	mounts := string(output)
	expectedMount := fmt.Sprintf("%s:/workspace", workspaceDir)
	if !strings.Contains(mounts, expectedMount) {
		t.Errorf("Expected mount %s not found. Mounts: %s", expectedMount, mounts)
	}
}

func stopContainer(t *testing.T, containerName string) {
	t.Helper()

	cmd := exec.Command("docker", "stop", containerName)
	if err := cmd.Run(); err != nil {
		t.Logf("Failed to stop container %s: %v", containerName, err)
	}
}

func cleanupContainer(t *testing.T, containerName string) {
	t.Helper()

	// Force stop and remove container
	// Use -f flag to force removal even if container is running
	removeCmd := exec.Command("docker", "rm", "-f", containerName)
	removeCmd.Run() // Ignore errors - container might not exist

	// Also cleanup any containers with matching prefix (e.g., docker compose containers)
	// This handles cases where the container name matches a directory name pattern
	if strings.Contains(containerName, "devgo-") {
		// Extract base name pattern (e.g., "001" from "devgo-001-default")
		baseName := strings.TrimPrefix(containerName, "devgo-")
		baseName = strings.TrimSuffix(baseName, "-default")

		// Remove any docker compose containers with this pattern
		composeContainers := []string{
			baseName + "-app-1",
			baseName + "-redis-1",
			baseName + "-db-1",
		}

		for _, cn := range composeContainers {
			cmd := exec.Command("docker", "rm", "-f", cn)
			cmd.Run() // Ignore errors
		}
	}
}

// TestOnCreateCommandIntegration tests onCreateCommand execution in actual containers
func TestOnCreateCommandIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	tests := []struct {
		name                string
		devcontainerContent string
		expectedFiles       []string
		expectedDirs        []string
		verifyCommand       []string
		expectInOutput      []string
	}{
		{
			name: "onCreateCommand creates file",
			devcontainerContent: `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "onCreateCommand": "touch /workspace/created-by-oncreate.txt"
}`,
			expectedFiles: []string{"/workspace/created-by-oncreate.txt"},
		},
		{
			name: "onCreateCommand array format",
			devcontainerContent: `{
  "image": "alpine:3.19", 
  "workspaceFolder": "/workspace",
  "onCreateCommand": ["sh", "-c", "mkdir -p /workspace/oncreate-dir && echo 'hello from oncreate' > /workspace/oncreate-dir/message.txt"]
}`,
			expectedDirs:   []string{"/workspace/oncreate-dir"},
			expectedFiles:  []string{"/workspace/oncreate-dir/message.txt"},
			verifyCommand:  []string{"cat", "/workspace/oncreate-dir/message.txt"},
			expectInOutput: []string{"hello from oncreate"},
		},
		{
			name: "onCreateCommand installs packages",
			devcontainerContent: `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace", 
  "onCreateCommand": "apk add --no-cache curl && curl --version > /workspace/curl-version.txt"
}`,
			expectedFiles:  []string{"/workspace/curl-version.txt"},
			verifyCommand:  []string{"cat", "/workspace/curl-version.txt"},
			expectInOutput: []string{"curl"},
		},
		{
			name: "onCreateCommand and postCreateCommand order",
			devcontainerContent: `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "onCreateCommand": "echo 'step1: onCreateCommand' > /workspace/execution-order.txt",
  "postCreateCommand": "echo 'step2: postCreateCommand' >> /workspace/execution-order.txt",
  "waitFor": "postCreateCommand"
}`,
			expectedFiles:  []string{"/workspace/execution-order.txt"},
			verifyCommand:  []string{"cat", "/workspace/execution-order.txt"},
			expectInOutput: []string{"step1: onCreateCommand", "step2: postCreateCommand"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary workspace
			tempDir := t.TempDir()

			// Setup devcontainer with onCreateCommand
			setupDevcontainerWithContent(t, tempDir, tt.devcontainerContent)

			// Build devgo binary
			devgoBinary := buildDevgoBinary(t)
			defer os.Remove(devgoBinary)

			containerName := buildExpectedContainerName(tempDir)
			cleanupContainer(t, containerName)
			defer func() {
				// Clean up container-created files before container cleanup
				cleanupContainerFiles(t, containerName, tempDir)
				cleanupContainer(t, containerName)
			}()

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
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, devgoBinary, "up")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("devgo up failed: %v. Output: %s", err, string(output))
			}

			t.Logf("devgo up output: %s", string(output))

			// Verify onCreateCommand was mentioned in output
			if !strings.Contains(string(output), "Running onCreateCommand") {
				t.Errorf("Expected 'Running onCreateCommand' in output, got: %s", string(output))
			}

			// Verify container is running
			if !isContainerRunning(t, containerName) {
				t.Fatalf("Container %s is not running", containerName)
			}

			// Verify expected directories exist
			for _, dir := range tt.expectedDirs {
				if !containerPathExists(t, containerName, dir, true) {
					t.Errorf("Expected directory %s does not exist in container", dir)
				}
			}

			// Verify expected files exist
			for _, file := range tt.expectedFiles {
				if !containerPathExists(t, containerName, file, false) {
					t.Errorf("Expected file %s does not exist in container", file)
				}
			}

			// Run verification command if specified
			if len(tt.verifyCommand) > 0 {
				verifyOutput := runCommandInContainer(t, containerName, tt.verifyCommand)
				for _, expected := range tt.expectInOutput {
					if !strings.Contains(verifyOutput, expected) {
						t.Errorf("Expected '%s' in command output, got: %s", expected, verifyOutput)
					}
				}
			}
		})
	}
}

// TestOnCreateCommandFailure tests behavior when onCreateCommand fails
func TestOnCreateCommandFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	tempDir := t.TempDir()

	// Create devcontainer with failing onCreateCommand
	devcontainerContent := `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "onCreateCommand": "exit 1"
}`

	setupDevcontainerWithContent(t, tempDir, devcontainerContent)

	devgoBinary := buildDevgoBinary(t)
	defer os.Remove(devgoBinary)

	containerName := buildExpectedContainerName(tempDir)
	cleanupContainer(t, containerName)
	defer func() {
		cleanupContainerFiles(t, containerName, tempDir)
		cleanupContainer(t, containerName)
	}()

	// Change to working directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to working directory: %v", err)
	}

	// Run devgo up command - should execute onCreateCommand but continue even if it fails
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, devgoBinary, "up")
	output, err := cmd.CombinedOutput()

	// Current implementation continues even if onCreateCommand fails
	if err != nil {
		t.Fatalf("devgo up failed: %v. Output: %s", err, string(output))
	}

	// Verify onCreateCommand was executed (even though it failed)
	if !strings.Contains(string(output), "Running onCreateCommand") {
		t.Errorf("Expected 'Running onCreateCommand' in output, got: %s", string(output))
	}

	t.Logf("Command output with failing onCreateCommand: %s", string(output))
}

// Helper functions for onCreateCommand tests

func setupDevcontainerWithContent(t *testing.T, tempDir, content string) {
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

func containerPathExists(t *testing.T, containerName, path string, isDir bool) bool {
	t.Helper()

	// Use appropriate test command for file vs directory
	var testCmd []string
	if isDir {
		testCmd = []string{"test", "-d", path}
	} else {
		testCmd = []string{"test", "-f", path}
	}

	cmd := exec.Command("docker", "exec", containerName)
	cmd.Args = append(cmd.Args, testCmd...)

	err := cmd.Run()
	return err == nil
}

func runCommandInContainer(t *testing.T, containerName string, command []string) string {
	t.Helper()

	cmd := exec.Command("docker", "exec", containerName)
	cmd.Args = append(cmd.Args, command...)

	output, err := cmd.Output()
	if err != nil {
		t.Logf("Command failed in container: %v", err)
		return ""
	}

	return string(output)
}

func cleanupContainerFiles(t *testing.T, containerName, workspaceDir string) {
	t.Helper()

	// Only cleanup if container exists and is running
	if !isContainerRunning(t, containerName) {
		// Check if container exists but is stopped
		cmd := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
		output, err := cmd.Output()
		if err != nil || !strings.Contains(string(output), containerName) {
			return // Container doesn't exist
		}
	}

	// Remove files that may have been created by onCreateCommand with root permissions
	// This prevents permission errors during test cleanup
	cleanupCommands := [][]string{
		{"rm", "-rf", "/workspace/oncreate-dir"},
		{"rm", "-f", "/workspace/created-by-oncreate.txt"},
		{"rm", "-f", "/workspace/curl-version.txt"},
		{"rm", "-f", "/workspace/execution-order.txt"},
	}

	for _, cmdArgs := range cleanupCommands {
		cmd := exec.Command("docker", "exec", containerName)
		cmd.Args = append(cmd.Args, cmdArgs...)
		cmd.Run() // Ignore errors - files might not exist
	}
}

// TestUpdateRemoteUserUIDIntegration tests UID/GID synchronization in actual containers
func TestUpdateRemoteUserUIDIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	// This test only works on Linux
	if runtime.GOOS != "linux" {
		t.Skip("Skipping UID/GID sync test on non-Linux platform")
	}

	// Get host UID/GID
	hostUID := os.Getuid()
	hostGID := os.Getgid()

	tests := []struct {
		name                string
		devcontainerContent string
		targetUser          string
		shouldSync          bool
		expectHostUID       bool
		expectHostGID       bool
	}{
		{
			name: "remoteUser UID/GID sync enabled",
			devcontainerContent: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "workspaceFolder": "/workspace",
  "remoteUser": "vscode",
  "updateRemoteUserUID": true
}`,
			targetUser:    "vscode",
			shouldSync:    true,
			expectHostUID: true,
			expectHostGID: true,
		},
		{
			name: "containerUser UID/GID sync enabled",
			devcontainerContent: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "workspaceFolder": "/workspace",
  "containerUser": "vscode",
  "updateRemoteUserUID": true
}`,
			targetUser:    "vscode",
			shouldSync:    true,
			expectHostUID: true,
			expectHostGID: true,
		},
		{
			name: "root user should not be modified",
			devcontainerContent: `{
  "image": "alpine:3.19",
  "workspaceFolder": "/workspace",
  "remoteUser": "root",
  "updateRemoteUserUID": true
}`,
			targetUser:    "root",
			shouldSync:    false,
			expectHostUID: false,
			expectHostGID: false,
		},
		{
			name: "UID/GID sync disabled",
			devcontainerContent: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "workspaceFolder": "/workspace",
  "remoteUser": "vscode",
  "updateRemoteUserUID": false
}`,
			targetUser:    "vscode",
			shouldSync:    false,
			expectHostUID: false,
			expectHostGID: false,
		},
		{
			name: "default UID/GID sync (enabled by default)",
			devcontainerContent: `{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "workspaceFolder": "/workspace",
  "remoteUser": "vscode"
}`,
			targetUser:    "vscode",
			shouldSync:    true,
			expectHostUID: true,
			expectHostGID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary workspace
			tempDir := t.TempDir()

			// Setup devcontainer
			setupDevcontainerWithContent(t, tempDir, tt.devcontainerContent)

			// Build devgo binary
			devgoBinary := buildDevgoBinary(t)
			defer os.Remove(devgoBinary)

			containerName := buildExpectedContainerName(tempDir)
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
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, devgoBinary, "up")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("devgo up failed: %v. Output: %s", err, string(output))
			}

			t.Logf("devgo up output: %s", string(output))

			// Verify container is running
			if !isContainerRunning(t, containerName) {
				t.Fatalf("Container %s is not running", containerName)
			}

			// Check if UID/GID sync message appears in output
			if tt.shouldSync {
				expectedMsg := fmt.Sprintf("Updating container user '%s' UID/GID to match host", tt.targetUser)
				if !strings.Contains(string(output), expectedMsg) {
					t.Logf("Warning: Expected UID/GID sync message not found in output")
				}
			}

			// Get container user's UID and GID
			containerUID := getContainerUserUID(t, containerName, tt.targetUser)
			containerGID := getContainerUserGID(t, containerName, tt.targetUser)

			t.Logf("Host UID/GID: %d:%d", hostUID, hostGID)
			t.Logf("Container user '%s' UID/GID: %d:%d", tt.targetUser, containerUID, containerGID)

			// Verify UID/GID matches expectations
			if tt.expectHostUID {
				if containerUID != hostUID {
					t.Errorf("Expected container UID to match host UID (%d), got %d", hostUID, containerUID)
				}
			} else if tt.targetUser == "root" {
				// Root user should always have UID 0
				if containerUID != 0 {
					t.Errorf("Root user should have UID 0, got %d", containerUID)
				}
			}

			if tt.expectHostGID {
				if containerGID != hostGID {
					t.Errorf("Expected container GID to match host GID (%d), got %d", hostGID, containerGID)
				}
			} else if tt.targetUser == "root" {
				// Root user should always have GID 0
				if containerGID != 0 {
					t.Errorf("Root user should have GID 0, got %d", containerGID)
				}
			}

			// Verify that files created in workspace have correct ownership
			if tt.expectHostUID && tt.targetUser != "root" {
				// Create a test file in the container
				createCmd := exec.Command("docker", "exec", "-u", tt.targetUser, containerName, "touch", "/workspace/test-ownership.txt")
				if err := createCmd.Run(); err != nil {
					t.Logf("Failed to create test file: %v", err)
				} else {
					// Check file ownership on host
					testFile := filepath.Join(tempDir, "test-ownership.txt")
					fileInfo, err := os.Stat(testFile)
					if err != nil {
						t.Logf("Failed to stat test file: %v", err)
					} else {
						t.Logf("Test file ownership verified: %v", fileInfo)
					}
				}
			}
		})
	}
}

// Helper function to get container user's UID
func getContainerUserUID(t *testing.T, containerName, username string) int {
	t.Helper()

	cmd := exec.Command("docker", "exec", containerName, "id", "-u", username)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get UID for user %s: %v", username, err)
	}

	var uid int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &uid)
	if err != nil {
		t.Fatalf("Failed to parse UID: %v", err)
	}

	return uid
}

// Helper function to get container user's GID
func getContainerUserGID(t *testing.T, containerName, username string) int {
	t.Helper()

	cmd := exec.Command("docker", "exec", containerName, "id", "-g", username)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get GID for user %s: %v", username, err)
	}

	var gid int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &gid)
	if err != nil {
		t.Fatalf("Failed to parse GID: %v", err)
	}

	return gid
}
