package cmd

import (
	"testing"
)

func TestRunReadConfigurationCommand(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectError  bool
		errorMessage string
	}{
		{
			name:         "empty args",
			args:         []string{},
			expectError:  true,
			errorMessage: "read-configuration command not implemented",
		},
		{
			name:         "with args",
			args:         []string{"--help"},
			expectError:  true,
			errorMessage: "read-configuration command not implemented",
		},
		{
			name:         "multiple args",
			args:         []string{"arg1", "arg2"},
			expectError:  true,
			errorMessage: "read-configuration command not implemented",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runReadConfigurationCommand(tt.args)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if err.Error() != tt.errorMessage {
					t.Errorf("expected error message %q, got %q", tt.errorMessage, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}