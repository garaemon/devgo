package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/garaemon/devgo/pkg/config"
	"github.com/garaemon/devgo/pkg/constants"
	"github.com/garaemon/devgo/pkg/devcontainer"
)

func TestExecuteInteractiveShell(t *testing.T) {
	tests := []struct {
		name             string
		containerName    string
		devContainer     *devcontainer.DevContainer
		containers       []container.Summary
		execCreateResp   container.ExecCreateResponse
		execCreateError  error
		execAttachError  error
		inspectResponse  types.ContainerJSON
		expectError      bool
		expectedErrorMsg string
	}{
		{
			name:          "successful shell execution",
			containerName: "test-container",
			devContainer: &devcontainer.DevContainer{
				ContainerUser:   "root",
				WorkspaceFolder: "/workspace",
			},
			containers: []container.Summary{
				{
					ID:    "abc123",
					Names: []string{"/test-container"},
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
			},
			inspectResponse: types.ContainerJSON{
				Config: &container.Config{
					Env: []string{"PATH=/usr/bin"},
				},
			},
			execCreateResp: container.ExecCreateResponse{
				ID: "exec123",
			},
		},
		{
			name:          "container not running",
			containerName: "missing-container",
			devContainer: &devcontainer.DevContainer{
				ContainerUser:   "root",
				WorkspaceFolder: "/workspace",
			},
			containers:       []container.Summary{},
			expectError:      true,
			expectedErrorMsg: "is not running",
		},
		{
			name:          "exec create error",
			containerName: "test-container",
			devContainer: &devcontainer.DevContainer{
				ContainerUser:   "root",
				WorkspaceFolder: "/workspace",
			},
			containers: []container.Summary{
				{
					ID:    "abc123",
					Names: []string{"/test-container"},
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
			},
			inspectResponse: types.ContainerJSON{
				Config: &container.Config{
					Env: []string{"PATH=/usr/bin"},
				},
			},
			execCreateError:  fmt.Errorf("failed to create exec"),
			expectError:      true,
			expectedErrorMsg: "failed to create exec instance",
		},
		{
			name:          "exec attach error",
			containerName: "test-container",
			devContainer: &devcontainer.DevContainer{
				ContainerUser:   "root",
				WorkspaceFolder: "/workspace",
			},
			containers: []container.Summary{
				{
					ID:    "abc123",
					Names: []string{"/test-container"},
					Labels: map[string]string{
						constants.DevgoManagedLabel: constants.DevgoManagedValue,
					},
				},
			},
			execCreateResp: container.ExecCreateResponse{
				ID: "exec123",
			},
			inspectResponse: types.ContainerJSON{
				Config: &container.Config{
					Env: []string{"PATH=/usr/bin"},
				},
			},
			execAttachError:  fmt.Errorf("failed to attach"),
			expectError:      true,
			expectedErrorMsg: "failed to attach to exec instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mockResp types.HijackedResponse
			if tt.name == "successful shell execution" {
				// For successful execution, provide a proper mock response
				buf := &bytes.Buffer{}
				buf.WriteString("root@abc123:/workspace# ")

				mockResp = types.HijackedResponse{
					Conn:   &mockConn{Buffer: &bytes.Buffer{}},
					Reader: bufio.NewReader(buf),
				}
			} else {
				mockResp = createMockHijackedResponse()
			}

			mockClient := &mockExecClient{
				containers:         tt.containers,
				execCreateResponse: tt.execCreateResp,
				execCreateError:    tt.execCreateError,
				execAttachResponse: mockResp,
				execAttachError:    tt.execAttachError,
				inspectResponse:    tt.inspectResponse,
			}

			err := executeInteractiveShell(context.Background(), mockClient, tt.containerName, tt.devContainer, []string{"/bin/bash", "-i"}, nil)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.expectedErrorMsg != "" && !strings.Contains(err.Error(), tt.expectedErrorMsg) {
					t.Errorf("error message %q does not contain %q", err.Error(), tt.expectedErrorMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveEnvVars(t *testing.T) {
	t.Setenv("DEVGO_TEST_HOST_VAR", "from_host")

	got := resolveEnvVars([]string{
		"FOO=bar",
		"EMPTY=",
		"WITH=equals=sign",
		"DEVGO_TEST_HOST_VAR",
		"DEVGO_TEST_UNSET_VAR",
		"",
	})

	want := map[string]string{
		"FOO":                 "bar",
		"EMPTY":               "",
		"WITH":                "equals=sign",
		"DEVGO_TEST_HOST_VAR": "from_host",
	}

	if len(got) != len(want) {
		t.Fatalf("resolveEnvVars() = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("resolveEnvVars()[%q] = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["DEVGO_TEST_UNSET_VAR"]; ok {
		t.Errorf("unset host var should be skipped, but it was present")
	}
}

func TestBuildShellEnv(t *testing.T) {
	toMap := func(env []string) map[string]string {
		m := make(map[string]string)
		for _, e := range env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				m[parts[0]] = parts[1]
			}
		}
		return m
	}

	t.Run("defaults TERM when not provided", func(t *testing.T) {
		got := toMap(buildShellEnv(nil, nil))
		if got["TERM"] != "xterm-256color" {
			t.Errorf("TERM = %q, want xterm-256color", got["TERM"])
		}
	})

	t.Run("includes container env", func(t *testing.T) {
		got := toMap(buildShellEnv(map[string]string{"PATH": "/usr/bin"}, nil))
		if got["PATH"] != "/usr/bin" {
			t.Errorf("PATH = %q, want /usr/bin", got["PATH"])
		}
	})

	t.Run("user env overrides container env", func(t *testing.T) {
		got := toMap(buildShellEnv(
			map[string]string{"FOO": "container"},
			[]string{"FOO=user", "BAR=baz"},
		))
		if got["FOO"] != "user" {
			t.Errorf("FOO = %q, want user (override)", got["FOO"])
		}
		if got["BAR"] != "baz" {
			t.Errorf("BAR = %q, want baz", got["BAR"])
		}
	})

	t.Run("user env can override TERM", func(t *testing.T) {
		got := toMap(buildShellEnv(nil, []string{"TERM=dumb"}))
		if got["TERM"] != "dumb" {
			t.Errorf("TERM = %q, want dumb", got["TERM"])
		}
	})
}

func TestShellCommand_PassesExtraEnv(t *testing.T) {
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "testuser",
		WorkspaceFolder: "/workspace",
	}
	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}
	baseMockClient := &mockExecClient{
		containers:         containers,
		execCreateResponse: container.ExecCreateResponse{ID: "exec123"},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{Env: []string{"PATH=/usr/bin"}},
		},
	}
	mockClient := &mockShellExecClient{mockExecClient: baseMockClient}

	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer,
		[]string{"/bin/bash", "-i"}, []string{"FOO=bar"})

	found := false
	for _, e := range mockClient.capturedExecOptions.Env {
		if e == "FOO=bar" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected FOO=bar in exec env, got %v", mockClient.capturedExecOptions.Env)
	}
}

func TestResolveEnvVars_Wildcard(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "ASIAEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_SESSION_TOKEN", "token")
	t.Setenv("NOT_AWS_VAR", "ignored")

	got := resolveEnvVars([]string{"AWS_*"})

	want := map[string]string{
		"AWS_ACCESS_KEY_ID":     "ASIAEXAMPLE",
		"AWS_SECRET_ACCESS_KEY": "secret",
		"AWS_SESSION_TOKEN":     "token",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("resolveEnvVars()[%q] = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["NOT_AWS_VAR"]; ok {
		t.Errorf("wildcard AWS_* should not match NOT_AWS_VAR")
	}
}

func TestResolveEnvVars_MultilineExport(t *testing.T) {
	// Simulates `--env "$(aws configure export-credentials --format env)"`,
	// whose output is several `export KEY=VALUE` lines in one argument.
	blob := "export AWS_ACCESS_KEY_ID=ASIAEXAMPLE\n" +
		"export AWS_SECRET_ACCESS_KEY=secret\n" +
		"export AWS_SESSION_TOKEN=token\n"

	got := resolveEnvVars([]string{blob})

	want := map[string]string{
		"AWS_ACCESS_KEY_ID":     "ASIAEXAMPLE",
		"AWS_SECRET_ACCESS_KEY": "secret",
		"AWS_SESSION_TOKEN":     "token",
	}
	if len(got) != len(want) {
		t.Fatalf("resolveEnvVars() = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("resolveEnvVars()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

// TestResolveEnvVars_EdgeCases covers tricky parsing situations, especially
// whitespace handling in values, which must be preserved exactly.
func TestResolveEnvVars_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		wantKey string
		wantVal string
	}{
		{
			name:    "value with embedded spaces",
			entries: []string{"GREETING=hello world"},
			wantKey: "GREETING",
			wantVal: "hello world",
		},
		{
			name:    "value with trailing spaces is preserved",
			entries: []string{"PADDED=value   "},
			wantKey: "PADDED",
			wantVal: "value   ",
		},
		{
			name:    "value with leading spaces is preserved",
			entries: []string{"PADDED=   value"},
			wantKey: "PADDED",
			wantVal: "   value",
		},
		{
			name:    "value with tabs is preserved",
			entries: []string{"TABBED=a\tb"},
			wantKey: "TABBED",
			wantVal: "a\tb",
		},
		{
			name:    "value containing equals signs",
			entries: []string{"QUERY=a=b=c"},
			wantKey: "QUERY",
			wantVal: "a=b=c",
		},
		{
			name:    "empty value",
			entries: []string{"EMPTY="},
			wantKey: "EMPTY",
			wantVal: "",
		},
		{
			name:    "value of only spaces is preserved",
			entries: []string{"SPACES=   "},
			wantKey: "SPACES",
			wantVal: "   ",
		},
		{
			name:    "export prefix is stripped",
			entries: []string{"export FOO=bar"},
			wantKey: "FOO",
			wantVal: "bar",
		},
		{
			name:    "export prefix with extra spaces before key",
			entries: []string{"export   FOO=bar"},
			wantKey: "FOO",
			wantVal: "bar",
		},
		{
			name:    "leading indentation before key is trimmed",
			entries: []string{"   FOO=bar"},
			wantKey: "FOO",
			wantVal: "bar",
		},
		{
			name:    "key with surrounding spaces is trimmed",
			entries: []string{"  FOO  =bar"},
			wantKey: "FOO",
			wantVal: "bar",
		},
		{
			name:    "CRLF line ending strips the carriage return",
			entries: []string{"FOO=bar\r"},
			wantKey: "FOO",
			wantVal: "bar",
		},
		{
			name:    "value that itself looks like an export statement",
			entries: []string{"CMD=export FOO=bar"},
			wantKey: "CMD",
			wantVal: "export FOO=bar",
		},
		{
			name:    "later assignment wins",
			entries: []string{"FOO=first", "FOO=second"},
			wantKey: "FOO",
			wantVal: "second",
		},
		{
			name:    "later assignment within a multiline blob wins",
			entries: []string{"FOO=first\nFOO=second"},
			wantKey: "FOO",
			wantVal: "second",
		},
		{
			name:    "value with a literal newline-like backslash sequence is kept",
			entries: []string{`FOO=a\nb`},
			wantKey: "FOO",
			wantVal: `a\nb`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveEnvVars(tt.entries)
			if v, ok := got[tt.wantKey]; !ok {
				t.Fatalf("resolveEnvVars() missing key %q (got %v)", tt.wantKey, got)
			} else if v != tt.wantVal {
				t.Errorf("resolveEnvVars()[%q] = %q, want %q", tt.wantKey, v, tt.wantVal)
			}
		})
	}
}

// TestResolveEnvVars_Skips checks inputs that should not produce any variable.
func TestResolveEnvVars_Skips(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
	}{
		{name: "empty string", entries: []string{""}},
		{name: "whitespace only line", entries: []string{"   "}},
		{name: "blank lines in a blob", entries: []string{"\n\n  \n"}},
		{name: "bare export keyword", entries: []string{"export "}},
		{name: "line with only equals and no key", entries: []string{"=novalue"}},
		{name: "line with only spaces before equals", entries: []string{"   =x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveEnvVars(tt.entries)
			if len(got) != 0 {
				t.Errorf("resolveEnvVars(%q) = %v, want empty", tt.entries, got)
			}
		})
	}
}

// TestResolveEnvVars_BlobWithBlankAndExportLines mirrors the real AWS CLI
// output, which can include blank lines and an expiration field.
func TestResolveEnvVars_BlobWithBlankAndExportLines(t *testing.T) {
	blob := "\n" +
		"export AWS_ACCESS_KEY_ID=ASIAEXAMPLE\n" +
		"\n" +
		"export AWS_SECRET_ACCESS_KEY=secret/with+slashes\n" +
		"export AWS_SESSION_TOKEN=token==\n" +
		"export AWS_CREDENTIAL_EXPIRATION=2026-06-27T00:00:00+00:00\n"

	got := resolveEnvVars([]string{blob})

	want := map[string]string{
		"AWS_ACCESS_KEY_ID":         "ASIAEXAMPLE",
		"AWS_SECRET_ACCESS_KEY":     "secret/with+slashes",
		"AWS_SESSION_TOKEN":         "token==",
		"AWS_CREDENTIAL_EXPIRATION": "2026-06-27T00:00:00+00:00",
	}
	if len(got) != len(want) {
		t.Fatalf("resolveEnvVars() = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("resolveEnvVars()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

// TestBuildShellEnv_PreservesWhitespaceValue verifies a value with spaces
// survives end-to-end into the exec env slice as a single entry.
func TestBuildShellEnv_PreservesWhitespaceValue(t *testing.T) {
	env := buildShellEnv(nil, []string{"MSG=hello world  "})

	found := false
	for _, e := range env {
		if e == "MSG=hello world  " {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %q in env, got %v", "MSG=hello world  ", env)
	}
}

func TestResolveShellCommand(t *testing.T) {
	tests := []struct {
		name     string
		override string
		userConfig  *config.UserConfig
		want     []string
	}{
		{
			name:     "default when nothing set",
			override: "",
			userConfig:  nil,
			want:     []string{"/bin/bash", "-i"},
		},
		{
			name:     "user config wins over default",
			override: "",
			userConfig:  &config.UserConfig{Shell: "zsh"},
			want:     []string{"zsh", "-i"},
		},
		{
			name:     "CLI override wins over user config",
			override: "/usr/bin/fish",
			userConfig:  &config.UserConfig{Shell: "zsh"},
			want:     []string{"/usr/bin/fish", "-i"},
		},
		{
			name:     "CLI override with nil user config",
			override: "zsh",
			userConfig:  nil,
			want:     []string{"zsh", "-i"},
		},
		{
			name:     "empty user config Shell falls back to default",
			override: "",
			userConfig:  &config.UserConfig{Shell: ""},
			want:     []string{"/bin/bash", "-i"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveShellCommand(tt.override, tt.userConfig)
			if len(got) != len(tt.want) {
				t.Fatalf("resolveShellCommand() = %v, want %v", got, tt.want)
			}
			for i, v := range tt.want {
				if got[i] != v {
					t.Errorf("resolveShellCommand()[%d] = %q, want %q", i, got[i], v)
				}
			}
		})
	}
}

func TestShellCommand_FallsBackToContainerUser(t *testing.T) {
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "node",
		WorkspaceFolder: "/workspace",
	}
	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}
	baseMockClient := &mockExecClient{
		containers:         containers,
		execCreateResponse: container.ExecCreateResponse{ID: "exec123"},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{Env: []string{"PATH=/usr/bin"}},
		},
	}
	mockClient := &mockShellExecClient{mockExecClient: baseMockClient}

	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer, []string{"/bin/bash", "-i"}, nil)

	if mockClient.capturedExecOptions.User != "node" {
		t.Errorf("expected shell to fall back to containerUser %q, got %q", "node", mockClient.capturedExecOptions.User)
	}
}

func TestShellCommand_PrefersRemoteUser(t *testing.T) {
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "root",
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspace",
	}
	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}
	baseMockClient := &mockExecClient{
		containers:         containers,
		execCreateResponse: container.ExecCreateResponse{ID: "exec123"},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{Env: []string{"PATH=/usr/bin"}},
		},
	}
	mockClient := &mockShellExecClient{mockExecClient: baseMockClient}

	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer, []string{"/bin/bash", "-i"}, nil)

	if mockClient.capturedExecOptions.User != "vscode" {
		t.Errorf("expected shell to run as remoteUser %q, got %q", "vscode", mockClient.capturedExecOptions.User)
	}
}

func TestShellCommand_UsesResolvedShell(t *testing.T) {
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "testuser",
		WorkspaceFolder: "/workspace",
	}
	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}
	baseMockClient := &mockExecClient{
		containers:         containers,
		execCreateResponse: container.ExecCreateResponse{ID: "exec123"},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{Env: []string{"PATH=/usr/bin"}},
		},
	}
	mockClient := &mockShellExecClient{mockExecClient: baseMockClient}

	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer, []string{"zsh", "-i"}, nil)

	got := mockClient.capturedExecOptions.Cmd
	want := []string{"zsh", "-i"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Cmd = %v, want %v", got, want)
	}
}

func TestRunShellCommand_NoArgs(t *testing.T) {
	// Shell command should not require arguments (unlike exec)
	err := runShellCommand([]string{})

	// Should fail due to missing devcontainer config, not due to argument validation
	if err != nil && strings.Contains(err.Error(), "requires at least one argument") {
		t.Errorf("shell command should not require arguments, got: %v", err)
	}

	// Should fail with devcontainer config error instead
	if err == nil {
		t.Errorf("expected error due to missing devcontainer config, got nil")
	} else if !strings.Contains(err.Error(), "devcontainer config") {
		// This is expected - should fail on devcontainer config lookup
		t.Logf("Expected failure due to missing devcontainer config: %v", err)
	}
}

// mockShellExecClient extends mockExecClient to capture exec options
type mockShellExecClient struct {
	*mockExecClient
	capturedExecOptions container.ExecOptions
}

func (m *mockShellExecClient) ContainerExecCreate(ctx context.Context, containerID string, config container.ExecOptions) (container.ExecCreateResponse, error) {
	m.capturedExecOptions = config
	return m.mockExecClient.ContainerExecCreate(ctx, containerID, config)
}

func TestShellCommandExecOptions(t *testing.T) {
	// Test that shell command uses correct exec options
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "testuser",
		WorkspaceFolder: "/test-workspace",
	}

	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}

	baseMockClient := &mockExecClient{
		containers: containers,
		execCreateResponse: container.ExecCreateResponse{
			ID: "exec123",
		},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{
				Env: []string{"PATH=/usr/bin"},
			},
		},
	}

	mockClient := &mockShellExecClient{
		mockExecClient: baseMockClient,
	}

	// This will fail due to terminal handling, but we can still test the exec options
	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer, []string{"/bin/bash", "-i"}, nil)

	// Verify exec options are set correctly for shell command
	capturedExecOptions := mockClient.capturedExecOptions
	if capturedExecOptions.User != "testuser" {
		t.Errorf("expected User to be 'testuser', got %q", capturedExecOptions.User)
	}
	if capturedExecOptions.WorkingDir != "/test-workspace" {
		t.Errorf("expected WorkingDir to be '/test-workspace', got %q", capturedExecOptions.WorkingDir)
	}
	if !capturedExecOptions.Tty {
		t.Errorf("expected Tty to be true for shell command")
	}
	if !capturedExecOptions.AttachStdin {
		t.Errorf("expected AttachStdin to be true for shell command")
	}
	if !capturedExecOptions.AttachStdout {
		t.Errorf("expected AttachStdout to be true for shell command")
	}
	if !capturedExecOptions.AttachStderr {
		t.Errorf("expected AttachStderr to be true for shell command")
	}

	// Verify shell command uses /bin/bash -i for interactive shell
	expectedCmd := []string{"/bin/bash", "-i"}
	if len(capturedExecOptions.Cmd) != len(expectedCmd) {
		t.Errorf("expected Cmd to be %v, got %v", expectedCmd, capturedExecOptions.Cmd)
	} else {
		for i, cmd := range expectedCmd {
			if capturedExecOptions.Cmd[i] != cmd {
				t.Errorf("expected Cmd[%d] to be %q, got %q", i, cmd, capturedExecOptions.Cmd[i])
			}
		}
	}
}

func TestShellCommandContainerNameLogic(t *testing.T) {
	// Test that shell command follows the same container naming logic as other commands
	workspaceDir := "/test/workspace"

	tests := []struct {
		name          string
		devContainer  *devcontainer.DevContainer
		containerName string
		expectedName  string
	}{
		{
			name: "uses devcontainer name",
			devContainer: &devcontainer.DevContainer{
				Name: "custom-shell-container",
			},
			expectedName: "custom-shell-container-default-" + GeneratePathHash(workspaceDir),
		},
		{
			name:         "uses workspace directory name",
			devContainer: &devcontainer.DevContainer{},
			expectedName: "workspace-default-" + GeneratePathHash(workspaceDir),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original global variable
			originalContainerName := containerName
			defer func() {
				containerName = originalContainerName
			}()

			containerName = tt.containerName
			result := determineContainerName(tt.devContainer, workspaceDir)

			if result != tt.expectedName {
				t.Errorf("determineContainerName() = %q, want %q", result, tt.expectedName)
			}
		})
	}
}

func TestShellRespectsBashrc(t *testing.T) {
	devContainer := &devcontainer.DevContainer{
		ContainerUser:   "testuser",
		WorkspaceFolder: "/workspace",
	}

	containers := []container.Summary{
		{
			ID:    "test123",
			Names: []string{"/test-container"},
			Labels: map[string]string{
				constants.DevgoManagedLabel: constants.DevgoManagedValue,
			},
		},
	}

	baseMockClient := &mockExecClient{
		containers: containers,
		execCreateResponse: container.ExecCreateResponse{
			ID: "exec123",
		},
		execAttachResponse: createMockHijackedResponse(),
		inspectResponse: types.ContainerJSON{
			Config: &container.Config{
				Env: []string{"PATH=/usr/bin"},
			},
		},
	}

	mockClient := &mockShellExecClient{
		mockExecClient: baseMockClient,
	}

	_ = executeInteractiveShell(context.Background(), mockClient, "test-container", devContainer, []string{"/bin/bash", "-i"}, nil)

	capturedExecOptions := mockClient.capturedExecOptions

	// Verify PS1 is NOT set in environment to respect container's .bashrc
	// This aligns with README.md documentation about respecting .bashrc configuration
	for _, env := range capturedExecOptions.Env {
		if strings.HasPrefix(env, "PS1=") {
			t.Errorf("PS1 should not be set in environment to respect .bashrc, but found: %q", env)
		}
	}

	// Verify /bin/bash -i is used for interactive shell
	expectedCmd := []string{"/bin/bash", "-i"}
	if len(capturedExecOptions.Cmd) != len(expectedCmd) {
		t.Errorf("expected Cmd to be %v for proper .bashrc sourcing, got %v", expectedCmd, capturedExecOptions.Cmd)
	} else {
		for i, cmd := range expectedCmd {
			if capturedExecOptions.Cmd[i] != cmd {
				t.Errorf("expected Cmd[%d] to be %q for proper .bashrc sourcing, got %q", i, cmd, capturedExecOptions.Cmd[i])
			}
		}
	}
}
