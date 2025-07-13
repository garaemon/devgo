package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestDockerComposeIntegration tests the up command with Docker Compose
func TestDockerComposeIntegration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	// Check if Docker Compose is available
	if !isDockerComposeAvailable() {
		t.Skip("Docker Compose is not available, skipping integration test")
	}

	// Create a temporary workspace for testing
	tempDir := t.TempDir()

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tempDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer directory: %v", err)
	}

	// Create docker-compose.yml
	dockerComposeContent := `version: '3.8'
services:
  app:
    image: alpine:3.19
    command: sleep infinity
    working_dir: /workspace
    volumes:
      - ..:/workspace
    environment:
      - TEST_ENV=docker_compose_test
  
  redis:
    image: redis:7-alpine
    command: redis-server --appendonly yes
`

	dockerComposePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(dockerComposePath, []byte(dockerComposeContent), 0644); err != nil {
		t.Fatalf("Failed to create docker-compose.yml: %v", err)
	}

	// Test cases
	tests := []struct {
		name                string
		devcontainerContent string
		expectError         bool
		cleanup             func()
		validateContainer   func(t *testing.T, tempDir string)
	}{
		{
			name: "single service docker compose",
			devcontainerContent: `{
  "name": "Docker Compose Dev Environment",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "workspaceFolder": "/workspace"
}`,
			expectError: false,
			cleanup: func() {
				cleanupDockerCompose(t, tempDir)
			},
			validateContainer: func(t *testing.T, tempDir string) {
				projectName := filepath.Base(tempDir)
				containerName := projectName + "-app-1"
				if !isContainerRunning(t, containerName) {
					t.Errorf("Expected container '%s' to be running", containerName)
				}
			},
		},
		{
			name: "multiple services docker compose",
			devcontainerContent: `{
  "name": "Multi-Service Docker Compose",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "runServices": ["app", "redis"],
  "workspaceFolder": "/workspace"
}`,
			expectError: false,
			cleanup: func() {
				cleanupDockerCompose(t, tempDir)
			},
			validateContainer: func(t *testing.T, tempDir string) {
				projectName := filepath.Base(tempDir)
				appContainer := projectName + "-app-1"
				redisContainer := projectName + "-redis-1"
				
				if !isContainerRunning(t, appContainer) {
					t.Errorf("Expected app container '%s' to be running", appContainer)
				}
				if !isContainerRunning(t, redisContainer) {
					t.Errorf("Expected redis container '%s' to be running", redisContainer)
				}
			},
		},
		{
			name: "docker compose without service",
			devcontainerContent: `{
  "name": "Invalid Docker Compose",
  "dockerComposeFile": "docker-compose.yml",
  "workspaceFolder": "/workspace"
}`,
			expectError: true,
			cleanup: func() {
				cleanupDockerCompose(t, tempDir)
			},
			validateContainer: nil,
		},
	}

	// Build devgo binary
	devgoBinary := buildDevgoBinary(t)
	defer os.Remove(devgoBinary)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pre-cleanup any existing containers
			cleanupDockerCompose(t, tempDir)

			// Ensure cleanup happens
			if tt.cleanup != nil {
				defer tt.cleanup()
			}

			// Create devcontainer.json
			devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
			if err := os.WriteFile(devcontainerPath, []byte(tt.devcontainerContent), 0644); err != nil {
				t.Fatalf("Failed to create devcontainer.json: %v", err)
			}

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
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
				t.Errorf("Unexpected error: %v. Output: %s", err, string(output))
				return
			}

			// Wait a moment for containers to be fully started
			time.Sleep(5 * time.Second)

			// Validate containers if specified
			if tt.validateContainer != nil {
				tt.validateContainer(t, tempDir)
			}

			t.Logf("Command output: %s", string(output))
		})
	}
}

// TestDockerComposeMultipleFiles tests docker compose with multiple files
func TestDockerComposeMultipleFiles(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	// Check if Docker Compose is available
	if !isDockerComposeAvailable() {
		t.Skip("Docker Compose is not available, skipping integration test")
	}

	// Create a temporary workspace for testing
	tempDir := t.TempDir()

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tempDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer directory: %v", err)
	}

	// Create base docker-compose.yml
	dockerComposeContent := `version: '3.8'
services:
  app:
    image: alpine:3.19
    command: sleep infinity
    working_dir: /workspace
    volumes:
      - ..:/workspace
`

	dockerComposePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(dockerComposePath, []byte(dockerComposeContent), 0644); err != nil {
		t.Fatalf("Failed to create docker-compose.yml: %v", err)
	}

	// Create override docker-compose.dev.yml
	dockerComposeDevContent := `version: '3.8'
services:
  app:
    environment:
      - TEST_ENV=multi_file_test
      - NODE_ENV=development
  
  db:
    image: postgres:15-alpine
    environment:
      - POSTGRES_PASSWORD=testpass
      - POSTGRES_DB=testdb
`

	dockerComposeDevPath := filepath.Join(tempDir, "docker-compose.dev.yml")
	if err := os.WriteFile(dockerComposeDevPath, []byte(dockerComposeDevContent), 0644); err != nil {
		t.Fatalf("Failed to create docker-compose.dev.yml: %v", err)
	}

	// Create devcontainer.json with multiple compose files
	devcontainerContent := `{
  "name": "Multi-File Docker Compose",
  "dockerComposeFile": ["docker-compose.yml", "docker-compose.dev.yml"],
  "service": "app",
  "runServices": ["app", "db"],
  "workspaceFolder": "/workspace"
}`

	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(devcontainerContent), 0644); err != nil {
		t.Fatalf("Failed to create devcontainer.json: %v", err)
	}

	// Build devgo binary
	devgoBinary := buildDevgoBinary(t)
	defer os.Remove(devgoBinary)

	// Pre-cleanup any existing containers
	cleanupDockerCompose(t, tempDir)
	defer cleanupDockerCompose(t, tempDir)

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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, devgoBinary, "up")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Errorf("Unexpected error: %v. Output: %s", err, string(output))
		return
	}

	// Wait a moment for containers to be fully started
	time.Sleep(5 * time.Second)

	// Validate containers
	projectName := filepath.Base(tempDir)
	appContainer := projectName + "-app-1"
	dbContainer := projectName + "-db-1"

	if !isContainerRunning(t, appContainer) {
		t.Errorf("Expected app container '%s' to be running", appContainer)
	}
	if !isContainerRunning(t, dbContainer) {
		t.Errorf("Expected db container '%s' to be running", dbContainer)
	}

	t.Logf("Command output: %s", string(output))
}

// isDockerComposeAvailable checks if Docker Compose is available
func isDockerComposeAvailable() bool {
	cmd := exec.Command("docker", "compose", "version")
	return cmd.Run() == nil
}

// cleanupDockerCompose stops and removes all containers created by docker compose
func cleanupDockerCompose(t *testing.T, projectDir string) {
	t.Helper()

	// Stop and remove containers using docker compose down
	cmd := exec.Command("docker", "compose", "down", "--remove-orphans")
	cmd.Dir = projectDir
	if err := cmd.Run(); err != nil {
		t.Logf("Warning: failed to run docker compose down: %v", err)
	}

	// Also try to clean up individual containers by project name
	projectName := filepath.Base(projectDir)
	containers := []string{
		projectName + "-app-1",
		projectName + "-redis-1",
		projectName + "-db-1",
	}

	for _, containerName := range containers {
		cleanupContainer(t, containerName)
	}
}