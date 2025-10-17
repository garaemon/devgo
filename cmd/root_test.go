package cmd

import (
	"testing"
)

func TestParseAllFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
		errorMsg    string
		expectedLen int
	}{
		{
			name:        "valid flags",
			args:        []string{"--help", "--version"},
			expectError: false,
			expectedLen: 0,
		},
		{
			name:        "valid flags with values",
			args:        []string{"--workspace-folder", "/path", "up"},
			expectError: false,
			expectedLen: 1,
		},
		{
			name:        "unknown flag",
			args:        []string{"--unknown-flag"},
			expectError: true,
			errorMsg:    "unknown option: --unknown-flag",
		},
		{
			name:        "unknown flag with other valid flags",
			args:        []string{"--verbose", "--unknown-option", "up"},
			expectError: true,
			errorMsg:    "unknown option: --unknown-option",
		},
		{
			name:        "command without flags",
			args:        []string{"up"},
			expectError: false,
			expectedLen: 1,
		},
		{
			name:        "all valid flags",
			args:        []string{"--config", "test.json", "--force-build", "--push", "--pull"},
			expectError: false,
			expectedLen: 0,
		},
		{
			name:        "session flag",
			args:        []string{"--session", "test", "up"},
			expectError: false,
			expectedLen: 1,
		},
		{
			name:        "name and image-name flags",
			args:        []string{"--name", "container", "--image-name", "image:tag"},
			expectError: false,
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global flags before each test
			showHelp = false
			showVersion = false
			verbose = false
			workspaceFolder = ""
			configPath = ""
			containerName = ""
			imageName = ""
			sessionName = ""
			forceBuild = false
			push = false
			pull = false

			result, err := parseAllFlags(tt.args)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("expected error message %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if len(result) != tt.expectedLen {
					t.Errorf("expected %d non-flag args, got %d: %v", tt.expectedLen, len(result), result)
				}
			}
		})
	}
}

func TestParseAllFlags_FlagValues(t *testing.T) {
	tests := []struct {
		name                  string
		args                  []string
		expectWorkspaceFolder string
		expectConfigPath      string
		expectContainerName   string
		expectImageName       string
		expectSessionName     string
		expectForceBuild      bool
		expectPush            bool
		expectPull            bool
		expectVerbose         bool
		expectHelp            bool
		expectVersion         bool
	}{
		{
			name:                  "workspace-folder flag",
			args:                  []string{"--workspace-folder", "/test/path"},
			expectWorkspaceFolder: "/test/path",
		},
		{
			name:             "config flag",
			args:             []string{"--config", "/test/config.json"},
			expectConfigPath: "/test/config.json",
		},
		{
			name:                "name flag",
			args:                []string{"--name", "my-container"},
			expectContainerName: "my-container",
		},
		{
			name:            "image-name flag",
			args:            []string{"--image-name", "my-image:v1.0"},
			expectImageName: "my-image:v1.0",
		},
		{
			name:              "session flag",
			args:              []string{"--session", "dev"},
			expectSessionName: "dev",
		},
		{
			name:             "force-build flag",
			args:             []string{"--force-build"},
			expectForceBuild: true,
		},
		{
			name:       "push flag",
			args:       []string{"--push"},
			expectPush: true,
		},
		{
			name:       "pull flag",
			args:       []string{"--pull"},
			expectPull: true,
		},
		{
			name:          "verbose flag",
			args:          []string{"--verbose"},
			expectVerbose: true,
		},
		{
			name:       "help flag",
			args:       []string{"--help"},
			expectHelp: true,
		},
		{
			name:          "version flag",
			args:          []string{"--version"},
			expectVersion: true,
		},
		{
			name: "multiple flags",
			args: []string{
				"--workspace-folder", "/test",
				"--config", "config.json",
				"--force-build",
				"--verbose",
			},
			expectWorkspaceFolder: "/test",
			expectConfigPath:      "config.json",
			expectForceBuild:      true,
			expectVerbose:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global flags before each test
			showHelp = false
			showVersion = false
			verbose = false
			workspaceFolder = ""
			configPath = ""
			containerName = ""
			imageName = ""
			sessionName = ""
			forceBuild = false
			push = false
			pull = false

			_, err := parseAllFlags(tt.args)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if workspaceFolder != tt.expectWorkspaceFolder {
				t.Errorf("workspaceFolder = %q, want %q", workspaceFolder, tt.expectWorkspaceFolder)
			}
			if configPath != tt.expectConfigPath {
				t.Errorf("configPath = %q, want %q", configPath, tt.expectConfigPath)
			}
			if containerName != tt.expectContainerName {
				t.Errorf("containerName = %q, want %q", containerName, tt.expectContainerName)
			}
			if imageName != tt.expectImageName {
				t.Errorf("imageName = %q, want %q", imageName, tt.expectImageName)
			}
			if sessionName != tt.expectSessionName {
				t.Errorf("sessionName = %q, want %q", sessionName, tt.expectSessionName)
			}
			if forceBuild != tt.expectForceBuild {
				t.Errorf("forceBuild = %v, want %v", forceBuild, tt.expectForceBuild)
			}
			if push != tt.expectPush {
				t.Errorf("push = %v, want %v", push, tt.expectPush)
			}
			if pull != tt.expectPull {
				t.Errorf("pull = %v, want %v", pull, tt.expectPull)
			}
			if verbose != tt.expectVerbose {
				t.Errorf("verbose = %v, want %v", verbose, tt.expectVerbose)
			}
			if showHelp != tt.expectHelp {
				t.Errorf("showHelp = %v, want %v", showHelp, tt.expectHelp)
			}
			if showVersion != tt.expectVersion {
				t.Errorf("showVersion = %v, want %v", showVersion, tt.expectVersion)
			}
		})
	}
}
