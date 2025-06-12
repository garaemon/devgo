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
