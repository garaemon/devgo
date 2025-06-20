package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	// "github.com/garaemon/devgo/pkg/config"
	// "github.com/garaemon/devgo/pkg/devcontainer"
	// "github.com/garaemon/devgo/pkg/docker"
)

var (
	workspaceFolder string
	configPath      string
	forceBuild      bool
	containerName   string
	imageName       string
	push            bool
	verbose         bool
	showHelp        bool
	showVersion     bool
)

// parseAllFlags parses all flags from the argument list, returning non-flag arguments
func parseAllFlags(args []string) []string {
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
		} else if arg == "--force-build" {
			forceBuild = true
		} else if arg == "--push" {
			push = true
		} else {
			nonFlagArgs = append(nonFlagArgs, arg)
		}
	}
	
	return nonFlagArgs
}

func Execute() error {
	// Parse all flags from command line arguments
	args := parseAllFlags(os.Args[1:])

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
  stop                    Stop containers
  down                    Stop and delete containers
  list                    List all devgo containers
  run-user-commands       Run user commands in container
  read-configuration      Output current workspace configuration

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
