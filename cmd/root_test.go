package cmd

import (
	"bytes"
	"os"
	"strings"
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
			debug = false
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
		expectDebug           bool
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
			name:        "debug flag",
			args:        []string{"--debug"},
			expectDebug: true,
		},
		{
			name:        "verbose alias still enables debug",
			args:        []string{"--verbose"},
			expectDebug: true,
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
				"--debug",
			},
			expectWorkspaceFolder: "/test",
			expectConfigPath:      "config.json",
			expectForceBuild:      true,
			expectDebug:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global flags before each test
			showHelp = false
			showVersion = false
			debug = false
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
			if debug != tt.expectDebug {
				t.Errorf("debug = %v, want %v", debug, tt.expectDebug)
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

// resetPersonalizationFlags clears the package-level globals introduced by
// this PR (dotfiles + shell override). Older flag tests in this file reset
// only the older globals; this helper covers the per-user / personal flags
// added together.
func resetPersonalizationFlags() {
	dotfilesRepository = ""
	dotfilesTargetPath = ""
	dotfilesInstallCommand = ""
	noDotfiles = false
	forceDotfiles = false
	shellOverride = ""
}

func TestParseAllFlags_ShellFlag(t *testing.T) {
	resetPersonalizationFlags()
	if _, err := parseAllFlags([]string{"--shell", "/usr/bin/zsh"}); err != nil {
		t.Fatalf("parseAllFlags error = %v", err)
	}
	if shellOverride != "/usr/bin/zsh" {
		t.Errorf("shellOverride = %q, want %q", shellOverride, "/usr/bin/zsh")
	}
}

func TestParseAllFlags_EnvFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "single --env",
			args: []string{"shell", "--env", "FOO=bar"},
			want: []string{"FOO=bar"},
		},
		{
			name: "repeated --env and -e",
			args: []string{"shell", "--env", "FOO=bar", "-e", "BAZ=qux", "-e", "PATH"},
			want: []string{"FOO=bar", "BAZ=qux", "PATH"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shellEnvVars = nil
			defer func() { shellEnvVars = nil }()

			if _, err := parseAllFlags(tt.args); err != nil {
				t.Fatalf("parseAllFlags error = %v", err)
			}
			if len(shellEnvVars) != len(tt.want) {
				t.Fatalf("shellEnvVars = %v, want %v", shellEnvVars, tt.want)
			}
			for i, v := range tt.want {
				if shellEnvVars[i] != v {
					t.Errorf("shellEnvVars[%d] = %q, want %q", i, shellEnvVars[i], v)
				}
			}
		})
	}
}

func TestParseAllFlags_DotfilesFlags(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantRepository string
		wantTargetPath string
		wantInstallCmd string
		wantNoDotfile  bool
		wantForce      bool
	}{
		{
			name:           "dotfiles-repository",
			args:           []string{"--dotfiles-repository", "https://example.com/df"},
			wantRepository: "https://example.com/df",
		},
		{
			name:           "dotfiles-target-path",
			args:           []string{"--dotfiles-target-path", "/home/u/df"},
			wantTargetPath: "/home/u/df",
		},
		{
			name:           "dotfiles-install-command",
			args:           []string{"--dotfiles-install-command", "bootstrap.sh"},
			wantInstallCmd: "bootstrap.sh",
		},
		{
			name:          "no-dotfiles",
			args:          []string{"--no-dotfiles"},
			wantNoDotfile: true,
		},
		{
			name:      "force-dotfiles",
			args:      []string{"--force-dotfiles"},
			wantForce: true,
		},
		{
			name: "all dotfiles flags together",
			args: []string{
				"--dotfiles-repository", "https://example.com/df",
				"--dotfiles-target-path", "/df",
				"--dotfiles-install-command", "setup.sh",
				"--force-dotfiles",
			},
			wantRepository: "https://example.com/df",
			wantTargetPath: "/df",
			wantInstallCmd: "setup.sh",
			wantForce:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetPersonalizationFlags()
			if _, err := parseAllFlags(tt.args); err != nil {
				t.Fatalf("parseAllFlags error = %v", err)
			}
			if dotfilesRepository != tt.wantRepository {
				t.Errorf("dotfilesRepository = %q, want %q", dotfilesRepository, tt.wantRepository)
			}
			if dotfilesTargetPath != tt.wantTargetPath {
				t.Errorf("dotfilesTargetPath = %q, want %q", dotfilesTargetPath, tt.wantTargetPath)
			}
			if dotfilesInstallCommand != tt.wantInstallCmd {
				t.Errorf("dotfilesInstallCommand = %q, want %q", dotfilesInstallCommand, tt.wantInstallCmd)
			}
			if noDotfiles != tt.wantNoDotfile {
				t.Errorf("noDotfiles = %v, want %v", noDotfiles, tt.wantNoDotfile)
			}
			if forceDotfiles != tt.wantForce {
				t.Errorf("forceDotfiles = %v, want %v", forceDotfiles, tt.wantForce)
			}
		})
	}
}

func TestExecute_UnknownOption(t *testing.T) {
	// Save original os.Args and restore after test
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Set os.Args to include an unknown option
	os.Args = []string{"devgo", "--unknown-option"}

	// Execute should return an error
	err := Execute()
	if err == nil {
		t.Error("expected error but got none")
	}

	// Check error message
	expectedError := "unknown option: --unknown-option"
	if err.Error() != expectedError {
		t.Errorf("expected error %q, got %q", expectedError, err.Error())
	}

	// Close writer and read stderr
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = oldStderr

	stderrOutput := buf.String()

	// Verify that error message and usage were printed to stderr
	if !strings.Contains(stderrOutput, "Error: unknown option: --unknown-option") {
		t.Errorf("stderr should contain error message, got: %s", stderrOutput)
	}
	if !strings.Contains(stderrOutput, "devgo - Run commands in a devcontainer") {
		t.Errorf("stderr should contain usage help, got: %s", stderrOutput)
	}
}

func TestExecute_Help(t *testing.T) {
	// Save original os.Args and restore after test
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Set os.Args to use --help
	os.Args = []string{"devgo", "--help"}

	// Execute should not return an error
	err := Execute()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Close writer and read stderr
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = oldStderr

	stderrOutput := buf.String()

	// Verify that usage was printed to stderr
	if !strings.Contains(stderrOutput, "devgo - Run commands in a devcontainer") {
		t.Errorf("stderr should contain usage help, got: %s", stderrOutput)
	}
}
