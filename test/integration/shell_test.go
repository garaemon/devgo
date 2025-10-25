package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestShellCommand_BasicExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create temporary test directory
	tempDir, err := os.MkdirTemp("", "devgo-shell-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tempDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer directory: %v", err)
	}

	// Create devcontainer.json
	devcontainerJSON := `{
		"name": "shell-test",
		"image": "ubuntu:22.04",
		"workspaceFolder": "/workspace"
	}`

	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(devcontainerJSON), 0644); err != nil {
		t.Fatalf("Failed to write devcontainer.json: %v", err)
	}

	// Build devgo binary
	devgoBinary := buildDevgoBinary(t)
	defer os.Remove(devgoBinary)

	// Start container with devgo up
	upCmd := exec.Command(devgoBinary, "up", "--workspace-folder", tempDir)
	upCmd.Dir = tempDir
	if output, err := upCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run devgo up: %v\nOutput: %s", err, output)
	}

	// Clean up container after test
	defer func() {
		downCmd := exec.Command(devgoBinary, "down", "--workspace-folder", tempDir)
		downCmd.Dir = tempDir
		if output, err := downCmd.CombinedOutput(); err != nil {
			t.Logf("Warning: Failed to clean up container: %v\nOutput: %s", err, output)
		}
	}()

	// Wait a bit for container to be fully ready
	time.Sleep(2 * time.Second)

	// Test that we can execute a command in the container
	execCmd := exec.Command(devgoBinary, "exec", "--workspace-folder", tempDir, "echo", "test-output")
	execCmd.Dir = tempDir
	output, err := execCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run devgo exec: %v\nOutput: %s", err, output)
	}

	expectedOutput := "test-output\n"
	if string(output) != expectedOutput {
		t.Errorf("Expected output %q, got %q", expectedOutput, string(output))
	}
}

func TestShellCommand_WorkingDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create temporary test directory
	tempDir, err := os.MkdirTemp("", "devgo-shell-pwd-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tempDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer directory: %v", err)
	}

	// Create devcontainer.json with custom workspace folder
	devcontainerJSON := `{
		"name": "shell-pwd-test",
		"image": "ubuntu:22.04",
		"workspaceFolder": "/custom/workspace"
	}`

	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(devcontainerJSON), 0644); err != nil {
		t.Fatalf("Failed to write devcontainer.json: %v", err)
	}

	// Build devgo binary
	devgoBinary := buildDevgoBinary(t)
	defer os.Remove(devgoBinary)

	// Start container
	upCmd := exec.Command(devgoBinary, "up", "--workspace-folder", tempDir)
	upCmd.Dir = tempDir
	if output, err := upCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run devgo up: %v\nOutput: %s", err, output)
	}

	defer func() {
		downCmd := exec.Command(devgoBinary, "down", "--workspace-folder", tempDir)
		downCmd.Dir = tempDir
		if output, err := downCmd.CombinedOutput(); err != nil {
			t.Logf("Warning: Failed to clean up: %v\nOutput: %s", err, output)
		}
	}()

	// Wait for container
	time.Sleep(2 * time.Second)

	// Execute pwd command to verify working directory
	execCmd := exec.Command(devgoBinary, "exec", "--workspace-folder", tempDir, "pwd")
	execCmd.Dir = tempDir
	output, err := execCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run devgo exec pwd: %v\nOutput: %s", err, output)
	}

	expectedPwd := "/custom/workspace\n"
	if string(output) != expectedPwd {
		t.Errorf("Expected working directory %q, got %q", expectedPwd, string(output))
	}

	t.Logf("Successfully verified working directory: %s", string(output))
}
