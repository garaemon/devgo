package cmd

import (
	"testing"
)

func TestRunDownCommand(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectError   bool
		errorContains string
	}{
		{
			name:          "down command with no args",
			args:          []string{},
			expectError:   false, // Command succeeds, container doesn't exist message is expected
			errorContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runDownCommand(tt.args)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !containsSubstring(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain '%s' but got '%s'", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}