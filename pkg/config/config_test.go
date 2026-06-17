package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Clear environment variables to test default values
	os.Unsetenv("DOCKER_HOST")
	os.Unsetenv("DEVGO_CONTAINER_PREFIX")
	os.Unsetenv("DEVGO_WORKSPACE_MOUNT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DockerHost != "" {
		t.Errorf("DockerHost = %v, want empty string", cfg.DockerHost)
	}

	if cfg.ContainerPrefix != "devgo-" {
		t.Errorf("ContainerPrefix = %v, want %v", cfg.ContainerPrefix, "devgo-")
	}

	if cfg.WorkspaceMount != "/workspace" {
		t.Errorf("WorkspaceMount = %v, want %v", cfg.WorkspaceMount, "/workspace")
	}
}

func TestLoad_WithEnvironmentVariables(t *testing.T) {
	// Set environment variables
	os.Setenv("DOCKER_HOST", "tcp://localhost:2376")
	os.Setenv("DEVGO_CONTAINER_PREFIX", "test-")
	os.Setenv("DEVGO_WORKSPACE_MOUNT", "/test")

	// Clean up after test
	defer func() {
		os.Unsetenv("DOCKER_HOST")
		os.Unsetenv("DEVGO_CONTAINER_PREFIX")
		os.Unsetenv("DEVGO_WORKSPACE_MOUNT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DockerHost != "tcp://localhost:2376" {
		t.Errorf("DockerHost = %v, want %v", cfg.DockerHost, "tcp://localhost:2376")
	}

	if cfg.ContainerPrefix != "test-" {
		t.Errorf("ContainerPrefix = %v, want %v", cfg.ContainerPrefix, "test-")
	}

	if cfg.WorkspaceMount != "/test" {
		t.Errorf("WorkspaceMount = %v, want %v", cfg.WorkspaceMount, "/test")
	}
}

func TestGetEnv_WithValue(t *testing.T) {
	key := "TEST_ENV_VAR"
	expectedValue := "test_value"
	os.Setenv(key, expectedValue)
	defer os.Unsetenv(key)

	result := getEnv(key, "default")

	if result != expectedValue {
		t.Errorf("getEnv() = %v, want %v", result, expectedValue)
	}
}

func TestGetEnv_WithoutValue(t *testing.T) {
	key := "NON_EXISTENT_ENV_VAR"
	defaultValue := "default_value"
	os.Unsetenv(key)

	result := getEnv(key, defaultValue)

	if result != defaultValue {
		t.Errorf("getEnv() = %v, want %v", result, defaultValue)
	}
}

func TestGetEnv_WithEmptyValue(t *testing.T) {
	key := "EMPTY_ENV_VAR"
	defaultValue := "default_value"
	os.Setenv(key, "")
	defer os.Unsetenv(key)

	result := getEnv(key, defaultValue)

	if result != defaultValue {
		t.Errorf("getEnv() = %v, want %v", result, defaultValue)
	}
}

func TestLoadUserConfigFile_Missing(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")

	cfg, err := LoadUserConfigFile(path)
	if err != nil {
		t.Fatalf("LoadUserConfigFile() unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatalf("LoadUserConfigFile() returned nil, want empty UserConfig")
	}
	if cfg.Dotfiles != nil {
		t.Errorf("expected Dotfiles to be nil for missing file, got %+v", cfg.Dotfiles)
	}
}

func TestLoadUserConfigFile_ShellField(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte(`{"shell": "zsh"}`), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	cfg, err := LoadUserConfigFile(path)
	if err != nil {
		t.Fatalf("LoadUserConfigFile() error = %v", err)
	}
	if cfg.Shell != "zsh" {
		t.Errorf("Shell = %q, want %q", cfg.Shell, "zsh")
	}
}

func TestLoadUserConfigFile_FullDotfiles(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	contents := `{
  "dotfiles": {
    "repository": "https://github.com/example/dotfiles",
    "targetPath": "/home/user/dotfiles",
    "installCommand": "bootstrap.sh"
  }
}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	cfg, err := LoadUserConfigFile(path)
	if err != nil {
		t.Fatalf("LoadUserConfigFile() error = %v", err)
	}
	if cfg.Dotfiles == nil {
		t.Fatalf("Dotfiles is nil, want populated")
	}
	if cfg.Dotfiles.Repository != "https://github.com/example/dotfiles" {
		t.Errorf("Repository = %q, want %q", cfg.Dotfiles.Repository, "https://github.com/example/dotfiles")
	}
	if cfg.Dotfiles.TargetPath != "/home/user/dotfiles" {
		t.Errorf("TargetPath = %q, want %q", cfg.Dotfiles.TargetPath, "/home/user/dotfiles")
	}
	if cfg.Dotfiles.InstallCommand != "bootstrap.sh" {
		t.Errorf("InstallCommand = %q, want %q", cfg.Dotfiles.InstallCommand, "bootstrap.sh")
	}
}

func TestLoadUserConfigFile_MalformedJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	if _, err := LoadUserConfigFile(path); err == nil {
		t.Fatalf("expected error for malformed JSON, got nil")
	}
}

func TestUserConfigPath_FromXDGEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got, err := UserConfigPath()
	if err != nil {
		t.Fatalf("UserConfigPath() error = %v", err)
	}
	want := filepath.Join(tmp, "devgo", "config.json")
	if got != want {
		t.Errorf("UserConfigPath() = %q, want %q", got, want)
	}
}

func TestUserConfigPath_DefaultsToHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	// Pin HOME so the test does not depend on the runner's environment.
	t.Setenv("HOME", tmp)

	got, err := UserConfigPath()
	if err != nil {
		t.Fatalf("UserConfigPath() error = %v", err)
	}

	want := filepath.Join(tmp, ".config", "devgo", "config.json")
	if got != want {
		t.Errorf("UserConfigPath() = %q, want %q", got, want)
	}
}

func TestProfilePath_FromXDGEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got, err := ProfilePath("go")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}
	want := filepath.Join(tmp, "devgo", "profiles", "go", "devcontainer.json")
	if got != want {
		t.Errorf("ProfilePath() = %q, want %q", got, want)
	}
}

func TestProfilePath_DefaultsToHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", tmp)

	got, err := ProfilePath("rust")
	if err != nil {
		t.Fatalf("ProfilePath() error = %v", err)
	}
	want := filepath.Join(tmp, ".config", "devgo", "profiles", "rust", "devcontainer.json")
	if got != want {
		t.Errorf("ProfilePath() = %q, want %q", got, want)
	}
}

func TestListProfiles_MissingDirIsEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	names, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if len(names) != 0 {
		t.Errorf("ListProfiles() = %v, want empty", names)
	}
}

func TestListProfiles_OnlyDirsWithConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	profilesDir := filepath.Join(tmp, "devgo", "profiles")
	// Two valid profiles (go, rust), one directory without devcontainer.json
	// (empty), and a stray file that must be ignored.
	for _, name := range []string{"rust", "go"} {
		dir := filepath.Join(profilesDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create profile dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "devcontainer.json"), []byte("{}"), 0o600); err != nil {
			t.Fatalf("failed to write profile config: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(profilesDir, "empty"), 0o755); err != nil {
		t.Fatalf("failed to create empty profile dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "stray.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to write stray file: %v", err)
	}

	names, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}

	want := []string{"go", "rust"} // sorted, excludes empty and stray
	if len(names) != len(want) {
		t.Fatalf("ListProfiles() = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("ListProfiles()[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}
