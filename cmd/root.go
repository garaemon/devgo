package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	// "github.com/garaemon/devgo/pkg/config"
	// "github.com/garaemon/devgo/pkg/devcontainer"
	// "github.com/garaemon/devgo/pkg/docker"
)

var (
	workspaceFolder = flag.String("workspace-folder", "", "Path to workspace folder")
	configPath      = flag.String("config", "", "Path to devcontainer.json file")
	forceBuild      = flag.Bool("force-build", false, "Force rebuild of container")
	containerName   = flag.String("name", "", "Override container name")
	imageName       = flag.String("image-name", "", "Set image name and optional version")
	push            = flag.Bool("push", false, "Publish the built image")
	showHelp        = flag.Bool("help", false, "Show help")
	showVersion     = flag.Bool("version", false, "Show version")
)

func Execute() error {
	flag.Parse()

	if *showHelp {
		showUsage()
		return nil
	}

	if *showVersion {
		showVersionInfo()
		return nil
	}

	args := flag.Args()
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
  run-user-commands       Run user commands in container
  read-configuration      Output current workspace configuration

Flags:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
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
	fmt.Printf("config: %s, build: %t, name: %s\n", *configPath, *forceBuild, *containerName)

	devcontainerPath, err := findDevcontainerConfig(*configPath)
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
