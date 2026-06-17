package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	base, err := configBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "config.json"), nil
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

// configBaseDir returns the directory that holds devgo's per-user
// configuration, honoring XDG_CONFIG_HOME and falling back to ~/.config/devgo.
func configBaseDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "devgo"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "devgo"), nil
}

// ProfilesDir returns the directory that holds named global container
// profiles. Each profile is a subdirectory containing a devcontainer.json,
// e.g. ~/.config/devgo/profiles/go/devcontainer.json.
func ProfilesDir() (string, error) {
	base, err := configBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "profiles"), nil
}

// ValidateProfileName reports whether name is usable as a single profile
// directory component. A profile name may come from an untrusted source (the
// --profile flag or the DEVGO_PROFILE environment variable) and is joined into
// a filesystem path, so it must not contain path separators, traversal
// segments, or be absolute; otherwise it could escape the profiles directory.
func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name must not be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid profile name %q", name)
	}
	if filepath.IsAbs(name) ||
		strings.ContainsRune(name, '/') ||
		strings.ContainsRune(name, os.PathSeparator) {
		return fmt.Errorf("profile name %q must be a single path component", name)
	}
	return nil
}

// ProfilePath returns the path to the devcontainer.json for the named global
// profile. The name must be a single path component (see ValidateProfileName);
// path separators and traversal segments are rejected so the result always
// stays inside the profiles directory. It does not check whether the file
// exists.
func ProfilePath(name string) (string, error) {
	if err := ValidateProfileName(name); err != nil {
		return "", err
	}
	dir, err := ProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name, "devcontainer.json"), nil
}

// ListProfiles returns the names of available global profiles, sorted
// alphabetically. A profile is a subdirectory of ProfilesDir that contains a
// devcontainer.json. A missing profiles directory is not an error and yields
// an empty slice.
func ListProfiles() ([]string, error) {
	dir, err := ProfilesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read profiles directory %s: %w", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		configFile := filepath.Join(dir, entry.Name(), "devcontainer.json")
		if _, err := os.Stat(configFile); err == nil {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
