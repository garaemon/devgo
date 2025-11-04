package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runInitCommand(args []string) error {
	// Determine target directory
	targetDir, err := determineInitDirectory(args)
	if err != nil {
		return fmt.Errorf("failed to determine target directory: %w", err)
	}

	if verbose {
		fmt.Printf("Target directory: %s\n", targetDir)
	}

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(targetDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		return fmt.Errorf("failed to create .devcontainer directory: %w", err)
	}

	// Check if devcontainer.json already exists
	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if _, err := os.Stat(devcontainerPath); err == nil {
		return fmt.Errorf("devcontainer.json already exists at %s", devcontainerPath)
	}

	// Create default devcontainer.json template
	template := createDefaultTemplate()

	// Write to file
	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal template: %w", err)
	}

	if err := os.WriteFile(devcontainerPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write devcontainer.json: %w", err)
	}

	fmt.Printf("Created devcontainer.json at %s\n", devcontainerPath)
	return nil
}

func determineInitDirectory(args []string) (string, error) {
	// Check if directory is provided as argument
	if len(args) > 0 {
		targetDir := args[0]
		absPath, err := filepath.Abs(targetDir)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path: %w", err)
		}

		// Verify directory exists
		if stat, err := os.Stat(absPath); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("directory does not exist: %s", absPath)
			}
			return "", err
		} else if !stat.IsDir() {
			return "", fmt.Errorf("not a directory: %s", absPath)
		}

		return absPath, nil
	}

	// Default to git root
	gitRoot, err := findGitRoot()
	if err != nil {
		// If not in a git repository, use current directory
		if verbose {
			fmt.Fprintf(os.Stderr, "Warning: not in a git repository, using current directory\n")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		return cwd, nil
	}

	return gitRoot, nil
}

func findGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to find git root: %w", err)
	}

	gitRoot := strings.TrimSpace(string(output))
	return gitRoot, nil
}

func createDefaultTemplate() map[string]interface{} {
	return map[string]interface{}{
		"name":  "Development Container",
		"image": "ghcr.io/garaemon/ubuntu-noble:latest",
		"features": map[string]interface{}{},
		"customizations": map[string]interface{}{
			"vscode": map[string]interface{}{
				"extensions": []string{},
			},
		},
		"forwardPorts":      []interface{}{},
		"postCreateCommand": "",
	}
}
