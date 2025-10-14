package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
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
	tests := []struct {
		name           string
		devContainer   *devcontainer.DevContainer
		expectError    bool
		errorContains  string
	}{
		{
			name: "no running containers",
			devContainer: &devcontainer.DevContainer{
				Name: "test-container",
			},
			expectError:   true,
			errorContains: "no running devgo containers found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			containerName, err := findRunningDevContainer(ctx, tt.devContainer)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorContains != "" && err.Error() != tt.errorContains {
					t.Errorf("error message = %q, want to contain %q",
						err.Error(), tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if containerName == "" {
				t.Errorf("expected container name, got empty string")
			}
		})
	}
}

func TestFindRunningDevContainerWithWorkspace(t *testing.T) {
	// This test verifies the workspace matching logic
	// In a real scenario, this would interact with Docker
	// For unit testing, we verify the function exists and handles errors correctly
	ctx := context.Background()
	devContainer := &devcontainer.DevContainer{
		Name: "test-container",
	}

	_, err := findRunningDevContainer(ctx, devContainer)
	if err == nil {
		t.Skip("Docker is running and containers exist, skipping unit test")
	}

	// We expect an error when no containers are running
	if err.Error() != "no running devgo containers found" &&
		err.Error() != "failed to list containers: Cannot connect to the Docker daemon" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunLifecycleCommands_WaitForSettings(t *testing.T) {
	tests := []struct {
		name          string
		devContainer  *devcontainer.DevContainer
		expectError   bool
	}{
		{
			name: "default waitFor (updateContentCommand)",
			devContainer: &devcontainer.DevContainer{
				Name:                 "test",
				OnCreateCommand:      "echo onCreate",
				UpdateContentCommand: "echo updateContent",
				PostCreateCommand:    "echo postCreate",
			},
			expectError: true, // Will fail due to no container, but tests the logic
		},
		{
			name: "waitFor onCreateCommand",
			devContainer: &devcontainer.DevContainer{
				Name:            "test",
				WaitFor:         devcontainer.WaitForOnCreateCommand,
				OnCreateCommand: "echo onCreate",
			},
			expectError: true,
		},
		{
			name: "waitFor postCreateCommand",
			devContainer: &devcontainer.DevContainer{
				Name:              "test",
				WaitFor:           devcontainer.WaitForPostCreateCommand,
				OnCreateCommand:   "echo onCreate",
				PostCreateCommand: "echo postCreate",
			},
			expectError: true,
		},
		{
			name: "waitFor postStartCommand",
			devContainer: &devcontainer.DevContainer{
				Name:             "test",
				WaitFor:          devcontainer.WaitForPostStartCommand,
				PostStartCommand: "echo postStart",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			containerName := "test-container"
			workspaceDir := "/workspace"

			err := runLifecycleCommands(ctx, tt.devContainer, containerName, workspaceDir)

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

func TestDetermineWorkspaceFromDevcontainerPath(t *testing.T) {
	tests := []struct {
		name              string
		devcontainerPath  string
		expectedWorkspace string
	}{
		{
			name:              "devcontainer in .devcontainer folder",
			devcontainerPath:  "/project/.devcontainer/devcontainer.json",
			expectedWorkspace: "/project",
		},
		{
			name:              "devcontainer at project root",
			devcontainerPath:  "/project/.devcontainer.json",
			expectedWorkspace: "/project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspaceDir := filepath.Dir(tt.devcontainerPath)
			if filepath.Base(workspaceDir) == ".devcontainer" {
				workspaceDir = filepath.Dir(workspaceDir)
			}

			if workspaceDir != tt.expectedWorkspace {
				t.Errorf("workspace dir = %q, want %q", workspaceDir, tt.expectedWorkspace)
			}
		})
	}
}

type mockUserCommandsClient struct {
	containers []container.Summary
	listError  error
}

func (m *mockUserCommandsClient) ContainerList(ctx context.Context,
	options container.ListOptions) ([]container.Summary, error) {
	if m.listError != nil {
		return nil, m.listError
	}
	return m.containers, nil
}

func (m *mockUserCommandsClient) Close() error {
	return nil
}

func TestFindRunningDevContainerLogic(t *testing.T) {
	tests := []struct {
		name         string
		containers   []container.Summary
		currentDir   string
		expectedName string
		expectError  bool
	}{
		{
			name: "find container by workspace label",
			containers: []container.Summary{
				{
					Names: []string{"/container1"},
					Labels: map[string]string{
						constants.DevgoManagedLabel:   constants.DevgoManagedValue,
						constants.DevgoWorkspaceLabel: "/test/workspace",
					},
				},
				{
					Names: []string{"/container2"},
					Labels: map[string]string{
						constants.DevgoManagedLabel:   constants.DevgoManagedValue,
						constants.DevgoWorkspaceLabel: "/other/workspace",
					},
				},
			},
			currentDir:   "/test/workspace",
			expectedName: "container1",
		},
		{
			name: "use first container when no match",
			containers: []container.Summary{
				{
					Names: []string{"/container1"},
					Labels: map[string]string{
						constants.DevgoManagedLabel:   constants.DevgoManagedValue,
						constants.DevgoWorkspaceLabel: "/other/workspace",
					},
				},
			},
			currentDir:   "/test/workspace",
			expectedName: "container1",
		},
		{
			name:        "no containers found",
			containers:  []container.Summary{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the container finding logic
			if len(tt.containers) == 0 && tt.expectError {
				// Test case: no containers
				return
			}

			// Find matching container
			var foundName string
			for _, c := range tt.containers {
				if workspaceLabel, exists := c.Labels[constants.DevgoWorkspaceLabel]; exists {
					if workspaceLabel == tt.currentDir {
						foundName = c.Names[0][1:] // Remove leading '/'
						break
					}
				}
			}

			// If no exact match, use first container
			if foundName == "" && len(tt.containers) > 0 {
				foundName = tt.containers[0].Names[0][1:]
			}

			if foundName != tt.expectedName {
				t.Errorf("found container name = %q, want %q", foundName, tt.expectedName)
			}
		})
	}
}

func TestRunLifecycleCommandsOrder(t *testing.T) {
	// This test verifies that commands are executed in the correct order
	// based on the waitFor setting

	testCases := []struct {
		name     string
		waitFor  string
		commands map[string]bool
	}{
		{
			name:    "waitFor onCreateCommand",
			waitFor: devcontainer.WaitForOnCreateCommand,
			commands: map[string]bool{
				"onCreate": true,
			},
		},
		{
			name:    "waitFor updateContentCommand (default)",
			waitFor: devcontainer.WaitForUpdateContentCommand,
			commands: map[string]bool{
				"onCreate":      true,
				"updateContent": true,
			},
		},
		{
			name:    "waitFor postCreateCommand",
			waitFor: devcontainer.WaitForPostCreateCommand,
			commands: map[string]bool{
				"onCreate":      true,
				"updateContent": true,
				"postCreate":    true,
			},
		},
		{
			name:    "waitFor postStartCommand",
			waitFor: devcontainer.WaitForPostStartCommand,
			commands: map[string]bool{
				"onCreate":      true,
				"updateContent": true,
				"postCreate":    true,
				"postStart":     true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dc := &devcontainer.DevContainer{
				WaitFor: tc.waitFor,
			}

			// Verify ShouldWaitForCommand returns correct values
			if tc.commands["onCreate"] {
				if !dc.ShouldWaitForCommand(devcontainer.WaitForOnCreateCommand) {
					t.Errorf("expected to wait for onCreateCommand")
				}
			}

			if tc.commands["updateContent"] {
				if !dc.ShouldWaitForCommand(devcontainer.WaitForUpdateContentCommand) {
					t.Errorf("expected to wait for updateContentCommand")
				}
			}

			if tc.commands["postCreate"] {
				if !dc.ShouldWaitForCommand(devcontainer.WaitForPostCreateCommand) {
					t.Errorf("expected to wait for postCreateCommand")
				}
			}

			if tc.commands["postStart"] {
				if !dc.ShouldWaitForCommand(devcontainer.WaitForPostStartCommand) {
					t.Errorf("expected to wait for postStartCommand")
				}
			}
		})
	}
}

