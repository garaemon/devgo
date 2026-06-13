package features

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/titanous/json5"
)

// FeatureMetadata is the subset of devcontainer-feature.json that devgo uses
// in its MVP implementation.
type FeatureMetadata struct {
	ID      string                   `json:"id"`
	Version string                   `json:"version"`
	Options map[string]FeatureOption `json:"options"`
	// ContainerEnv is merged into the final image via ENV instructions.
	ContainerEnv map[string]string `json:"containerEnv"`
}

// FeatureOption describes a single configurable option of a feature.
type FeatureOption struct {
	Type      string      `json:"type"`
	Default   interface{} `json:"default"`
	Enum      []string    `json:"enum"`
	Proposals []string    `json:"proposals"`
}

// ParseFeatureMetadata parses the contents of a devcontainer-feature.json file.
func ParseFeatureMetadata(data []byte) (*FeatureMetadata, error) {
	var meta FeatureMetadata
	if err := json5.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse devcontainer-feature.json: %w", err)
	}
	return &meta, nil
}

var (
	nonWordPattern = regexp.MustCompile(`[^\w_]`)
	leadingPattern = regexp.MustCompile(`^[\d_]+`)
)

// OptionEnv converts a feature option id into the environment variable name that
// the feature's install.sh expects, following the devcontainers spec rule:
//
//	id.replace(/[^\w_]/g, '_').replace(/^[\d_]+/g, '_').toUpperCase()
func OptionEnv(optionID string) string {
	s := nonWordPattern.ReplaceAllString(optionID, "_")
	s = leadingPattern.ReplaceAllString(s, "_")
	return strings.ToUpper(s)
}

// ResolveOptionValues computes the environment variables passed to install.sh.
// User-provided values override the option's declared default; options omitted
// by the user fall back to their default. Options the user provides that are not
// declared in the metadata are still passed through.
func (m *FeatureMetadata) ResolveOptionValues(userOpts map[string]interface{}) map[string]string {
	result := make(map[string]string)

	// Start from declared defaults.
	for id, opt := range m.Options {
		if opt.Default != nil {
			result[OptionEnv(id)] = stringifyOptionValue(opt.Default)
		}
	}

	// Apply user overrides (and any extra options).
	for id, val := range userOpts {
		result[OptionEnv(id)] = stringifyOptionValue(val)
	}

	return result
}

// stringifyOptionValue renders an option value as a string suitable for an env
// var. JSON5 decodes numbers as float64, so integral values are rendered without
// a trailing ".0".
func stringifyOptionValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}
