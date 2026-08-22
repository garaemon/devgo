package features

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeRuntime writes an executable stub that stands in for docker/podman. It
// records the arguments it was invoked with, prints stdout, and exits with the
// given status.
func fakeRuntime(t *testing.T, stdout string, exitCode int) (binary, argsFile string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the stub runtime is a shell script")
	}

	dir := t.TempDir()
	binary = filepath.Join(dir, "fake-runtime")
	argsFile = filepath.Join(dir, "args")

	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" > " + argsFile + "\n" +
		"printf '%s' '" + stdout + "'\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil { //nolint:gosec // test stub
		t.Fatalf("failed to write the stub runtime: %v", err)
	}
	return binary, argsFile
}

func TestCommandImageUser(t *testing.T) {
	binary, argsFile := fakeRuntime(t, "vscode\n", 0)

	got, err := CommandImageUser(binary)("mcr.microsoft.com/devcontainers/base:jammy")
	if err != nil {
		t.Fatalf("CommandImageUser returned an unexpected error: %v", err)
	}
	if got != "vscode" {
		t.Errorf("user = %q, want %q", got, "vscode")
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read the recorded arguments: %v", err)
	}
	wantArgs := "image inspect --format {{.Config.User}} mcr.microsoft.com/devcontainers/base:jammy"
	if strings.TrimSpace(string(args)) != wantArgs {
		t.Errorf("runtime called with %q, want %q", strings.TrimSpace(string(args)), wantArgs)
	}
}

func TestCommandImageUserNoUserConfigured(t *testing.T) {
	binary, _ := fakeRuntime(t, "\n", 0)

	got, err := CommandImageUser(binary)("ubuntu:22.04")
	if err != nil {
		t.Fatalf("CommandImageUser returned an unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("user = %q, want an empty string for an image without a USER", got)
	}
}

func TestCommandImageUserFailure(t *testing.T) {
	binary, _ := fakeRuntime(t, "", 1)

	_, err := CommandImageUser(binary)("missing:latest")
	if err == nil {
		t.Fatal("CommandImageUser succeeded despite the runtime failing")
	}
	if !strings.Contains(err.Error(), "missing:latest") {
		t.Errorf("error = %q, want it to name the image", err)
	}
}

func TestCommandImageUserMissingBinary(t *testing.T) {
	_, err := CommandImageUser(filepath.Join(t.TempDir(), "not-installed"))("ubuntu:22.04")
	if err == nil {
		t.Fatal("CommandImageUser succeeded despite the runtime being absent")
	}
}
