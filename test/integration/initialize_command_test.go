package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/garaemon/devgo/pkg/devcontainer"
)

func TestInitializeCommandIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name            string
		setupFiles      func(workspaceDir string) error
		devcontainerDef map[string]interface{}
		expectError     bool
		expectedOutput  string
	}{
		{
			name: "string initialize command creates file",
			setupFiles: func(workspaceDir string) error {
				return nil
			},
			devcontainerDef: map[string]interface{}{
				"name":              "Initialize Command Test",
				"image":             "alpine:latest",
				"initializeCommand": "touch /tmp/initialize-test.txt",
			},
			expectError:    false,
			expectedOutput: "",
		},
		{
			name: "array initialize command with echo",
			setupFiles: func(workspaceDir string) error {
				return nil
			},
			devcontainerDef: map[string]interface{}{
				"name":              "Initialize Command Array Test",
				"image":             "alpine:latest",
				"initializeCommand": []string{"echo", "Initialize command executed"},
			},
			expectError:    false,
			expectedOutput: "Initialize command executed",
		},
		{
			name: "initialize command writes to workspace",
			setupFiles: func(workspaceDir string) error {
				return nil
			},
			devcontainerDef: map[string]interface{}{
				"name":              "Initialize Command Workspace Test",
				"image":             "alpine:latest",
				"initializeCommand": "echo 'Initialized' > init-output.txt",
			},
			expectError:    false,
			expectedOutput: "",
		},
		{
			name: "no initialize command",
			setupFiles: func(workspaceDir string) error {
				return nil
			},
			devcontainerDef: map[string]interface{}{
				"name":  "No Initialize Command Test",
				"image": "alpine:latest",
			},
			expectError:    false,
			expectedOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "devgo-init-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			workspaceDir := filepath.Join(tempDir, "workspace")
			if err := os.MkdirAll(workspaceDir, 0755); err != nil {
				t.Fatalf("Failed to create workspace dir: %v", err)
			}

			devcontainerDir := filepath.Join(workspaceDir, ".devcontainer")
			if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
				t.Fatalf("Failed to create .devcontainer dir: %v", err)
			}

			devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
			if err := writeDevcontainerJSON(devcontainerPath, tt.devcontainerDef); err != nil {
				t.Fatalf("Failed to write devcontainer.json: %v", err)
			}

			if err := tt.setupFiles(workspaceDir); err != nil {
				t.Fatalf("Failed to setup test files: %v", err)
			}

			devContainer, err := devcontainer.Parse(devcontainerPath)
			if err != nil {
				t.Fatalf("Failed to parse devcontainer.json: %v", err)
			}

			args := devContainer.GetInitializeCommandArgs()
			if len(args) == 0 {
				if tt.expectedOutput != "" {
					t.Errorf("Expected output '%s' but no command to run", tt.expectedOutput)
				}
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err = executeInitializeCommandForTest(ctx, args, workspaceDir)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if strings.Contains(tt.name, "writes to workspace") {
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
			}

			if strings.Contains(tt.name, "creates file") {
				testFile := "/tmp/initialize-test.txt"
				if _, err := os.Stat(testFile); os.IsNotExist(err) {
					t.Error("Expected test file to be created")
				}
				os.Remove(testFile)
			}
		})
	}
}

func executeInitializeCommandForTest(ctx context.Context, args []string, workspaceDir string) error {
	if len(args) == 0 {
		return nil
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = workspaceDir

	return cmd.Run()
}

func writeDevcontainerJSON(path string, def map[string]interface{}) error {
	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}