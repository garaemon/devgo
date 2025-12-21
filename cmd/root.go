package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
	// "github.com/garaemon/devgo/pkg/config"
	// "github.com/garaemon/devgo/pkg/docker"
)

var (
	workspaceFolder string
	configPath      string
	forceBuild      bool
	containerName   string
	imageName       string
	sessionName     string
	push            bool
	pull            bool
	verbose         bool
	showHelp        bool
	showVersion     bool
)

// parseAllFlags parses all flags from the argument list, returning non-flag arguments
func parseAllFlags(args []string) ([]string, error) {
	var nonFlagArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--help" {
			showHelp = true
		} else if arg == "--version" {
			showVersion = true
		} else if arg == "--verbose" {
			verbose = true
		} else if arg == "--workspace-folder" && i+1 < len(args) {
			workspaceFolder = args[i+1]
			i++ // skip the next argument as it's the value
		} else if arg == "--config" && i+1 < len(args) {
			configPath = args[i+1]
			i++ // skip the next argument as it's the value
		} else if arg == "--name" && i+1 < len(args) {
			containerName = args[i+1]
			i++ // skip the next argument as it's the value
		} else if arg == "--image-name" && i+1 < len(args) {
			imageName = args[i+1]
			i++ // skip the next argument as it's the value
		} else if arg == "--session" && i+1 < len(args) {
			sessionName = args[i+1]
			i++ // skip the next argument as it's the value
		} else if arg == "--force-build" {
			forceBuild = true
		} else if arg == "--push" {
			push = true
		} else if arg == "--pull" {
			pull = true
		} else if len(arg) > 2 && arg[:2] == "--" {
			// Check if this is an unknown flag
			return nil, fmt.Errorf("unknown option: %s", arg)
		} else {
			nonFlagArgs = append(nonFlagArgs, arg)
		}
	}

	return nonFlagArgs, nil
}

func Execute() error {
	// Parse all flags from command line arguments
	args, err := parseAllFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		showUsage()
		return err
	}

	if showHelp {
		showUsage()
		return nil
	}

	if showVersion {
		showVersionInfo()
		return nil
	}

	if len(args) == 0 {
		return runDevContainer(args)
	}

	command := args[0]
	commandArgs := args[1:]

	switch command {
	case "up":
		return runUpCommand(commandArgs)
	case "build":
		return runBuildCommand(commandArgs)
	case "exec":
		return runExecCommand(commandArgs)
	case "shell":
		return runShellCommand(commandArgs)
	case "stop":
		return runStopCommand(commandArgs)
	case "down":
		return runDownCommand(commandArgs)
	case "list":
		return runListCommand(commandArgs)
	case "run-user-commands":
		return runUserCommandsCommand(commandArgs)
	case "read-configuration":
		return runReadConfigurationCommand(commandArgs)
	case "init":
		return runInitCommand(commandArgs)
	default:
		return runDevContainer(args)
	}
}

func showUsage() {
	fmt.Fprintf(os.Stderr, `devgo - Run commands in a devcontainer based on devcontainer.json

Usage:
  devgo [flags] [command] [args...]

Commands:
  up                       Create and run dev container
  build [path]            Build a dev container image
  exec <cmd> [args...]    Execute command in running container
  shell                   Start interactive bash shell in container
  stop                    Stop containers
  down                    Stop and delete containers
  list                    List all devgo containers
  run-user-commands       Run user commands in container
  read-configuration      Output current workspace configuration
  init [directory]        Initialize devcontainer.json template

Flags:
  --config string
        Path to devcontainer.json file
  --force-build
        Force rebuild of container
  --help
        Show help
  --image-name string
        Set image name and optional version
  --name string
        Override container name
  --push
        Publish the built image
  --pull
        Force pull image before starting container
  --session string
        Session name for running multiple containers (default "default")
  --verbose
        Enable verbose output
  --version
        Show version
  --workspace-folder string
        Path to workspace folder

Examples:
  devgo up --workspace-folder .
  devgo build --image-name myapp:latest
  devgo exec bash
  devgo shell
  devgo stop
`)
}

func showVersionInfo() {
	fmt.Println("devgo version 0.1.0")
}

func runDevContainer(args []string) error {
	// TODO: Implement actual functionality
	fmt.Printf("devgo called with args: %v\n", args)
	fmt.Printf("config: %s, build: %t, name: %s\n", configPath, forceBuild, containerName)

	devcontainerPath, err := findDevcontainerConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to find devcontainer config: %w", err)
	}

	fmt.Printf("Found devcontainer config at: %s\n", devcontainerPath)
	return nil
}

func findDevcontainerConfig(configPath string) (string, error) {
	if configPath != "" {
		return configPath, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for dir := cwd; dir != "/"; dir = filepath.Dir(dir) {
		if verbose {
			fmt.Printf("Checking directory: %s\n", dir)
		}

		configFile := filepath.Join(dir, ".devcontainer", "devcontainer.json")
		if _, err := os.Stat(configFile); err == nil {
			return configFile, nil
		}

		configFile = filepath.Join(dir, ".devcontainer.json")
		if _, err := os.Stat(configFile); err == nil {
			return configFile, nil
		}
	}

	return "", fmt.Errorf("no devcontainer.json found in current directory or parent directories")
}

func determineWorkspaceFolder(devcontainerPath string) string {
	if workspaceFolder != "" {
		absPath, err := filepath.Abs(workspaceFolder)
		if err != nil {
			return workspaceFolder
		}
		return absPath
	}
	// Convert to absolute path first to handle relative paths correctly
	absPath, err := filepath.Abs(devcontainerPath)
	if err != nil {
		// Fallback to original behavior if absolute path conversion fails
		return filepath.Dir(filepath.Dir(devcontainerPath))
	}
	// Use the directory containing the devcontainer.json as the workspace
	return filepath.Dir(filepath.Dir(absPath))
}

// GeneratePathHash generates a short hash from the given path for container naming
func GeneratePathHash(path string) string {
	h := sha256.New()
	h.Write([]byte(path))
	hash := hex.EncodeToString(h.Sum(nil))
	return hash[:8]
}

func determineContainerName(devContainer *devcontainer.DevContainer, workspaceDir string) string {
	if containerName != "" {
		return containerName
	}

	session := sessionName
	if session == "" {
		session = constants.DefaultSessionName
	}

	// For docker compose, use service name with project prefix
	if devContainer.HasDockerCompose() && devContainer.GetService() != "" {
		projectName := filepath.Base(workspaceDir)
		pathHash := GeneratePathHash(workspaceDir)
		return fmt.Sprintf("%s-%s-%s-1", pathHash, projectName, devContainer.GetService())
	}

	pathHash := GeneratePathHash(workspaceDir)
	baseName := ""
	if devContainer.Name != "" {
		baseName = devContainer.Name
	} else {
		baseName = filepath.Base(workspaceDir)
	}

	return fmt.Sprintf("%s-%s-%s", baseName, session, pathHash)
}
