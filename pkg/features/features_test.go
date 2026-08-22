package features

import (
	"archive/tar"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/garaemon/devgo/pkg/devcontainer"
)

// nodeFeatureEntries is a minimal but realistic feature payload: one declared
// option with a default, one containerEnv entry, and an install script.
func nodeFeatureEntries() []tarEntry {
	return []tarEntry{
		{
			name:     "devcontainer-feature.json",
			typeflag: tar.TypeReg,
			mode:     0o644,
			body: `{
				"id": "node",
				"version": "1.0.0",
				"options": {
					"version": { "type": "string", "default": "lts" },
					"install-yarn": { "type": "boolean", "default": true }
				},
				"containerEnv": { "NVM_DIR": "/usr/local/share/nvm" }
			}`,
		},
		{name: "install.sh", typeflag: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\necho installing\n"},
	}
}

func TestApplyFeaturesRestoresBaseUser(t *testing.T) {
	host := startTestRegistry(t)
	ref := pushTestFeature(t, host, "devcontainers/features/node", "1", nodeFeatureEntries())

	buildContext := t.TempDir()
	resolved := ""
	result, err := ApplyFeatures(ApplyInput{
		BaseImage:    "mcr.microsoft.com/devcontainers/base:jammy",
		Specs:        []devcontainer.FeatureSpec{{Ref: ref}},
		BuildContext: buildContext,
		ResolveBaseUser: func(image string) (string, error) {
			resolved = image
			return "vscode", nil
		},
	})
	if err != nil {
		t.Fatalf("ApplyFeatures returned an unexpected error: %v", err)
	}
	defer result.Cleanup()

	if resolved != "mcr.microsoft.com/devcontainers/base:jammy" {
		t.Errorf("resolver was asked about %q, want the base image", resolved)
	}

	wantPath := filepath.Join(buildContext, featuresSubdir, "Dockerfile.devgo")
	if result.DockerfilePath != wantPath {
		t.Errorf("DockerfilePath = %q, want %q", result.DockerfilePath, wantPath)
	}

	data, err := os.ReadFile(result.DockerfilePath)
	if err != nil {
		t.Fatalf("failed to read the generated Dockerfile: %v", err)
	}
	got := string(data)

	if !strings.HasSuffix(strings.TrimSpace(got), "USER vscode") {
		t.Errorf("generated Dockerfile does not end by restoring the base user\n---\n%s", got)
	}
	if !strings.Contains(got, "FROM mcr.microsoft.com/devcontainers/base:jammy") {
		t.Errorf("generated Dockerfile does not build on the base image\n---\n%s", got)
	}

	// The feature payload is extracted where the generated COPY expects it.
	parsed, err := ParseOCIRef(ref)
	if err != nil {
		t.Fatalf("failed to parse the test reference: %v", err)
	}
	installScript := filepath.Join(buildContext, featuresSubdir, parsed.SanitizedID(), "install.sh")
	if _, err := os.Stat(installScript); err != nil {
		t.Errorf("expected install.sh to be extracted into the build context: %v", err)
	}
}

func TestApplyFeaturesPrefersExplicitBaseUser(t *testing.T) {
	host := startTestRegistry(t)
	ref := pushTestFeature(t, host, "devcontainers/features/node", "1", nodeFeatureEntries())

	result, err := ApplyFeatures(ApplyInput{
		BaseImage:    "ubuntu:22.04",
		Specs:        []devcontainer.FeatureSpec{{Ref: ref}},
		BuildContext: t.TempDir(),
		BaseUser:     "devgo",
		ResolveBaseUser: func(string) (string, error) {
			t.Error("resolver was called even though BaseUser was already known")
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("ApplyFeatures returned an unexpected error: %v", err)
	}
	defer result.Cleanup()

	data, err := os.ReadFile(result.DockerfilePath)
	if err != nil {
		t.Fatalf("failed to read the generated Dockerfile: %v", err)
	}
	if !strings.Contains(string(data), "\nUSER devgo\n") {
		t.Errorf("generated Dockerfile does not use the explicit base user\n---\n%s", data)
	}
}

func TestApplyFeaturesWithoutResolverLeavesUserUnchanged(t *testing.T) {
	host := startTestRegistry(t)
	ref := pushTestFeature(t, host, "devcontainers/features/node", "1", nodeFeatureEntries())

	result, err := ApplyFeatures(ApplyInput{
		BaseImage:    "ubuntu:22.04",
		Specs:        []devcontainer.FeatureSpec{{Ref: ref}},
		BuildContext: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("ApplyFeatures returned an unexpected error: %v", err)
	}
	defer result.Cleanup()

	data, err := os.ReadFile(result.DockerfilePath)
	if err != nil {
		t.Fatalf("failed to read the generated Dockerfile: %v", err)
	}
	if strings.Count(string(data), "USER ") != 1 {
		t.Errorf("expected only USER root when no base user is known\n---\n%s", data)
	}
}

func TestApplyFeaturesResolverFailure(t *testing.T) {
	host := startTestRegistry(t)
	ref := pushTestFeature(t, host, "devcontainers/features/node", "1", nodeFeatureEntries())

	wantErr := errors.New("no such image")
	buildContext := t.TempDir()
	_, err := ApplyFeatures(ApplyInput{
		BaseImage:       "ubuntu:22.04",
		Specs:           []devcontainer.FeatureSpec{{Ref: ref}},
		BuildContext:    buildContext,
		ResolveBaseUser: func(string) (string, error) { return "", wantErr },
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want it to wrap %v", err, wantErr)
	}

	// The failure happens before anything is pulled, so the build context stays
	// untouched.
	if _, statErr := os.Stat(filepath.Join(buildContext, featuresSubdir)); !os.IsNotExist(statErr) {
		t.Error("the build context was modified despite the resolver failing")
	}
}

func TestApplyFeaturesResolvesOptions(t *testing.T) {
	host := startTestRegistry(t)
	ref := pushTestFeature(t, host, "devcontainers/features/node", "1", nodeFeatureEntries())

	result, err := ApplyFeatures(ApplyInput{
		BaseImage:    "ubuntu:22.04",
		Specs:        []devcontainer.FeatureSpec{{Ref: ref, Options: map[string]interface{}{"version": "18"}}},
		BuildContext: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("ApplyFeatures returned an unexpected error: %v", err)
	}
	defer result.Cleanup()

	data, err := os.ReadFile(result.DockerfilePath)
	if err != nil {
		t.Fatalf("failed to read the generated Dockerfile: %v", err)
	}
	got := string(data)

	want := []string{
		"ENV VERSION=18",                   // user value wins over the "lts" default
		"ENV INSTALL_YARN=true",            // untouched option falls back to its default
		"ENV NVM_DIR=/usr/local/share/nvm", // the feature's containerEnv
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("generated Dockerfile missing %q\n---\n%s", w, got)
		}
	}
}

func TestApplyFeaturesNoSpecs(t *testing.T) {
	_, err := ApplyFeatures(ApplyInput{
		BaseImage:    "ubuntu:22.04",
		BuildContext: t.TempDir(),
	})
	if err == nil {
		t.Fatal("ApplyFeatures accepted an empty feature list")
	}
}

func TestApplyFeaturesUnknownFeature(t *testing.T) {
	host := startTestRegistry(t)

	_, err := ApplyFeatures(ApplyInput{
		BaseImage:    "ubuntu:22.04",
		Specs:        []devcontainer.FeatureSpec{{Ref: host + "/devcontainers/features/absent:1"}},
		BuildContext: t.TempDir(),
	})
	if err == nil {
		t.Fatal("ApplyFeatures succeeded for a feature that was never pushed")
	}
}

func TestApplyFeaturesRejectsLocalReference(t *testing.T) {
	_, err := ApplyFeatures(ApplyInput{
		BaseImage:    "ubuntu:22.04",
		Specs:        []devcontainer.FeatureSpec{{Ref: "./local-feature"}},
		BuildContext: t.TempDir(),
	})
	if err == nil {
		t.Fatal("ApplyFeatures accepted a local path reference")
	}
	if !strings.Contains(err.Error(), "local path") {
		t.Errorf("error = %q, want it to explain that local paths are unsupported", err)
	}
}

func TestApplyResultCleanup(t *testing.T) {
	host := startTestRegistry(t)
	ref := pushTestFeature(t, host, "devcontainers/features/node", "1", nodeFeatureEntries())

	buildContext := t.TempDir()
	result, err := ApplyFeatures(ApplyInput{
		BaseImage:    "ubuntu:22.04",
		Specs:        []devcontainer.FeatureSpec{{Ref: ref}},
		BuildContext: buildContext,
	})
	if err != nil {
		t.Fatalf("ApplyFeatures returned an unexpected error: %v", err)
	}

	featuresDir := filepath.Join(buildContext, featuresSubdir)
	if _, err := os.Stat(featuresDir); err != nil {
		t.Fatalf("expected the features directory to exist before cleanup: %v", err)
	}

	result.Cleanup()

	if _, err := os.Stat(featuresDir); !os.IsNotExist(err) {
		t.Error("Cleanup did not remove the features directory")
	}

	// Cleanup is safe to call more than once, and on a zero value.
	result.Cleanup()
	(&ApplyResult{}).Cleanup()
	var nilResult *ApplyResult
	nilResult.Cleanup()
}
