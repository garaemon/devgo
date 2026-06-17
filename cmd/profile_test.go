package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetProfileGlobals clears the flag-backed globals that profile resolution
// reads, restoring them after the test.
func resetProfileGlobals(t *testing.T) {
	t.Helper()
	origProfile := profileName
	origConfig := configPath
	origWorkspace := workspaceFolder
	t.Cleanup(func() {
		profileName = origProfile
		configPath = origConfig
		workspaceFolder = origWorkspace
	})
	profileName = ""
	configPath = ""
	workspaceFolder = ""
	t.Setenv("DEVGO_PROFILE", "")
}

func TestParseAllFlags_Profile(t *testing.T) {
	resetProfileGlobals(t)

	if _, err := parseAllFlags([]string{"--profile", "go", "up"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profileName != "go" {
		t.Errorf("profileName = %q, want %q", profileName, "go")
	}
}

func TestResolveProfileName_FlagWinsOverEnv(t *testing.T) {
	resetProfileGlobals(t)
	t.Setenv("DEVGO_PROFILE", "rust")

	profileName = "go"
	if got := resolveProfileName(); got != "go" {
		t.Errorf("resolveProfileName() = %q, want %q (flag should win)", got, "go")
	}

	profileName = ""
	if got := resolveProfileName(); got != "rust" {
		t.Errorf("resolveProfileName() = %q, want %q (env fallback)", got, "rust")
	}
}

func TestFindDevcontainerConfig_Profile(t *testing.T) {
	resetProfileGlobals(t)
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Create a profile on disk.
	profileDir := filepath.Join(tmp, "devgo", "profiles", "go")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("failed to create profile dir: %v", err)
	}
	want := filepath.Join(profileDir, "devcontainer.json")
	if err := os.WriteFile(want, []byte(`{"image":"alpine"}`), 0o600); err != nil {
		t.Fatalf("failed to write profile config: %v", err)
	}

	profileName = "go"
	got, err := findDevcontainerConfig(configPath)
	if err != nil {
		t.Fatalf("findDevcontainerConfig() error = %v", err)
	}
	if got != want {
		t.Errorf("findDevcontainerConfig() = %q, want %q", got, want)
	}
}

func TestFindDevcontainerConfig_ConfigBeatsProfile(t *testing.T) {
	resetProfileGlobals(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	profileName = "go"
	configPath = "/explicit/devcontainer.json"

	got, err := findDevcontainerConfig(configPath)
	if err != nil {
		t.Fatalf("findDevcontainerConfig() error = %v", err)
	}
	if got != "/explicit/devcontainer.json" {
		t.Errorf("findDevcontainerConfig() = %q, want explicit config path", got)
	}
}

func TestFindDevcontainerConfig_MissingProfileError(t *testing.T) {
	resetProfileGlobals(t)
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// One existing profile so the error lists available profiles.
	existing := filepath.Join(tmp, "devgo", "profiles", "rust")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("failed to create profile dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existing, "devcontainer.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to write profile config: %v", err)
	}

	profileName = "go"
	_, err := findDevcontainerConfig(configPath)
	if err == nil {
		t.Fatalf("expected error for missing profile, got nil")
	}
	if !strings.Contains(err.Error(), "rust") {
		t.Errorf("error should list available profile 'rust', got: %v", err)
	}
}

func TestRunInitProfile_ReplacesNameWithProfileName(t *testing.T) {
	resetProfileGlobals(t)
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := runInitProfile("go"); err != nil {
		t.Fatalf("runInitProfile() error = %v", err)
	}

	generated := filepath.Join(tmp, "devgo", "profiles", "go", "devcontainer.json")
	content, err := os.ReadFile(generated)
	if err != nil {
		t.Fatalf("failed to read generated profile: %v", err)
	}

	if !strings.Contains(string(content), `"name": "go"`) {
		t.Errorf("generated profile should set name to %q, got:\n%s", "go", content)
	}
	if strings.Contains(string(content), `"name": "development-container"`) {
		t.Errorf("generated profile still contains default name, got:\n%s", content)
	}
}

func TestRunInitProfile_ErrorsWhenProfileExists(t *testing.T) {
	resetProfileGlobals(t)
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := runInitProfile("go"); err != nil {
		t.Fatalf("first runInitProfile() error = %v", err)
	}

	err := runInitProfile("go")
	if err == nil {
		t.Fatalf("expected error when profile already exists, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}
}

func TestRunInitProfile_RejectsTraversalName(t *testing.T) {
	resetProfileGlobals(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := runInitProfile("../escape")
	if err == nil {
		t.Fatalf("expected error for traversal profile name, got nil")
	}
}

func TestDetermineWorkspaceFolder_ProfileUsesCwd(t *testing.T) {
	resetProfileGlobals(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	profileName = "go"
	// The devcontainer path lives under the home directory; the workspace must
	// still resolve to the current directory, not the profile's location.
	got := determineWorkspaceFolder("/home/user/.config/devgo/profiles/go/devcontainer.json")
	if got != cwd {
		t.Errorf("determineWorkspaceFolder() = %q, want cwd %q", got, cwd)
	}
}

func TestDetermineWorkspaceFolder_ProfileWorkspaceFolderOverride(t *testing.T) {
	resetProfileGlobals(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	profileName = "go"
	workspaceFolder = "/tmp/explicit"
	got := determineWorkspaceFolder("/home/user/.config/devgo/profiles/go/devcontainer.json")
	if got != "/tmp/explicit" {
		t.Errorf("determineWorkspaceFolder() = %q, want %q", got, "/tmp/explicit")
	}
}
