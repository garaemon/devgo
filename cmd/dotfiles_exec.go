package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// dotfilesExecExitUnknown signals that the command failed before its real
// exit code could be observed (e.g. an I/O error). Distinguishes "command
// ran and returned 0" from "we never got a status".
const dotfilesExecExitUnknown = -1

// dotfilesDockerClient is the subset of the Docker API used to execute
// dotfiles commands inside a running container. Defined here (rather than
// reusing DockerExecClient) so the exit code via ContainerExecInspect can be
// captured.
type dotfilesDockerClient interface {
	ContainerExecCreate(ctx context.Context, containerID string, config container.ExecOptions) (container.ExecCreateResponse, error)
	ContainerExecAttach(ctx context.Context, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error)
	ContainerExecStart(ctx context.Context, execID string, config container.ExecStartOptions) error
	ContainerExecInspect(ctx context.Context, execID string) (container.ExecInspect, error)
}

// dotfilesExecutor adapts a Docker client to the dotfiles.Executor interface.
type dotfilesExecutor struct {
	cli         dotfilesDockerClient
	containerID string
}

// newDotfilesExecutor wraps a Docker client so it satisfies the
// pkg/dotfiles.Executor interface, scoped to a single container ID.
func newDotfilesExecutor(cli dotfilesDockerClient, containerID string) *dotfilesExecutor {
	return &dotfilesExecutor{cli: cli, containerID: containerID}
}

// Exec runs cmd inside the container as user, capturing stdout/stderr and
// returning the exit code reported by Docker. On any error before a real
// exit code is observed it returns dotfilesExecExitUnknown so callers
// cannot mistake an I/O failure for a successful zero exit.
//
// TODO: pass Env explicitly (e.g. HOME from `getent passwd <user>`) for
// images where switching User via docker exec does not re-export $HOME.
// pkg/dotfiles.resolveHome currently relies on the shell expanding $HOME.
func (d *dotfilesExecutor) Exec(ctx context.Context, user string, cmd []string) (string, string, int, error) {
	execConfig := container.ExecOptions{
		User:         user,
		Tty:          false,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}

	create, err := d.cli.ContainerExecCreate(ctx, d.containerID, execConfig)
	if err != nil {
		return "", "", dotfilesExecExitUnknown, fmt.Errorf("failed to create exec: %w", err)
	}

	attach, err := d.cli.ContainerExecAttach(ctx, create.ID, container.ExecAttachOptions{Tty: false})
	if err != nil {
		return "", "", dotfilesExecExitUnknown, fmt.Errorf("failed to attach exec: %w", err)
	}
	defer attach.Close()

	if err := d.cli.ContainerExecStart(ctx, create.ID, container.ExecStartOptions{}); err != nil {
		return "", "", dotfilesExecExitUnknown, fmt.Errorf("failed to start exec: %w", err)
	}

	var stdout, stderr bytes.Buffer
	if _, copyErr := stdcopy.StdCopy(&stdout, &stderr, attach.Reader); copyErr != nil && !errors.Is(copyErr, io.EOF) {
		return stdout.String(), stderr.String(), dotfilesExecExitUnknown, fmt.Errorf("failed to read exec output: %w", copyErr)
	}

	inspect, err := d.cli.ContainerExecInspect(ctx, create.ID)
	if err != nil {
		return stdout.String(), stderr.String(), dotfilesExecExitUnknown, fmt.Errorf("failed to inspect exec: %w", err)
	}

	return stdout.String(), stderr.String(), inspect.ExitCode, nil
}
