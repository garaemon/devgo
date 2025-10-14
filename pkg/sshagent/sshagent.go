package sshagent

import (
	"fmt"
	"os"

	"github.com/garaemon/devgo/pkg/devcontainer"
)

const (
	sshAuthSockEnv        = "SSH_AUTH_SOCK"
	containerSSHAuthSock  = "/ssh-agent"
)

// GetHostSocket returns the SSH agent socket path from the host environment
func GetHostSocket() (string, error) {
	socketPath := os.Getenv(sshAuthSockEnv)
	if socketPath == "" {
		return "", fmt.Errorf("SSH_AUTH_SOCK environment variable is not set")
	}

	// Check if the socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return "", fmt.Errorf("SSH agent socket does not exist at %s", socketPath)
	}

	return socketPath, nil
}

// CreateMount creates a mount configuration for SSH agent forwarding
func CreateMount(hostSocketPath string) devcontainer.Mount {
	return devcontainer.Mount{
		Type:   "bind",
		Source: hostSocketPath,
		Target: containerSSHAuthSock,
	}
}

// GetContainerEnv returns the environment variable for the container
func GetContainerEnv() map[string]string {
	return map[string]string{
		sshAuthSockEnv: containerSSHAuthSock,
	}
}

// IsAvailable checks if SSH agent is available on the host
func IsAvailable() bool {
	_, err := GetHostSocket()
	return err == nil
}
