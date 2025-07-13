package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunReadConfigurationCommand(t *testing.T) {
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
  "workspaceFolder": "/workspace"
}`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		args        []string
		configPath  string
		expectError bool
		expectJSON  bool
	}{
		{
			name:        "valid devcontainer config",
			args:        []string{},
			configPath:  configFile,
			expectError: false,
			expectJSON:  true,
		},
		{
			name:        "invalid config path",
			args:        []string{},
			configPath:  "/nonexistent/path/devcontainer.json",
			expectError: true,
			expectJSON:  false,
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

			err := runReadConfigurationCommand(tt.args)

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

func TestReadConfigurationOutput(t *testing.T) {
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
  "workspaceFolder": "/workspace"
}`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	oldCwd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldCwd)
	
	configPath = ""

	err := runReadConfigurationCommand([]string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
