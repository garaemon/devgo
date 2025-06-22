package cmd

import (
	"testing"
)


func TestRunDownCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "down command with no args",
			args: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runDownCommand(tt.args)
			
			// The command may succeed if devcontainer.json exists (locally)
			// or fail if devcontainer.json doesn't exist (CI environment)
			// Both are acceptable behaviors for this test
			if err != nil {
				// If it fails, it should be due to devcontainer config issue
				if !containsSubstring(err.Error(), "failed to find devcontainer config") {
					t.Errorf("unexpected error: %v", err)
				}
			}
			// If err is nil, the command succeeded (found devcontainer, container doesn't exist)
		})
	}
}