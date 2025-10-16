package devcontainer

import (
	"fmt"
	"os"

	"github.com/titanous/json5"
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

// waitFor lifecycle command constants
const (
	WaitForInitializeCommand    = "initializeCommand"
	WaitForOnCreateCommand      = "onCreateCommand"
	WaitForUpdateContentCommand = "updateContentCommand" // default
	WaitForPostCreateCommand    = "postCreateCommand"
	WaitForPostStartCommand     = "postStartCommand"
)

type Mount struct {
	Type   string `json:"type,omitempty"`
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
}

type DevContainer struct {
	Name                 string                    `json:"name,omitempty"`
	Image                string                    `json:"image,omitempty"`
	Build                *BuildConfig              `json:"build,omitempty"`
	DockerComposeFile    interface{}               `json:"dockerComposeFile,omitempty"`
	Service              string                    `json:"service,omitempty"`
	RunServices          []string                  `json:"runServices,omitempty"`
	WorkspaceFolder      string                    `json:"workspaceFolder,omitempty"`
	ContainerUser        string                    `json:"containerUser,omitempty"`
	RemoteUser           string                    `json:"remoteUser,omitempty"`
	ContainerEnv         map[string]string         `json:"containerEnv,omitempty"`
	RemoteEnv            map[string]string         `json:"remoteEnv,omitempty"`
	Mounts               []Mount                   `json:"mounts,omitempty"`
	ForwardPorts         []interface{}             `json:"forwardPorts,omitempty"`
	PortsAttributes      map[string]PortAttributes `json:"portsAttributes,omitempty"`
	InitializeCommand    interface{}               `json:"initializeCommand,omitempty"`
	OnCreateCommand      interface{}               `json:"onCreateCommand,omitempty"`
	UpdateContentCommand interface{}               `json:"updateContentCommand,omitempty"`
	PostCreateCommand    interface{}               `json:"postCreateCommand,omitempty"`
	PostStartCommand     interface{}               `json:"postStartCommand,omitempty"`
	PostAttachCommand    interface{}               `json:"postAttachCommand,omitempty"`
	WaitFor              string                    `json:"waitFor,omitempty"`
}

func Parse(filePath string) (*DevContainer, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read devcontainer file: %w", err)
	}

	var devContainer DevContainer
	if err := json5.Unmarshal(data, &devContainer); err != nil {
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

func (dc *DevContainer) GetInitializeCommandArgs() []string {
	if dc.InitializeCommand == nil {
		return nil
	}
	return parseCommand(dc.InitializeCommand)
}

func (dc *DevContainer) GetOnCreateCommandArgs() []string {
	if dc.OnCreateCommand == nil {
		return nil
	}
	return parseCommand(dc.OnCreateCommand)
}

func (dc *DevContainer) GetUpdateContentCommandArgs() []string {
	if dc.UpdateContentCommand == nil {
		return nil
	}
	return parseCommand(dc.UpdateContentCommand)
}

func (dc *DevContainer) GetPostCreateCommandArgs() []string {
	if dc.PostCreateCommand == nil {
		return nil
	}
	return parseCommand(dc.PostCreateCommand)
}

func (dc *DevContainer) GetPostStartCommandArgs() []string {
	if dc.PostStartCommand == nil {
		return nil
	}
	return parseCommand(dc.PostStartCommand)
}

func (dc *DevContainer) GetPostAttachCommandArgs() []string {
	if dc.PostAttachCommand == nil {
		return nil
	}
	return parseCommand(dc.PostAttachCommand)
}

func (dc *DevContainer) GetWaitFor() string {
	if dc.WaitFor != "" {
		return dc.WaitFor
	}
	return WaitForUpdateContentCommand
}

func (dc *DevContainer) ShouldWaitForCommand(commandType string) bool {
	waitFor := dc.GetWaitFor()

	switch waitFor {
	case WaitForInitializeCommand:
		return commandType == WaitForInitializeCommand
	case WaitForOnCreateCommand:
		return commandType == WaitForInitializeCommand ||
			commandType == WaitForOnCreateCommand
	case WaitForUpdateContentCommand:
		return commandType == WaitForInitializeCommand ||
			commandType == WaitForOnCreateCommand ||
			commandType == WaitForUpdateContentCommand
	case WaitForPostCreateCommand:
		return commandType == WaitForInitializeCommand ||
			commandType == WaitForOnCreateCommand ||
			commandType == WaitForUpdateContentCommand ||
			commandType == WaitForPostCreateCommand
	case WaitForPostStartCommand:
		return commandType == WaitForInitializeCommand ||
			commandType == WaitForOnCreateCommand ||
			commandType == WaitForUpdateContentCommand ||
			commandType == WaitForPostCreateCommand ||
			commandType == WaitForPostStartCommand
	default:
		return false
	}
}

func (dc *DevContainer) HasDockerCompose() bool {
	return dc.DockerComposeFile != nil
}

func (dc *DevContainer) GetDockerComposeFiles() []string {
	if dc.DockerComposeFile == nil {
		return nil
	}

	switch v := dc.DockerComposeFile.(type) {
	case string:
		return []string{v}
	case []interface{}:
		var files []string
		for _, file := range v {
			if str, ok := file.(string); ok {
				files = append(files, str)
			}
		}
		return files
	default:
		return nil
	}
}

func (dc *DevContainer) GetService() string {
	return dc.Service
}

func (dc *DevContainer) GetRunServices() []string {
	return dc.RunServices
}

func parseCommand(cmd interface{}) []string {
	if cmd == nil {
		return nil
	}

	switch v := cmd.(type) {
	case string:
		// String commands are executed through shell to support shell features
		// like pipes, redirects, variable expansion, and command chaining (&&, ||)
		// Example: "npm install && npm run build"
		return []string{"/bin/sh", "-c", v}
	case []interface{}:
		// Array commands are executed directly without shell interpretation
		// for better security and performance when shell features are not needed
		// Example: ["npm", "install"]
		var args []string
		for _, arg := range v {
			if str, ok := arg.(string); ok {
				args = append(args, str)
			}
		}
		return args
	default:
		return nil
	}
}
