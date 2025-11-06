package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titanous/json5"
)

func TestCreateDefaultTemplate(t *testing.T) {
	template := createDefaultTemplate()

	// Verify the template is not empty
	if template == "" {
		t.Error("template should not be empty")
	}

	// Verify it contains expected strings
	expectedStrings := []string{
		`"name": "development-container"`,
		`"image": "ghcr.io/garaemon/ubuntu-noble:latest"`,
		`"features":`,
		`"customizations":`,
		`"forwardPorts":`,
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(template, expected) {
			t.Errorf("expected template to contain %q", expected)
		}
	}

	// Verify it contains commented fields
	expectedComments := []string{
		"// Container build configuration",
		"// Docker Compose configuration",
		"// Workspace configuration",
		"// User configuration",
		"// Lifecycle commands",
	}

	for _, expected := range expectedComments {
		if !strings.Contains(template, expected) {
			t.Errorf("expected template to contain comment %q", expected)
		}
	}

	// Verify the template is valid JSON5 by attempting to parse (strip comments first for basic validation)
	// Note: This is a basic check - actual JSON5 parsing would require a JSON5 library
	if !strings.HasPrefix(strings.TrimSpace(template), "{") {
		t.Error("template should start with '{'")
	}
	if !strings.HasSuffix(strings.TrimSpace(template), "}") {
		t.Error("template should end with '}'")
	}
}

func TestFindGitRoot(t *testing.T) {
	// Test in the current repository (which should be a git repo)
	gitRoot, err := findGitRoot()

	// This test will pass if we're in a git repo
	if err == nil {
		if gitRoot == "" {
			t.Error("git root should not be empty when no error")
		}
		// Verify it's an absolute path
		if !filepath.IsAbs(gitRoot) {
			t.Errorf("git root should be an absolute path, got %s", gitRoot)
		}
		// Verify .git directory exists
		gitDir := filepath.Join(gitRoot, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			t.Errorf("expected .git directory to exist at %s", gitDir)
		}
	}

	// Test in a non-git directory
	tempDir, err := os.MkdirTemp("", "devgo-test-nogit")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Change to the temp directory
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)

	os.Chdir(tempDir)

	_, err = findGitRoot()
	if err == nil {
		t.Error("expected error when not in a git repository")
	}
}

func TestDetermineInitDirectory(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		setupFunc   func() (string, func())
		expectError bool
		validateFunc func(t *testing.T, result string)
	}{
		{
			name: "explicit directory provided",
			setupFunc: func() (string, func()) {
				tempDir, err := os.MkdirTemp("", "devgo-test-explicit")
				if err != nil {
					t.Fatalf("failed to create temp dir: %v", err)
				}
				return tempDir, func() { os.RemoveAll(tempDir) }
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				if result == "" {
					t.Error("result should not be empty")
				}
			},
		},
		{
			name: "non-existent directory",
			args: []string{"/this/path/should/not/exist/12345"},
			expectError: true,
		},
		{
			name: "file instead of directory",
			setupFunc: func() (string, func()) {
				tempFile, err := os.CreateTemp("", "devgo-test-file")
				if err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}
				tempFile.Close()
				return tempFile.Name(), func() { os.Remove(tempFile.Name()) }
			},
			expectError: true,
		},
		{
			name: "no args - fallback to current directory when not in git repo",
			args: []string{},
			setupFunc: func() (string, func()) {
				// Create a temp directory and change to it
				tempDir, err := os.MkdirTemp("", "devgo-test-nogit")
				if err != nil {
					t.Fatalf("failed to create temp dir: %v", err)
				}
				originalDir, _ := os.Getwd()
				os.Chdir(tempDir)
				return tempDir, func() {
					os.Chdir(originalDir)
					os.RemoveAll(tempDir)
				}
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				// Should return the temp directory we changed to
				if !filepath.IsAbs(result) {
					t.Errorf("expected absolute path, got %s", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.args
			var cleanup func()

			if tt.setupFunc != nil {
				setupResult, cleanupFunc := tt.setupFunc()
				cleanup = cleanupFunc
				if args == nil && setupResult != "" {
					args = []string{setupResult}
				}
			}
			if cleanup != nil {
				defer cleanup()
			}

			result, err := determineInitDirectory(args)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.expectError && tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}

func TestRunInitCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		setupFunc   func() (string, func())
		expectError bool
		errorContains string
	}{
		{
			name: "create devcontainer.json successfully",
			setupFunc: func() (string, func()) {
				tempDir, err := os.MkdirTemp("", "devgo-test-init")
				if err != nil {
					t.Fatalf("failed to create temp dir: %v", err)
				}
				return tempDir, func() { os.RemoveAll(tempDir) }
			},
			expectError: false,
		},
		{
			name: "error when devcontainer.json already exists",
			setupFunc: func() (string, func()) {
				tempDir, err := os.MkdirTemp("", "devgo-test-init-exists")
				if err != nil {
					t.Fatalf("failed to create temp dir: %v", err)
				}
				// Create .devcontainer directory and devcontainer.json
				devcontainerDir := filepath.Join(tempDir, ".devcontainer")
				os.MkdirAll(devcontainerDir, 0755)
				devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
				os.WriteFile(devcontainerPath, []byte("{}"), 0644)

				return tempDir, func() { os.RemoveAll(tempDir) }
			},
			expectError: true,
			errorContains: "already exists",
		},
		{
			name: "error with non-existent directory",
			args: []string{"/this/path/should/not/exist/67890"},
			expectError: true,
			errorContains: "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.args
			var cleanup func()

			if tt.setupFunc != nil {
				setupResult, cleanupFunc := tt.setupFunc()
				cleanup = cleanupFunc
				if args == nil {
					args = []string{setupResult}
				}
			}
			if cleanup != nil {
				defer cleanup()
			}

			err := runInitCommand(args)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.expectError && tt.errorContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %v", tt.errorContains, err)
				}
			}

			// If successful, verify the file was created correctly
			if !tt.expectError && args != nil && len(args) > 0 {
				devcontainerPath := filepath.Join(args[0], ".devcontainer", "devcontainer.json")

				// Check file exists
				if _, err := os.Stat(devcontainerPath); os.IsNotExist(err) {
					t.Errorf("expected devcontainer.json to be created at %s", devcontainerPath)
				}

				// Verify file contents are valid JSON5
				data, err := os.ReadFile(devcontainerPath)
				if err != nil {
					t.Errorf("failed to read created file: %v", err)
				}

				var result map[string]interface{}
				if err := json5.Unmarshal(data, &result); err != nil {
					t.Errorf("created file is not valid JSON5: %v", err)
				}

				// Verify required fields
				if result["image"] != "ghcr.io/garaemon/ubuntu-noble:latest" {
					t.Errorf("expected default image, got %v", result["image"])
				}
				if result["name"] != "development-container" {
					t.Errorf("expected default name, got %v", result["name"])
				}
			}
		})
	}
}

func TestInitCommandIntegration(t *testing.T) {
	// Test the full workflow: init -> verify file -> can't init again
	tempDir, err := os.MkdirTemp("", "devgo-test-integration")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// First init should succeed
	err = runInitCommand([]string{tempDir})
	if err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Verify file exists and is valid
	devcontainerPath := filepath.Join(tempDir, ".devcontainer", "devcontainer.json")
	data, err := os.ReadFile(devcontainerPath)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}

	var config map[string]interface{}
	if err := json5.Unmarshal(data, &config); err != nil {
		t.Fatalf("created file is not valid JSON5: %v", err)
	}

	// Verify the template matches what we expect
	expectedImage := "ghcr.io/garaemon/ubuntu-noble:latest"
	if config["image"] != expectedImage {
		t.Errorf("expected image %q, got %v", expectedImage, config["image"])
	}

	// Second init should fail
	err = runInitCommand([]string{tempDir})
	if err == nil {
		t.Error("expected second init to fail, but it succeeded")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error about file existing, got: %v", err)
	}
}

func TestInitCommandWithVerbose(t *testing.T) {
	// Test that verbose flag works (doesn't cause errors)
	tempDir, err := os.MkdirTemp("", "devgo-test-verbose")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore verbose flag
	originalVerbose := verbose
	defer func() { verbose = originalVerbose }()

	verbose = true
	err = runInitCommand([]string{tempDir})
	if err != nil {
		t.Errorf("init with verbose flag failed: %v", err)
	}

	// Verify file was created
	devcontainerPath := filepath.Join(tempDir, ".devcontainer", "devcontainer.json")
	if _, err := os.Stat(devcontainerPath); os.IsNotExist(err) {
		t.Error("expected devcontainer.json to be created with verbose flag")
	}
}

func TestInitCommandInGitRepo(t *testing.T) {
	// Only run this test if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping test")
	}

	// Create a temp directory and initialize it as a git repo
	tempDir, err := os.MkdirTemp("", "devgo-test-gitrepo")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to initialize git repo: %v", err)
	}

	// Configure git user (required for commits, though we won't commit)
	exec.Command("git", "config", "user.email", "test@example.com").Dir = tempDir
	exec.Command("git", "config", "user.name", "Test User").Dir = tempDir

	// Change to the git repo directory
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Run init without args (should use git root)
	err = runInitCommand([]string{})
	if err != nil {
		t.Errorf("init in git repo failed: %v", err)
	}

	// Verify file was created in git root
	devcontainerPath := filepath.Join(tempDir, ".devcontainer", "devcontainer.json")
	if _, err := os.Stat(devcontainerPath); os.IsNotExist(err) {
		t.Error("expected devcontainer.json to be created in git root")
	}
}
