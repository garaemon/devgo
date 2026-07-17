package integration

import (
	"os"
	"strings"
)

// containerRuntime returns the container runtime binary the integration tests
// should drive for their own setup, verification, and cleanup commands. It
// mirrors how devgo itself resolves the runtime from DEVGO_CONTAINER_RUNTIME,
// so a single environment variable keeps the devgo binary under test and the
// tests' helper commands (ps, inspect, exec, compose, ...) pointed at the same
// engine. The default is "docker".
func containerRuntime() string {
	if r := strings.TrimSpace(os.Getenv("DEVGO_CONTAINER_RUNTIME")); r != "" {
		return strings.ToLower(r)
	}
	return "docker"
}
