package config

import (
	"os"
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