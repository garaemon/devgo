package devcontainer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_SimpleImage(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "test", "fixtures", "simple-image.json")

	dc, err := Parse(fixturePath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if dc.Name != "Go Dev Container" {
		t.Errorf("Name = %v, want %v", dc.Name, "Go Dev Container")
	}

	if dc.Image != "golang:1.21" {
		t.Errorf("Image = %v, want %v", dc.Image, "golang:1.21")
	}

	if dc.WorkspaceFolder != "/workspace" {
		t.Errorf("WorkspaceFolder = %v, want %v", dc.WorkspaceFolder, "/workspace")
	}

	if dc.ContainerUser != "vscode" {
		t.Errorf("ContainerUser = %v, want %v", dc.ContainerUser, "vscode")
	}

	if len(dc.ContainerEnv) != 2 {
		t.Errorf("ContainerEnv length = %v, want %v", len(dc.ContainerEnv), 2)
	}

	if dc.ContainerEnv["GO111MODULE"] != "on" {
		t.Errorf("ContainerEnv[GO111MODULE] = %v, want %v", dc.ContainerEnv["GO111MODULE"], "on")
	}

	if len(dc.ForwardPorts) != 2 {
		t.Errorf("ForwardPorts length = %v, want %v", len(dc.ForwardPorts), 2)
	}
}

func TestParse_DockerfileBuild(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "test", "fixtures", "dockerfile-build.json")

	dc, err := Parse(fixturePath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if dc.Name != "Custom Build Container" {
		t.Errorf("Name = %v, want %v", dc.Name, "Custom Build Container")
	}

	if dc.Build == nil {
		t.Fatal("Build should not be nil")
	}

	if dc.Build.Dockerfile != "Dockerfile" {
		t.Errorf("Build.Dockerfile = %v, want %v", dc.Build.Dockerfile, "Dockerfile")
	}

	if dc.Build.Context != "." {
		t.Errorf("Build.Context = %v, want %v", dc.Build.Context, ".")
	}

	if len(dc.Build.Args) != 2 {
		t.Errorf("Build.Args length = %v, want %v", len(dc.Build.Args), 2)
	}

	if dc.RemoteUser != "developer" {
		t.Errorf("RemoteUser = %v, want %v", dc.RemoteUser, "developer")
	}

	if len(dc.PortsAttributes) != 1 {
		t.Errorf("PortsAttributes length = %v, want %v", len(dc.PortsAttributes), 1)
	}

	if dc.PortsAttributes["8000"].Label != "Django Dev Server" {
		t.Errorf("PortsAttributes[8000].Label = %v, want %v",
			dc.PortsAttributes["8000"].Label, "Django Dev Server")
	}
}

func TestParse_Minimal(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "test", "fixtures", "minimal.json")

	dc, err := Parse(fixturePath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if dc.Image != "ubuntu:22.04" {
		t.Errorf("Image = %v, want %v", dc.Image, "ubuntu:22.04")
	}

	if dc.Name != "" {
		t.Errorf("Name = %v, want empty string", dc.Name)
	}
}

func TestParse_UpdateContentCommand(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "test", "fixtures", "update-content-command.json")

	dc, err := Parse(fixturePath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if dc.Name != "Update Content Command Test" {
		t.Errorf("Name = %v, want %v", dc.Name, "Update Content Command Test")
	}

	if dc.Image != "node:18" {
		t.Errorf("Image = %v, want %v", dc.Image, "node:18")
	}

	args := dc.GetUpdateContentCommandArgs()
	expected := []string{"/bin/sh", "-c", "npm update && npm audit fix"}
	if len(args) != len(expected) {
		t.Errorf("GetUpdateContentCommandArgs() length = %v, want %v", len(args), len(expected))
		return
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("GetUpdateContentCommandArgs()[%d] = %v, want %v", i, arg, expected[i])
		}
	}
}

func TestParse_NonExistentFile(t *testing.T) {
	_, err := Parse("nonexistent.json")
	if err == nil {
		t.Error("Parse() should return error for non-existent file")
	}
}

func TestParse_BrokenJSON(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "broken-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	brokenJSON := `{
		"name": "Broken Container",
		"image": "golang:1.21"
		"workspaceFolder": "/workspace"
	}`

	if _, err := tmpFile.WriteString(brokenJSON); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	_, err = Parse(tmpFile.Name())
	if err == nil {
		t.Error("Parse() should return error for broken JSON")
	}

	if !strings.Contains(err.Error(), "failed to parse devcontainer.json") {
		t.Errorf("Expected error message to contain 'failed to parse devcontainer.json', got: %v", err)
	}
}

func TestHasImage(t *testing.T) {
	tests := []struct {
		name     string
		dc       DevContainer
		expected bool
	}{
		{
			name:     "has image",
			dc:       DevContainer{Image: "golang:1.21"},
			expected: true,
		},
		{
			name:     "no image",
			dc:       DevContainer{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dc.HasImage(); got != tt.expected {
				t.Errorf("HasImage() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHasBuild(t *testing.T) {
	tests := []struct {
		name     string
		dc       DevContainer
		expected bool
	}{
		{
			name: "has build",
			dc: DevContainer{
				Build: &BuildConfig{Dockerfile: "Dockerfile"},
			},
			expected: true,
		},
		{
			name:     "no build",
			dc:       DevContainer{},
			expected: false,
		},
		{
			name: "build without dockerfile",
			dc: DevContainer{
				Build: &BuildConfig{Context: "."},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dc.HasBuild(); got != tt.expected {
				t.Errorf("HasBuild() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetWorkspaceFolder(t *testing.T) {
	tests := []struct {
		name     string
		dc       DevContainer
		expected string
	}{
		{
			name:     "custom workspace folder",
			dc:       DevContainer{WorkspaceFolder: "/app"},
			expected: "/app",
		},
		{
			name:     "default workspace folder",
			dc:       DevContainer{},
			expected: "/workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dc.GetWorkspaceFolder(); got != tt.expected {
				t.Errorf("GetWorkspaceFolder() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetContainerUser(t *testing.T) {
	tests := []struct {
		name     string
		dc       DevContainer
		expected string
	}{
		{
			name:     "custom container user",
			dc:       DevContainer{ContainerUser: "vscode"},
			expected: "vscode",
		},
		{
			name:     "default container user",
			dc:       DevContainer{},
			expected: "root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dc.GetContainerUser(); got != tt.expected {
				t.Errorf("GetContainerUser() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetOnCreateCommandArgs(t *testing.T) {
	tests := []struct {
		name     string
		dc       DevContainer
		expected []string
	}{
		{
			name:     "no on create command",
			dc:       DevContainer{},
			expected: nil,
		},
		{
			name: "string on create command",
			dc: DevContainer{
				OnCreateCommand: "apt-get update",
			},
			expected: []string{"/bin/sh", "-c", "apt-get update"},
		},
		{
			name: "array on create command",
			dc: DevContainer{
				OnCreateCommand: []interface{}{"apt-get", "update"},
			},
			expected: []string{"apt-get", "update"},
		},
		{
			name: "mixed array with non-string (should be ignored)",
			dc: DevContainer{
				OnCreateCommand: []interface{}{"apt-get", 123, "update"},
			},
			expected: []string{"apt-get", "update"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dc.GetOnCreateCommandArgs()
			if len(got) != len(tt.expected) {
				t.Errorf("GetOnCreateCommandArgs() length = %v, want %v", len(got), len(tt.expected))
				return
			}
			for i, arg := range got {
				if arg != tt.expected[i] {
					t.Errorf("GetOnCreateCommandArgs()[%d] = %v, want %v", i, arg, tt.expected[i])
				}
			}
		})
	}
}

func TestGetPostStartCommandArgs(t *testing.T) {
	tests := []struct {
		name     string
		command  interface{}
		expected []string
	}{
		{
			name:     "nil command",
			command:  nil,
			expected: nil,
		},
		{
			name:     "string command",
			command:  "npm start",
			expected: []string{"/bin/sh", "-c", "npm start"},
		},
		{
			name:     "empty string command",
			command:  "",
			expected: []string{"/bin/sh", "-c", ""},
		},
		{
			name:     "array command",
			command:  []interface{}{"npm", "run", "start"},
			expected: []string{"npm", "run", "start"},
		},
		{
			name:     "empty array command",
			command:  []interface{}{},
			expected: []string{},
		},
		{
			name:     "array command with non-string elements",
			command:  []interface{}{"echo", 123, "hello"},
			expected: []string{"echo", "hello"},
		},
		{
			name:     "invalid command type",
			command:  123,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := &DevContainer{
				PostStartCommand: tt.command,
			}

			result := dc.GetPostStartCommandArgs()

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected length %d, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected result[%d] = %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

func TestGetUpdateContentCommandArgs(t *testing.T) {
	tests := []struct {
		name     string
		dc       DevContainer
		expected []string
	}{
		{
			name:     "no update content command",
			dc:       DevContainer{},
			expected: nil,
		},
		{
			name: "string update content command",
			dc: DevContainer{
				UpdateContentCommand: "npm update",
			},
			expected: []string{"/bin/sh", "-c", "npm update"},
		},
		{
			name: "array update content command",
			dc: DevContainer{
				UpdateContentCommand: []interface{}{"pip", "install", "--upgrade", "-r", "requirements.txt"},
			},
			expected: []string{"pip", "install", "--upgrade", "-r", "requirements.txt"},
		},
		{
			name: "mixed array with non-string (should be ignored)",
			dc: DevContainer{
				UpdateContentCommand: []interface{}{"pip", "install", 123, "--upgrade"},
			},
			expected: []string{"pip", "install", "--upgrade"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dc.GetUpdateContentCommandArgs()
			if len(got) != len(tt.expected) {
				t.Errorf("GetUpdateContentCommandArgs() length = %v, want %v", len(got), len(tt.expected))
				return
			}
			for i, arg := range got {
				if arg != tt.expected[i] {
					t.Errorf("GetUpdateContentCommandArgs()[%d] = %v, want %v", i, arg, tt.expected[i])
				}
			}
		})
	}
}

func TestGetPostCreateCommandArgs(t *testing.T) {
	tests := []struct {
		name     string
		dc       DevContainer
		expected []string
	}{
		{
			name:     "no post create command",
			dc:       DevContainer{},
			expected: nil,
		},
		{
			name: "string post create command",
			dc: DevContainer{
				PostCreateCommand: "npm install",
			},
			expected: []string{"/bin/sh", "-c", "npm install"},
		},
		{
			name: "array post create command",
			dc: DevContainer{
				PostCreateCommand: []interface{}{"npm", "install", "--global", "yarn"},
			},
			expected: []string{"npm", "install", "--global", "yarn"},
		},
		{
			name: "mixed array with non-string (should be ignored)",
			dc: DevContainer{
				PostCreateCommand: []interface{}{"npm", "install", 123, "yarn"},
			},
			expected: []string{"npm", "install", "yarn"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dc.GetPostCreateCommandArgs()
			if len(got) != len(tt.expected) {
				t.Errorf("GetPostCreateCommandArgs() length = %v, want %v", len(got), len(tt.expected))
				return
			}
			for i, arg := range got {
				if arg != tt.expected[i] {
					t.Errorf("GetPostCreateCommandArgs()[%d] = %v, want %v", i, arg, tt.expected[i])
				}
			}
		})
	}
}

func TestGetInitializeCommandArgs(t *testing.T) {
	tests := []struct {
		name     string
		dc       DevContainer
		expected []string
	}{
		{
			name:     "no initialize command",
			dc:       DevContainer{},
			expected: nil,
		},
		{
			name: "string initialize command",
			dc: DevContainer{
				InitializeCommand: "git config --global user.name test",
			},
			expected: []string{"/bin/sh", "-c", "git config --global user.name test"},
		},
		{
			name: "array initialize command",
			dc: DevContainer{
				InitializeCommand: []interface{}{"git", "config", "--global", "user.name", "test"},
			},
			expected: []string{"git", "config", "--global", "user.name", "test"},
		},
		{
			name: "mixed array with non-string (should be ignored)",
			dc: DevContainer{
				InitializeCommand: []interface{}{"git", "config", 123, "user.name", "test"},
			},
			expected: []string{"git", "config", "user.name", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dc.GetInitializeCommandArgs()
			if len(got) != len(tt.expected) {
				t.Errorf("GetInitializeCommandArgs() length = %v, want %v", len(got), len(tt.expected))
				return
			}
			for i, arg := range got {
				if arg != tt.expected[i] {
					t.Errorf("GetInitializeCommandArgs()[%d] = %v, want %v", i, arg, tt.expected[i])
				}
			}
		})
	}
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmd      interface{}
		expected []string
	}{
		{
			name:     "nil command",
			cmd:      nil,
			expected: nil,
		},
		{
			name:     "string command",
			cmd:      "echo hello",
			expected: []string{"/bin/sh", "-c", "echo hello"},
		},
		{
			name:     "array command",
			cmd:      []interface{}{"echo", "hello", "world"},
			expected: []string{"echo", "hello", "world"},
		},
		{
			name:     "empty array command",
			cmd:      []interface{}{},
			expected: []string{},
		},
		{
			name:     "array with non-string elements",
			cmd:      []interface{}{"echo", 123, "world"},
			expected: []string{"echo", "world"},
		},
		{
			name:     "unsupported type",
			cmd:      123,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommand(tt.cmd)
			if len(got) != len(tt.expected) {
				t.Errorf("parseCommand() length = %v, want %v", len(got), len(tt.expected))
				return
			}
			for i, arg := range got {
				if arg != tt.expected[i] {
					t.Errorf("parseCommand()[%d] = %v, want %v", i, arg, tt.expected[i])
				}
			}
		})
	}
}

func TestGetPostAttachCommandArgs(t *testing.T) {
	tests := []struct {
		name     string
		command  interface{}
		expected []string
	}{
		{
			name:     "nil command",
			command:  nil,
			expected: nil,
		},
		{
			name:     "string command",
			command:  "code .",
			expected: []string{"/bin/sh", "-c", "code ."},
		},
		{
			name:     "empty string command",
			command:  "",
			expected: []string{"/bin/sh", "-c", ""},
		},
		{
			name:     "array command",
			command:  []interface{}{"code", "--new-window", "."},
			expected: []string{"code", "--new-window", "."},
		},
		{
			name:     "empty array command",
			command:  []interface{}{},
			expected: []string{},
		},
		{
			name:     "array command with non-string elements",
			command:  []interface{}{"echo", 123, "attached"},
			expected: []string{"echo", "attached"},
		},
		{
			name:     "invalid command type",
			command:  123,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := &DevContainer{
				PostAttachCommand: tt.command,
			}

			result := dc.GetPostAttachCommandArgs()

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected length %d, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected result[%d] = %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

func TestDevContainer_GetWaitFor(t *testing.T) {
	tests := []struct {
		name     string
		waitFor  string
		expected string
	}{
		{
			name:     "default value",
			waitFor:  "",
			expected: WaitForUpdateContentCommand,
		},
		{
			name:     "initializeCommand",
			waitFor:  WaitForInitializeCommand,
			expected: WaitForInitializeCommand,
		},
		{
			name:     "onCreateCommand",
			waitFor:  WaitForOnCreateCommand,
			expected: WaitForOnCreateCommand,
		},
		{
			name:     "updateContentCommand",
			waitFor:  WaitForUpdateContentCommand,
			expected: WaitForUpdateContentCommand,
		},
		{
			name:     "postCreateCommand",
			waitFor:  WaitForPostCreateCommand,
			expected: WaitForPostCreateCommand,
		},
		{
			name:     "postStartCommand",
			waitFor:  WaitForPostStartCommand,
			expected: WaitForPostStartCommand,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := &DevContainer{WaitFor: tt.waitFor}
			got := dc.GetWaitFor()
			if got != tt.expected {
				t.Errorf("GetWaitFor() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDevContainer_ShouldWaitForCommand(t *testing.T) {
	tests := []struct {
		name        string
		waitFor     string
		commandType string
		expected    bool
	}{
		// waitFor = initializeCommand
		{
			name:        "waitFor initializeCommand, check initializeCommand",
			waitFor:     WaitForInitializeCommand,
			commandType: WaitForInitializeCommand,
			expected:    true,
		},
		{
			name:        "waitFor initializeCommand, check onCreateCommand",
			waitFor:     WaitForInitializeCommand,
			commandType: WaitForOnCreateCommand,
			expected:    false,
		},
		// waitFor = onCreateCommand
		{
			name:        "waitFor onCreateCommand, check initializeCommand",
			waitFor:     WaitForOnCreateCommand,
			commandType: WaitForInitializeCommand,
			expected:    true,
		},
		{
			name:        "waitFor onCreateCommand, check onCreateCommand",
			waitFor:     WaitForOnCreateCommand,
			commandType: WaitForOnCreateCommand,
			expected:    true,
		},
		{
			name:        "waitFor onCreateCommand, check updateContentCommand",
			waitFor:     WaitForOnCreateCommand,
			commandType: WaitForUpdateContentCommand,
			expected:    false,
		},
		// waitFor = updateContentCommand (default)
		{
			name:        "waitFor updateContentCommand, check onCreateCommand",
			waitFor:     WaitForUpdateContentCommand,
			commandType: WaitForOnCreateCommand,
			expected:    true,
		},
		{
			name:        "waitFor updateContentCommand, check updateContentCommand",
			waitFor:     WaitForUpdateContentCommand,
			commandType: WaitForUpdateContentCommand,
			expected:    true,
		},
		{
			name:        "waitFor updateContentCommand, check postCreateCommand",
			waitFor:     WaitForUpdateContentCommand,
			commandType: WaitForPostCreateCommand,
			expected:    false,
		},
		// waitFor = postCreateCommand
		{
			name:        "waitFor postCreateCommand, check postCreateCommand",
			waitFor:     WaitForPostCreateCommand,
			commandType: WaitForPostCreateCommand,
			expected:    true,
		},
		{
			name:        "waitFor postCreateCommand, check postStartCommand",
			waitFor:     WaitForPostCreateCommand,
			commandType: WaitForPostStartCommand,
			expected:    false,
		},
		// waitFor = postStartCommand
		{
			name:        "waitFor postStartCommand, check postStartCommand",
			waitFor:     WaitForPostStartCommand,
			commandType: WaitForPostStartCommand,
			expected:    true,
		},
		{
			name:        "waitFor postStartCommand, check postAttachCommand",
			waitFor:     WaitForPostStartCommand,
			commandType: "postAttachCommand",
			expected:    false,
		},
		// Invalid waitFor value
		{
			name:        "invalid waitFor value",
			waitFor:     "invalidCommand",
			commandType: WaitForOnCreateCommand,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := &DevContainer{WaitFor: tt.waitFor}
			got := dc.ShouldWaitForCommand(tt.commandType)
			if got != tt.expected {
				t.Errorf("ShouldWaitForCommand() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParse_PostStartCommand(t *testing.T) {
	dc, err := Parse("../../test/fixtures/post-start-command.json")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if dc.PostStartCommand != "npm start" {
		t.Errorf("Expected PostStartCommand = 'npm start', got %v", dc.PostStartCommand)
	}

	args := dc.GetPostStartCommandArgs()
	expected := []string{"/bin/sh", "-c", "npm start"}

	if len(args) != len(expected) {
		t.Errorf("Expected %d args, got %d", len(expected), len(args))
		return
	}

	for i, expected := range expected {
		if args[i] != expected {
			t.Errorf("Expected args[%d] = %q, got %q", i, expected, args[i])
		}
	}
}

func TestParse_WaitFor(t *testing.T) {
	tests := []struct {
		name     string
		dc       DevContainer
		expected string
	}{
		{
			name:     "no waitFor specified",
			dc:       DevContainer{},
			expected: WaitForUpdateContentCommand,
		},
		{
			name:     "waitFor postCreateCommand",
			dc:       DevContainer{WaitFor: WaitForPostCreateCommand},
			expected: WaitForPostCreateCommand,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dc.GetWaitFor()
			if got != tt.expected {
				t.Errorf("GetWaitFor() = %v, want %v", got, tt.expected)
			}
		})
	}
}
