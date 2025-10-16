package sshagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetHostSocket(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		createFile  bool
		expectError bool
	}{
		{
			name:        "SSH_AUTH_SOCK not set",
			envValue:    "",
			createFile:  false,
			expectError: true,
		},
		{
			name:        "SSH_AUTH_SOCK set but file does not exist",
			envValue:    "/tmp/nonexistent-socket",
			createFile:  false,
			expectError: true,
		},
		{
			name:        "SSH_AUTH_SOCK set and file exists",
			envValue:    "",
			createFile:  true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment
			originalValue := os.Getenv("SSH_AUTH_SOCK")
			defer os.Setenv("SSH_AUTH_SOCK", originalValue)

			if tt.createFile {
				// Create a temporary file to simulate socket
				tmpDir := t.TempDir()
				socketPath := filepath.Join(tmpDir, "agent.sock")
				f, err := os.Create(socketPath)
				if err != nil {
					t.Fatalf("Failed to create temp socket file: %v", err)
				}
				f.Close()
				os.Setenv("SSH_AUTH_SOCK", socketPath)
			} else {
				os.Setenv("SSH_AUTH_SOCK", tt.envValue)
			}

			socketPath, err := GetHostSocket()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if socketPath == "" {
					t.Errorf("Expected socket path but got empty string")
				}
			}
		})
	}
}

func TestCreateMount(t *testing.T) {
	hostPath := "/tmp/ssh-agent.sock"
	mount := CreateMount(hostPath)

	if mount.Type != "bind" {
		t.Errorf("Expected mount type 'bind', got '%s'", mount.Type)
	}
	if mount.Source != hostPath {
		t.Errorf("Expected source '%s', got '%s'", hostPath, mount.Source)
	}
	if mount.Target != containerSSHAuthSock {
		t.Errorf("Expected target '%s', got '%s'", containerSSHAuthSock, mount.Target)
	}
}

func TestGetContainerEnv(t *testing.T) {
	env := GetContainerEnv()

	if len(env) != 1 {
		t.Errorf("Expected 1 environment variable, got %d", len(env))
	}

	if env[sshAuthSockEnv] != containerSSHAuthSock {
		t.Errorf("Expected %s=%s, got %s=%s",
			sshAuthSockEnv, containerSSHAuthSock,
			sshAuthSockEnv, env[sshAuthSockEnv])
	}
}

func TestIsAvailable(t *testing.T) {
	tests := []struct {
		name       string
		envValue   string
		createFile bool
		expected   bool
	}{
		{
			name:       "SSH agent not available",
			envValue:   "",
			createFile: false,
			expected:   false,
		},
		{
			name:       "SSH agent available",
			envValue:   "",
			createFile: true,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment
			originalValue := os.Getenv("SSH_AUTH_SOCK")
			defer os.Setenv("SSH_AUTH_SOCK", originalValue)

			if tt.createFile {
				tmpDir := t.TempDir()
				socketPath := filepath.Join(tmpDir, "agent.sock")
				f, err := os.Create(socketPath)
				if err != nil {
					t.Fatalf("Failed to create temp socket file: %v", err)
				}
				f.Close()
				os.Setenv("SSH_AUTH_SOCK", socketPath)
			} else {
				os.Setenv("SSH_AUTH_SOCK", tt.envValue)
			}

			available := IsAvailable()
			if available != tt.expected {
				t.Errorf("Expected IsAvailable() to return %v, got %v", tt.expected, available)
			}
		})
	}
}
