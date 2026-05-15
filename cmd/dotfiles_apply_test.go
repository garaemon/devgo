package cmd

import (
	"context"
	"testing"

	"github.com/garaemon/devgo/pkg/devcontainer"
)

// TestApplyDotfiles_NoOpWhenDisabled covers the short-circuit path where
// --no-dotfiles is set: no Docker client should be created, no container
// lookup attempted, and the function returns nil.
func TestApplyDotfiles_NoOpWhenDisabled(t *testing.T) {
	resetPersonalizationFlags()
	noDotfiles = true
	t.Cleanup(resetPersonalizationFlags)

	// Point user config at an empty temp dir to ensure no real config bleeds in.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dc := &devcontainer.DevContainer{ContainerUser: "vscode"}
	if err := applyDotfiles(context.Background(), dc, "any-container"); err != nil {
		t.Errorf("applyDotfiles with --no-dotfiles should be a no-op, got %v", err)
	}
}

// TestApplyDotfiles_NoOpWhenNotConfigured covers the short-circuit path
// when no user config file exists and no CLI flags are set: Resolve
// returns nil and applyDotfiles must return nil before contacting Docker.
func TestApplyDotfiles_NoOpWhenNotConfigured(t *testing.T) {
	resetPersonalizationFlags()
	t.Cleanup(resetPersonalizationFlags)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dc := &devcontainer.DevContainer{ContainerUser: "vscode"}
	if err := applyDotfiles(context.Background(), dc, "any-container"); err != nil {
		t.Errorf("applyDotfiles with no config and no flags should be a no-op, got %v", err)
	}
}
