package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/garaemon/devgo/pkg/devcontainer"
)

func TestDetermineDockerfilePath(t *testing.T) {
	tests := []struct {
		name             string
		devContainer     *devcontainer.DevContainer
		devcontainerPath string
		expected         string
	}{
		{
			name: "dockerfile specified in devcontainer",
			devContainer: &devcontainer.DevContainer{
				Build: &devcontainer.BuildConfig{
					Dockerfile: "custom.Dockerfile",
				},
			},
			devcontainerPath: "/workspace/.devcontainer/devcontainer.json",
			expected:         "/workspace/.devcontainer/custom.Dockerfile",
		},
		{
			name: "absolute dockerfile path",
			devContainer: &devcontainer.DevContainer{
				Build: &devcontainer.BuildConfig{
					Dockerfile: "/absolute/path/Dockerfile",
				},
			},
			devcontainerPath: "/workspace/.devcontainer/devcontainer.json",
			expected:         "/absolute/path/Dockerfile",
		},
		{
			name: "default dockerfile",
			devContainer: &devcontainer.DevContainer{
				Build: &devcontainer.BuildConfig{},
			},
			devcontainerPath: "/workspace/.devcontainer/devcontainer.json",
			expected:         "/workspace/.devcontainer/Dockerfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineDockerfilePath(tt.devContainer, tt.devcontainerPath)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestDetermineBuildContext(t *testing.T) {
	tests := []struct {
		name             string
		devContainer     *devcontainer.DevContainer
		workspaceDir     string
		devcontainerPath string
		expected         string
	}{
		{
			name: "context specified in devcontainer",
			devContainer: &devcontainer.DevContainer{
				Build: &devcontainer.BuildConfig{
					Context: "../",
				},
			},
			workspaceDir:     "/workspace",
			devcontainerPath: "/workspace/.devcontainer/devcontainer.json",
			expected:         "/workspace",
		},
		{
			name: "absolute context path",
			devContainer: &devcontainer.DevContainer{
				Build: &devcontainer.BuildConfig{
					Context: "/absolute/path",
				},
			},
			workspaceDir:     "/workspace",
			devcontainerPath: "/workspace/.devcontainer/devcontainer.json",
			expected:         "/absolute/path",
		},
		{
			name: "default context",
			devContainer: &devcontainer.DevContainer{
				Build: &devcontainer.BuildConfig{},
			},
			workspaceDir:     "/workspace",
			devcontainerPath: "/workspace/.devcontainer/devcontainer.json",
			expected:         "/workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineBuildContext(tt.devContainer, tt.workspaceDir, tt.devcontainerPath)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestDetermineImageTag(t *testing.T) {
	tests := []struct {
		name         string
		devContainer *devcontainer.DevContainer
		workspaceDir string
		imageName    string
		expected     string
	}{
		{
			name:         "image name flag specified",
			devContainer: &devcontainer.DevContainer{},
			workspaceDir: "/workspace/myproject",
			imageName:    "custom-image:v1.0",
			expected:     "custom-image:v1.0",
		},
		{
			name: "devcontainer name specified",
			devContainer: &devcontainer.DevContainer{
				Name: "myapp",
			},
			workspaceDir: "/workspace/myproject",
			imageName:    "",
			expected:     "devgo-myapp:latest",
		},
		{
			name:         "default name from workspace",
			devContainer: &devcontainer.DevContainer{},
			workspaceDir: "/workspace/myproject",
			imageName:    "",
			expected:     "devgo-myproject:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original imageName and restore after test
			originalImageName := imageName
			defer func() { imageName = originalImageName }()

			imageName = tt.imageName
			result := determineImageTag(tt.devContainer, tt.workspaceDir)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetWorkspaceDirectory(t *testing.T) {
	tests := []struct {
		name            string
		workspaceFolder string
		expectError     bool
	}{
		{
			name:            "workspace folder specified",
			workspaceFolder: "/custom/workspace",
			expectError:     false,
		},
		{
			name:            "use current directory",
			workspaceFolder: "",
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original workspaceFolder and restore after test
			originalWorkspaceFolder := workspaceFolder
			defer func() { workspaceFolder = originalWorkspaceFolder }()

			workspaceFolder = tt.workspaceFolder
			result, err := getWorkspaceDirectory()

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.expectError {
				if tt.workspaceFolder != "" && result != tt.workspaceFolder {
					t.Errorf("expected %s, got %s", tt.workspaceFolder, result)
				}
				if tt.workspaceFolder == "" {
					// Should return current working directory
					cwd, _ := os.Getwd()
					if result != cwd {
						t.Errorf("expected current directory %s, got %s", cwd, result)
					}
				}
			}
		})
	}
}

func TestRunBuildCommand(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "devgo-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test devcontainer.json
	devcontainerDir := filepath.Join(tempDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("failed to create devcontainer dir: %v", err)
	}

	devcontainerContent := `{
		"name": "test-container",
		"build": {
			"dockerfile": "Dockerfile",
			"context": ".."
		}
	}`
	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(devcontainerContent), 0644); err != nil {
		t.Fatalf("failed to write devcontainer.json: %v", err)
	}

	tests := []struct {
		name            string
		workspaceFolder string
		configPath      string
		expectError     bool
	}{
		{
			name:            "missing devcontainer config",
			workspaceFolder: "/nonexistent",
			configPath:      "",
			expectError:     true,
		},
		{
			name:            "valid config but no dockerfile",
			workspaceFolder: tempDir,
			configPath:      devcontainerPath,
			expectError:     true, // Will fail because docker command isn't available in test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original values and restore after test
			originalWorkspaceFolder := workspaceFolder
			originalConfigPath := configPath
			defer func() {
				workspaceFolder = originalWorkspaceFolder
				configPath = originalConfigPath
			}()

			workspaceFolder = tt.workspaceFolder
			configPath = tt.configPath

			err := runBuildCommand([]string{})
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
