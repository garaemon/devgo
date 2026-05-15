package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	DockerHost      string
	ContainerPrefix string
	WorkspaceMount  string
}

// DotfilesConfig holds the persistent dotfiles preferences loaded from
// ~/.config/devgo/config.json. CLI flags may override these values at run
// time; that merging is performed by pkg/dotfiles, not here.
type DotfilesConfig struct {
	Repository     string `json:"repository,omitempty"`
	TargetPath     string `json:"targetPath,omitempty"`
	InstallCommand string `json:"installCommand,omitempty"`
}

// UserConfig represents the contents of ~/.config/devgo/config.json. It is
// kept separate from Config (which is environment-derived) so each loader can
// evolve independently.
type UserConfig struct {
	// Dotfiles is the personal dotfiles repository configuration. Nil
	// means dotfiles are not configured for this user.
	Dotfiles *DotfilesConfig `json:"dotfiles,omitempty"`
	// Shell overrides the program devgo invokes for `devgo shell`. When
	// empty, devgo falls back to /bin/bash. Always launched with -i.
	Shell string `json:"shell,omitempty"`
}

func Load() (*Config, error) {
	cfg := &Config{
		DockerHost:      getEnv("DOCKER_HOST", ""),
		ContainerPrefix: getEnv("DEVGO_CONTAINER_PREFIX", "devgo-"),
		WorkspaceMount:  getEnv("DEVGO_WORKSPACE_MOUNT", "/workspace"),
	}

	return cfg, nil
}

// UserConfigPath returns the path to the per-user devgo config file. It
// honors XDG_CONFIG_HOME when set, and falls back to ~/.config/devgo/config.json.
func UserConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "devgo", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "devgo", "config.json"), nil
}

// LoadUserConfig loads the user-global config from the default location. A
// missing file is not an error and yields an empty UserConfig.
func LoadUserConfig() (*UserConfig, error) {
	path, err := UserConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadUserConfigFile(path)
}

// LoadUserConfigFile reads a UserConfig from the given path. A missing file
// is treated as an empty configuration (no error).
func LoadUserConfigFile(path string) (*UserConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &UserConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read user config %s: %w", path, err)
	}

	var cfg UserConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse user config %s: %w", path, err)
	}
	return &cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
