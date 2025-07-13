package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/garaemon/devgo/pkg/devcontainer"
)

func runReadConfigurationCommand(args []string) error {
	devcontainerPath, err := findDevcontainerConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to find devcontainer config: %w", err)
	}

	devContainer, err := devcontainer.Parse(devcontainerPath)
	if err != nil {
		return fmt.Errorf("failed to parse devcontainer config: %w", err)
	}

	jsonData, err := json.MarshalIndent(devContainer, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration to JSON: %w", err)
	}

	fmt.Println(string(jsonData))
	return nil
}
