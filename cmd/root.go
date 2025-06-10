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
	configPath    = flag.String("config", "", "Path to devcontainer.json file")
	forceBuild    = flag.Bool("build", false, "Force rebuild of container")
	containerName = flag.String("name", "", "Override container name")
	showHelp      = flag.Bool("help", false, "Show help")
)

func Execute() error {
	flag.Parse()
	
	if *showHelp {
		showUsage()
		return nil
	}
	
	args := flag.Args()
	return runDevContainer(args)
}

func showUsage() {
	fmt.Fprintf(os.Stderr, `devgo - Run commands in a devcontainer based on devcontainer.json

Usage:
  devgo [flags] [command]

Flags:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Examples:
  devgo                    # Start interactive shell
  devgo bash               # Run bash command
  devgo --build            # Force rebuild container
`)
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