// Package features implements devcontainer "Features" support by pulling
// feature artifacts from an OCI registry and generating a wrapper Dockerfile
// that installs them on top of a base image.
package features

import (
	"fmt"
	"regexp"
	"strings"
)

// OCIRef is a parsed reference to a feature published in an OCI registry,
// e.g. "ghcr.io/devcontainers/features/node:1".
type OCIRef struct {
	Registry   string // e.g. "ghcr.io"
	Repository string // e.g. "devcontainers/features/node"
	Tag        string // e.g. "1" (defaults to "latest")
	Raw        string // the original reference string
}

var sanitizeIDPattern = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// ParseOCIRef parses an OCI feature reference. Only registry-hosted references
// are supported; local paths and HTTPS tarballs are rejected. The registry host
// is identified by the first path segment containing a "." or a ":".
func ParseOCIRef(s string) (*OCIRef, error) {
	raw := s
	if s == "" {
		return nil, fmt.Errorf("empty feature reference")
	}
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") {
		return nil, fmt.Errorf("local path feature references are not supported: %q", raw)
	}
	if strings.Contains(s, "://") {
		return nil, fmt.Errorf("URL feature references are not supported: %q", raw)
	}

	// Split off the tag. Only treat a ":" as a tag separator if it appears after
	// the last "/", so that a registry host:port is not mistaken for a tag.
	tag := "latest"
	if idx := strings.LastIndex(s, ":"); idx != -1 && idx > strings.LastIndex(s, "/") {
		tag = s[idx+1:]
		s = s[:idx]
		if tag == "" {
			return nil, fmt.Errorf("empty tag in feature reference: %q", raw)
		}
	}

	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("feature reference must include a registry host: %q", raw)
	}
	registry := parts[0]
	if !strings.Contains(registry, ".") && !strings.Contains(registry, ":") {
		return nil, fmt.Errorf("feature reference must include a registry host: %q", raw)
	}
	repository := parts[1]
	if repository == "" {
		return nil, fmt.Errorf("feature reference must include a repository: %q", raw)
	}

	return &OCIRef{
		Registry:   registry,
		Repository: repository,
		Tag:        tag,
		Raw:        raw,
	}, nil
}

// Name returns the registry/repository:tag string used to pull the artifact.
func (r *OCIRef) Name() string {
	return fmt.Sprintf("%s/%s:%s", r.Registry, r.Repository, r.Tag)
}

// SanitizedID returns a filesystem-safe identifier derived from the reference,
// suitable for use as a directory name within the build context.
func (r *OCIRef) SanitizedID() string {
	id := fmt.Sprintf("%s-%s-%s", r.Registry, r.Repository, r.Tag)
	id = sanitizeIDPattern.ReplaceAllString(id, "-")
	return strings.Trim(id, "-")
}
