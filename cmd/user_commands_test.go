package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunUserCommandsCommand(t *testing.T) {
	originalConfigPath := configPath
	defer func() { configPath = originalConfigPath }()

	tempDir := t.TempDir()
	devcontainerDir := filepath.Join(tempDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatal(err)
	}

	configFile := filepath.Join(devcontainerDir, "devcontainer.json")
	configContent := `{
  "name": "test-container",
  "image": "node:18",
  "workspaceFolder": "/workspace",
  "postCreateCommand": "echo 'post create test'",
  "postStartCommand": "echo 'post start test'"
}`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		args        []string
		configPath  string
		expectError bool
	}{
		{
			name:        "valid devcontainer config but no running container",
			args:        []string{},
			configPath:  configFile,
			expectError: true, // Expected to fail as no container is running
		},
		{
			name:        "invalid config path",
			args:        []string{},
			configPath:  "/nonexistent/path/devcontainer.json",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath = tt.configPath

			oldCwd, _ := os.Getwd()
			if tt.configPath == configFile {
				os.Chdir(tempDir)
				configPath = ""
			}
			defer os.Chdir(oldCwd)

			err := runUserCommandsCommand(tt.args)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestFindRunningDevContainer(t *testing.T) {
	// This test requires Docker to be running and would need actual containers
	// In a real test environment, you would mock the Docker client
	t.Skip("Skipping test that requires running Docker containers")
}

func TestRunLifecycleCommands(t *testing.T) {
	// This test requires Docker to be running and would need actual containers
	// In a real test environment, you would mock the Docker client and command execution
	t.Skip("Skipping test that requires running Docker containers")
}