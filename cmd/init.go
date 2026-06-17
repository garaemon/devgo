package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/garaemon/devgo/pkg/config"
)

func runInitCommand(args []string) error {
	// When --profile is given, scaffold a reusable global profile under the
	// home directory instead of a repo-local .devcontainer.
	if profileName != "" {
		return runInitProfile(profileName)
	}

	// Determine target directory
	targetDir, err := determineInitDirectory(args)
	if err != nil {
		return fmt.Errorf("failed to determine target directory: %w", err)
	}

	debugf("Target directory: %s\n", targetDir)

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
	if err := os.WriteFile(devcontainerPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write devcontainer.json: %w", err)
	}

	fmt.Printf("Created devcontainer.json at %s\n", devcontainerPath)
	return nil
}

// runInitProfile scaffolds a global container profile at
// ~/.config/devgo/profiles/<name>/devcontainer.json. The generated template
// uses the profile name as the devcontainer name so resulting containers are
// easy to identify.
func runInitProfile(name string) error {
	devcontainerPath, err := config.ProfilePath(name)
	if err != nil {
		return fmt.Errorf("failed to determine profile path: %w", err)
	}

	profileDir := filepath.Dir(devcontainerPath)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return fmt.Errorf("failed to create profile directory: %w", err)
	}

	if _, err := os.Stat(devcontainerPath); err == nil {
		return fmt.Errorf("profile %q already exists at %s", name, devcontainerPath)
	}

	template := strings.Replace(
		createDefaultTemplate(),
		`"name": "development-container"`,
		fmt.Sprintf(`"name": %q`, name),
		1,
	)

	if err := os.WriteFile(devcontainerPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write profile devcontainer.json: %w", err)
	}

	fmt.Printf("Created profile %q at %s\n", name, devcontainerPath)
	fmt.Printf("Use it from any directory with: devgo up --profile %s\n", name)
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
		debugf("Warning: not in a git repository, using current directory\n")
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

func createDefaultTemplate() string {
	return `{
  "name": "development-container",
  "image": "ghcr.io/garaemon/ubuntu-noble:latest",

  // Container build configuration
  // "dockerFile": "Dockerfile",
  // "build": {
  //   "dockerfile": "Dockerfile",
  //   "context": "..",
  //   "args": { }
  // },

  // Docker Compose configuration
  // "dockerComposeFile": "docker-compose.yml",
  // "service": "app",
  // "runServices": ["db", "redis"],

  // Workspace configuration
  // "workspaceFolder": "/workspace",
  // "workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind",

  // Additional mounts
  // "mounts": [
  //   "source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"
  // ],

  // Environment variables
  // "containerEnv": {
  //   "MY_VAR": "value"
  // },

  // User configuration
  // "remoteUser": "vscode",
  // "containerUser": "vscode",
  // "updateRemoteUserUID": true,

  // Lifecycle commands (host-side)
  // "initializeCommand": "echo 'Host initialization'",

  // Lifecycle commands (container-side)
  // "onCreateCommand": "echo 'Container created'",
  // "updateContentCommand": "echo 'Content updated'",
  // "postCreateCommand": "echo 'Post-create setup'",
  // "postStartCommand": "echo 'Container started'",
  // "postAttachCommand": "echo 'Attached to container'",

  // Control command execution order
  // "waitFor": "updateContentCommand",

  // Personal dotfiles (NOT a devcontainer.json key)
  // Dotfiles are personal and must not be committed here. Configure them
  // per-user in ~/.config/devgo/config.json instead, e.g.:
  //   {
  //     "dotfiles": {
  //       "repository": "https://github.com/your-user/dotfiles",
  //       "targetPath": "~/dotfiles",
  //       "installCommand": "install.sh"
  //     }
  //   }
  // Or override per run: devgo up --dotfiles-repository <url>
  // See docs/dotfiles.md for details.

  // Features to install
  "features": {},

  // VSCode customizations
  "customizations": {
    "vscode": {
      "extensions": []
    }
  },

  // Port forwarding
  "forwardPorts": []
}
`
}
