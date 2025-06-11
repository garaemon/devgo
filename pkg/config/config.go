package config

import (
	"os"
)

type Config struct {
	DockerHost      string
	ContainerPrefix string
	WorkspaceMount  string
}

func Load() (*Config, error) {
	cfg := &Config{
		DockerHost:      getEnv("DOCKER_HOST", ""),
		ContainerPrefix: getEnv("DEVGO_CONTAINER_PREFIX", "devgo-"),
		WorkspaceMount:  getEnv("DEVGO_WORKSPACE_MOUNT", "/workspace"),
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
