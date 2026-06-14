package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
	// "github.com/garaemon/devgo/pkg/config"
	// "github.com/garaemon/devgo/pkg/docker"
)

var (
	workspaceFolder        string
	configPath             string
	forceBuild             bool
	containerName          string
	imageName              string
	sessionName            string
	push                   bool
	pull                   bool
	debug                  bool
	showHelp               bool
	showVersion            bool
	dotfilesRepository     string
	dotfilesTargetPath     string
	dotfilesInstallCommand string
	noDotfiles             bool
	forceDotfiles          bool
	shellOverride          string
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
		} else if arg == "--debug" || arg == "--verbose" {
			debug = true
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
		} else if arg == "--dotfiles-repository" && i+1 < len(args) {
			dotfilesRepository = args[i+1]
			i++
		} else if arg == "--dotfiles-target-path" && i+1 < len(args) {
			dotfilesTargetPath = args[i+1]
			i++
		} else if arg == "--dotfiles-install-command" && i+1 < len(args) {
			dotfilesInstallCommand = args[i+1]
			i++
		} else if arg == "--no-dotfiles" {
			noDotfiles = true
		} else if arg == "--force-dotfiles" {
			forceDotfiles = true
		} else if arg == "--shell" && i+1 < len(args) {
			shellOverride = args[i+1]
			i++
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

// warnf prints a "Warning: ..." message to stderr. Use this for non-fatal
// problems where the command continues; reserve stdout for the command's
// real output so warnings don't pollute pipelines.
func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}

// debugf writes a status/progress message to stderr only when --debug is
// enabled. Use this for informational logs that would clutter normal output
// (container lifecycle, image pulls, dotfiles progress, etc.).
func debugf(format string, args ...any) {
	if !debug {
		return
	}
	fmt.Fprintf(os.Stderr, format, args...)
}

// debugln writes a line of status/progress to stderr only when --debug is
// enabled. Counterpart to debugf for callers that just want to emit a fixed
// string without formatting.
func debugln(args ...any) {
	if !debug {
		return
	}
	fmt.Fprintln(os.Stderr, args...)
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
  --debug
        Print container lifecycle, dotfiles, and other progress messages
        to stderr. Without this flag devgo stays quiet on success.
        --verbose is accepted as a deprecated alias.
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
  --version
        Show version
  --workspace-folder string
        Path to workspace folder
  --dotfiles-repository string
        URL of the personal dotfiles repository to clone into the container
  --dotfiles-target-path string
        Path inside the container where the dotfiles repo is cloned (default "~/dotfiles")
  --dotfiles-install-command string
        Install command to run after cloning; first token is the script path
        (relative to target path), remaining tokens are passed as arguments
  --no-dotfiles
        Disable dotfiles processing for this invocation
  --force-dotfiles
        Re-clone the dotfiles repository even if the target path already exists
  --shell string
        Program to launch for 'devgo shell' (overrides shell setting in user config; defaults to /bin/bash)

Examples:
  devgo up --workspace-folder .
  devgo build --image-name myapp:latest
  devgo exec bash
  devgo shell
  devgo stop
`)
}

func showVersionInfo() {
	fmt.Println("devgo version 0.2.0")
}

func runDevContainer(args []string) error {
	// TODO: Implement actual functionality
	debugf("devgo called with args: %v\n", args)
	debugf("config: %s, build: %t, name: %s\n", configPath, forceBuild, containerName)

	devcontainerPath, err := findDevcontainerConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to find devcontainer config: %w", err)
	}

	debugf("Found devcontainer config at: %s\n", devcontainerPath)
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
		debugf("Checking directory: %s\n", dir)

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

func sanitizeDockerName(name string) string {
	name = strings.ToLower(name)
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			result.WriteRune(r)
		} else {
			// Replace spaces and all other invalid characters with underscore
			result.WriteRune('_')
		}
	}
	return result.String()
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
		projectName := sanitizeDockerName(filepath.Base(workspaceDir))
		pathHash := GeneratePathHash(workspaceDir)
		return fmt.Sprintf("%s-%s-%s-1", pathHash, projectName, devContainer.GetService())
	}

	pathHash := GeneratePathHash(workspaceDir)
	baseName := ""
	if devContainer.Name != "" {
		baseName = sanitizeDockerName(devContainer.Name)
	} else {
		baseName = sanitizeDockerName(filepath.Base(workspaceDir))
	}

	return fmt.Sprintf("%s-%s-%s", baseName, session, pathHash)
}
