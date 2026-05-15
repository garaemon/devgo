package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// mockDotfilesClient mocks the subset of the Docker SDK used by
// dotfilesExecutor. It captures the User passed to ContainerExecCreate so
// tests can assert that the executor forwards the caller-supplied user.
type mockDotfilesClient struct {
	createErr   error
	attachErr   error
	startErr    error
	inspectErr  error
	exitCode    int
	attachResp  types.HijackedResponse
	createdUser string
}

func (m *mockDotfilesClient) ContainerExecCreate(_ context.Context, _ string, cfg container.ExecOptions) (container.ExecCreateResponse, error) {
	m.createdUser = cfg.User
	if m.createErr != nil {
		return container.ExecCreateResponse{}, m.createErr
	}
	return container.ExecCreateResponse{ID: "exec1"}, nil
}

func (m *mockDotfilesClient) ContainerExecAttach(_ context.Context, _ string, _ container.ExecAttachOptions) (types.HijackedResponse, error) {
	if m.attachErr != nil {
		return types.HijackedResponse{}, m.attachErr
	}
	return m.attachResp, nil
}

func (m *mockDotfilesClient) ContainerExecStart(_ context.Context, _ string, _ container.ExecStartOptions) error {
	return m.startErr
}

func (m *mockDotfilesClient) ContainerExecInspect(_ context.Context, _ string) (container.ExecInspect, error) {
	if m.inspectErr != nil {
		return container.ExecInspect{}, m.inspectErr
	}
	return container.ExecInspect{ExitCode: m.exitCode}, nil
}

func TestDotfilesExecutor_HappyPath(t *testing.T) {
	mock := &mockDotfilesClient{
		attachResp: createMockHijackedResponseValid(),
		exitCode:   0,
	}
	exec := newDotfilesExecutor(mock, "container1")
	stdout, _, code, err := exec.Exec(context.Background(), "vscode", []string{"true"})
	if err != nil {
		t.Fatalf("Exec error = %v, want nil", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if mock.createdUser != "vscode" {
		t.Errorf("expected User=vscode forwarded, got %q", mock.createdUser)
	}
	if !strings.Contains(stdout, "ok") {
		t.Errorf("expected stdout to include payload, got %q", stdout)
	}
}

func TestDotfilesExecutor_CreateError_ReturnsExitUnknown(t *testing.T) {
	mock := &mockDotfilesClient{createErr: errors.New("boom")}
	exec := newDotfilesExecutor(mock, "container1")
	_, _, code, err := exec.Exec(context.Background(), "u", []string{"true"})
	if err == nil {
		t.Fatalf("expected error from Exec when ExecCreate fails")
	}
	if code != dotfilesExecExitUnknown {
		t.Errorf("expected exit code = %d (unknown), got %d", dotfilesExecExitUnknown, code)
	}
}

func TestDotfilesExecutor_AttachError_ReturnsExitUnknown(t *testing.T) {
	mock := &mockDotfilesClient{attachErr: errors.New("boom")}
	exec := newDotfilesExecutor(mock, "container1")
	_, _, code, err := exec.Exec(context.Background(), "u", []string{"true"})
	if err == nil {
		t.Fatalf("expected error from Exec when ExecAttach fails")
	}
	if code != dotfilesExecExitUnknown {
		t.Errorf("expected exit code = %d (unknown), got %d", dotfilesExecExitUnknown, code)
	}
}

func TestDotfilesExecutor_StartError_ReturnsExitUnknown(t *testing.T) {
	mock := &mockDotfilesClient{
		attachResp: createMockHijackedResponseValid(),
		startErr:   errors.New("boom"),
	}
	exec := newDotfilesExecutor(mock, "container1")
	_, _, code, err := exec.Exec(context.Background(), "u", []string{"true"})
	if err == nil {
		t.Fatalf("expected error from Exec when ExecStart fails")
	}
	if code != dotfilesExecExitUnknown {
		t.Errorf("expected exit code = %d (unknown), got %d", dotfilesExecExitUnknown, code)
	}
}

func TestDotfilesExecutor_InspectError_ReturnsExitUnknown(t *testing.T) {
	mock := &mockDotfilesClient{
		attachResp: createMockHijackedResponseValid(),
		inspectErr: errors.New("boom"),
	}
	exec := newDotfilesExecutor(mock, "container1")
	_, _, code, err := exec.Exec(context.Background(), "u", []string{"true"})
	if err == nil {
		t.Fatalf("expected error from Exec when ExecInspect fails")
	}
	if code != dotfilesExecExitUnknown {
		t.Errorf("expected exit code = %d (unknown), got %d", dotfilesExecExitUnknown, code)
	}
}

func TestDotfilesExecutor_InspectReportsRealExitCode(t *testing.T) {
	mock := &mockDotfilesClient{
		attachResp: createMockHijackedResponseValid(),
		exitCode:   42,
	}
	exec := newDotfilesExecutor(mock, "container1")
	_, _, code, err := exec.Exec(context.Background(), "u", []string{"false"})
	if err != nil {
		t.Fatalf("Exec error = %v, want nil (non-zero exit is not an error)", err)
	}
	if code != 42 {
		t.Errorf("expected exit code = 42 from ContainerExecInspect, got %d", code)
	}
}
