package devcontainer

import (
	"encoding/json"
	"fmt"
	"os"
)

type BuildConfig struct {
	Dockerfile string                 `json:"dockerfile,omitempty"`
	Context    string                 `json:"context,omitempty"`
	Args       map[string]interface{} `json:"args,omitempty"`
}

type PortAttributes struct {
	Label         string `json:"label,omitempty"`
	OnAutoForward string `json:"onAutoForward,omitempty"`
}

type DevContainer struct {
	Name              string                    `json:"name,omitempty"`
	Image             string                    `json:"image,omitempty"`
	Build             *BuildConfig              `json:"build,omitempty"`
	WorkspaceFolder   string                    `json:"workspaceFolder,omitempty"`
	ContainerUser     string                    `json:"containerUser,omitempty"`
	RemoteUser        string                    `json:"remoteUser,omitempty"`
	ContainerEnv      map[string]string         `json:"containerEnv,omitempty"`
	ForwardPorts      []interface{}             `json:"forwardPorts,omitempty"`
	PortsAttributes   map[string]PortAttributes `json:"portsAttributes,omitempty"`
	OnCreateCommand   interface{}               `json:"onCreateCommand,omitempty"`
	PostCreateCommand interface{}               `json:"postCreateCommand,omitempty"`
	PostStartCommand  interface{}               `json:"postStartCommand,omitempty"`
}

func Parse(filePath string) (*DevContainer, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read devcontainer file: %w", err)
	}

	var devContainer DevContainer
	if err := json.Unmarshal(data, &devContainer); err != nil {
		return nil, fmt.Errorf("failed to parse devcontainer.json: %w", err)
	}

	return &devContainer, nil
}

func (dc *DevContainer) HasImage() bool {
	return dc.Image != ""
}

func (dc *DevContainer) HasBuild() bool {
	return dc.Build != nil && dc.Build.Dockerfile != ""
}

func (dc *DevContainer) GetWorkspaceFolder() string {
	if dc.WorkspaceFolder != "" {
		return dc.WorkspaceFolder
	}
	return "/workspace"
}

func (dc *DevContainer) GetContainerUser() string {
	if dc.ContainerUser != "" {
		return dc.ContainerUser
	}
	return "root"
}
