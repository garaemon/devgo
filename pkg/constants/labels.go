package constants

// Docker labels used by devgo to manage containers
const (
	// DevgoManagedLabel is the label key used to identify containers managed by devgo
	DevgoManagedLabel = "devgo.managed"

	// DevgoManagedValue is the value for the managed label
	DevgoManagedValue = "true"

	// DevgoWorkspaceLabel is the label key used to store the workspace path
	DevgoWorkspaceLabel = "devgo.workspace"
)
