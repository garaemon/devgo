package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/garaemon/devgo/pkg/devcontainer"
	"github.com/opencontainers/image-spec/specs-go/v1"
)

// Note: DockerClient interface and DockerRunArgs are defined in up.go

// mockDockerClient implements DockerClient for testing
type mockDockerClient struct {
	containers        map[string]bool // name -> isRunning
	createError       error
	startError        error
	existsError       error
	isRunningError    error
	createdContainers []DockerRunArgs
}

func newMockDockerClient() *mockDockerClient {
	return &mockDockerClient{
		containers:        make(map[string]bool),
		createdContainers: make([]DockerRunArgs, 0),
	}
}

func (m *mockDockerClient) ContainerExists(ctx context.Context, name string) (bool, error) {
	if m.existsError != nil {
		return false, m.existsError
	}
	_, exists := m.containers[name]
	return exists, nil
}

func (m *mockDockerClient) IsContainerRunning(ctx context.Context, name string) (bool, error) {
	if m.isRunningError != nil {
		return false, m.isRunningError
	}
	return m.containers[name], nil
}

func (m *mockDockerClient) StartExistingContainer(ctx context.Context, name string) error {
	if m.startError != nil {
		return m.startError
	}
	if _, exists := m.containers[name]; !exists {
		return fmt.Errorf("container %s does not exist", name)
	}
	m.containers[name] = true
	return nil
}

func (m *mockDockerClient) CreateAndStartContainer(ctx context.Context, args DockerRunArgs) error {
	if m.createError != nil {
		return m.createError
	}
	m.containers[args.Name] = true
	m.createdContainers = append(m.createdContainers, args)
	return nil
}

func (m *mockDockerClient) Close() error {
	return nil
}

// Helper methods for test setup
func (m *mockDockerClient) addContainer(name string, isRunning bool) {
	m.containers[name] = isRunning
}

func (m *mockDockerClient) setCreateError(err error) {
	m.createError = err
}

func (m *mockDockerClient) setStartError(err error) {
	m.startError = err
}

func TestRunUpCommand(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*mockDockerClient)
		expectError    bool
		expectedOutput string
	}{
		{
			name: "container already running",
			setupMock: func(m *mockDockerClient) {
				m.addContainer("test-container", true)
			},
			expectError:    true,
			expectedOutput: "container 'test-container' is already running",
		},
		{
			name: "container exists but stopped",
			setupMock: func(m *mockDockerClient) {
				m.addContainer("test-container", false)
			},
			expectError:    false,
			expectedOutput: "",
		},
		{
			name: "new container creation",
			setupMock: func(m *mockDockerClient) {
				// No existing container
			},
			expectError:    false,
			expectedOutput: "",
		},
		{
			name: "start existing container fails",
			setupMock: func(m *mockDockerClient) {
				m.addContainer("test-container", false)
				m.setStartError(errors.New("start failed"))
			},
			expectError:    true,
			expectedOutput: "start failed",
		},
		{
			name: "create new container fails",
			setupMock: func(m *mockDockerClient) {
				m.setCreateError(errors.New("create failed"))
			},
			expectError:    true,
			expectedOutput: "create failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockDocker := newMockDockerClient()
			tt.setupMock(mockDocker)

			// Create test devcontainer
			devContainer := &devcontainer.DevContainer{
				Name:            "test-container",
				Image:           "ubuntu:22.04",
				WorkspaceFolder: "/workspace",
			}

			// Test the function
			ctx := context.Background()
			err := startContainerWithDocker(ctx, devContainer, "test-container", "/test/workspace", mockDocker)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.expectedOutput != "" && err.Error() != tt.expectedOutput {
					t.Errorf("expected error message '%s' but got '%s'", tt.expectedOutput, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestDetermineWorkspaceFolder(t *testing.T) {
	tests := []struct {
		name           string
		workspaceFlag  string
		expectedResult string
	}{
		{
			name:           "workspace folder flag provided",
			workspaceFlag:  "/custom/workspace",
			expectedResult: "/custom/workspace",
		},
		{
			name:           "no workspace folder flag",
			workspaceFlag:  "",
			expectedResult: "", // Will be current directory in real implementation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalWorkspaceFolder := workspaceFolder
			defer func() { workspaceFolder = originalWorkspaceFolder }()

			// Set test value
			workspaceFolder = tt.workspaceFlag

			result := determineWorkspaceFolder()
			if tt.workspaceFlag != "" && result != tt.expectedResult {
				t.Errorf("expected %s but got %s", tt.expectedResult, result)
			}
		})
	}
}

func TestDetermineContainerName(t *testing.T) {
	tests := []struct {
		name             string
		containerNameFlag string
		devContainerName  string
		workspaceDir     string
		expectedResult   string
	}{
		{
			name:             "container name flag provided",
			containerNameFlag: "custom-name",
			devContainerName:  "devcontainer-name",
			workspaceDir:     "/path/to/workspace",
			expectedResult:   "custom-name",
		},
		{
			name:             "devcontainer name provided",
			containerNameFlag: "",
			devContainerName:  "devcontainer-name",
			workspaceDir:     "/path/to/workspace",
			expectedResult:   "devcontainer-name",
		},
		{
			name:             "default name from workspace",
			containerNameFlag: "",
			devContainerName:  "",
			workspaceDir:     "/path/to/workspace",
			expectedResult:   "devgo-workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalContainerName := containerName
			defer func() { containerName = originalContainerName }()

			// Set test value
			containerName = tt.containerNameFlag

			devContainer := &devcontainer.DevContainer{
				Name: tt.devContainerName,
			}

			result := determineContainerName(devContainer, tt.workspaceDir)
			if result != tt.expectedResult {
				t.Errorf("expected %s but got %s", tt.expectedResult, result)
			}
		})
	}
}

// mockDockerAPIClient implements the dockerAPIClient interface for testing
type mockDockerAPIClient struct {
	containers    []container.Summary
	listError     error
}

func (m *mockDockerAPIClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	if m.listError != nil {
		return nil, m.listError
	}
	return m.containers, nil
}

func (m *mockDockerAPIClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return nil
}

func (m *mockDockerAPIClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
	return container.CreateResponse{}, nil
}

func (m *mockDockerAPIClient) Close() error {
	return nil
}

func TestRealDockerClientContainerExists(t *testing.T) {
	tests := []struct {
		name           string
		containerName  string
		setupMock      func(*mockDockerAPIClient)
		expectedResult bool
		expectError    bool
	}{
		{
			name:          "container exists with exact name match",
			containerName: "test-container",
			setupMock: func(m *mockDockerAPIClient) {
				m.containers = []container.Summary{
					{
						Names: []string{"/test-container"},
					},
				}
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "container exists with multiple names",
			containerName: "test-container",
			setupMock: func(m *mockDockerAPIClient) {
				m.containers = []container.Summary{
					{
						Names: []string{"/other-name", "/test-container"},
					},
				}
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "container does not exist",
			containerName: "non-existent-container",
			setupMock: func(m *mockDockerAPIClient) {
				m.containers = []container.Summary{
					{
						Names: []string{"/other-container"},
					},
				}
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name:          "no containers exist",
			containerName: "test-container",
			setupMock: func(m *mockDockerAPIClient) {
				m.containers = []container.Summary{}
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name:          "empty container name",
			containerName: "",
			setupMock: func(m *mockDockerAPIClient) {
				m.containers = []container.Summary{
					{
						Names: []string{"/test-container"},
					},
				}
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name:          "docker api error",
			containerName: "test-container",
			setupMock: func(m *mockDockerAPIClient) {
				m.listError = fmt.Errorf("docker daemon not available")
			},
			expectedResult: false,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockDockerAPIClient{}
			tt.setupMock(mockAPI)

			// Create a factory that returns our mock API client
			factory := func() (dockerAPIClient, error) {
				return mockAPI, nil
			}

			// Create realDockerClient with our mock factory
			dockerClient, err := newRealDockerClientWithFactory(factory)
			if err != nil {
				t.Fatalf("failed to create docker client: %v", err)
			}
			defer dockerClient.Close()

			// Actually call the ContainerExists method
			ctx := context.Background()
			result, err := dockerClient.ContainerExists(ctx, tt.containerName)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expectedResult {
				t.Errorf("expected %v but got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestRealDockerClientIsContainerRunning(t *testing.T) {
	tests := []struct {
		name           string
		containerName  string
		setupMock      func(*mockDockerAPIClient)
		expectedResult bool
		expectError    bool
	}{
		{
			name:          "container is running",
			containerName: "test-container",
			setupMock: func(m *mockDockerAPIClient) {
				m.containers = []container.Summary{
					{
						Names:  []string{"/test-container"},
						State:  "running",
						Status: "Up 5 minutes",
					},
				}
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "container exists but not running",
			containerName: "test-container",
			setupMock: func(m *mockDockerAPIClient) {
				// No containers returned because status filter is "running"
				m.containers = []container.Summary{}
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name:          "container with multiple names is running",
			containerName: "test-container",
			setupMock: func(m *mockDockerAPIClient) {
				m.containers = []container.Summary{
					{
						Names:  []string{"/other-name", "/test-container"},
						State:  "running",
						Status: "Up 10 minutes",
					},
				}
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "no containers exist",
			containerName: "test-container",
			setupMock: func(m *mockDockerAPIClient) {
				m.containers = []container.Summary{}
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name:          "empty container name",
			containerName: "",
			setupMock: func(m *mockDockerAPIClient) {
				m.containers = []container.Summary{
					{
						Names:  []string{"/test-container"},
						State:  "running",
						Status: "Up 1 hour",
					},
				}
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name:          "different container running",
			containerName: "target-container",
			setupMock: func(m *mockDockerAPIClient) {
				m.containers = []container.Summary{
					{
						Names:  []string{"/other-container"},
						State:  "running",
						Status: "Up 30 seconds",
					},
				}
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name:          "docker api error",
			containerName: "test-container",
			setupMock: func(m *mockDockerAPIClient) {
				m.listError = fmt.Errorf("docker daemon not available")
			},
			expectedResult: false,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockDockerAPIClient{}
			tt.setupMock(mockAPI)

			// Create a factory that returns our mock API client
			factory := func() (dockerAPIClient, error) {
				return mockAPI, nil
			}

			// Create realDockerClient with our mock factory
			dockerClient, err := newRealDockerClientWithFactory(factory)
			if err != nil {
				t.Fatalf("failed to create docker client: %v", err)
			}
			defer dockerClient.Close()

			// Actually call the IsContainerRunning method
			ctx := context.Background()
			result, err := dockerClient.IsContainerRunning(ctx, tt.containerName)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expectedResult {
				t.Errorf("expected %v but got %v", tt.expectedResult, result)
			}
		})
	}
}