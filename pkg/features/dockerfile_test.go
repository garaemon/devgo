package features

import (
	"strings"
	"testing"
)

func TestGenerateWrapperDockerfile(t *testing.T) {
	feats := []*PulledFeature{
		{
			Ref:      &OCIRef{Registry: "ghcr.io", Repository: "devcontainers/features/node", Tag: "1", Raw: "ghcr.io/devcontainers/features/node:1"},
			Metadata: &FeatureMetadata{ContainerEnv: map[string]string{"NVM_DIR": "/usr/local/share/nvm"}},
		},
	}
	optionValues := map[string]map[string]string{
		"ghcr.io/devcontainers/features/node:1": {"VERSION": "18"},
	}

	got := GenerateWrapperDockerfile("ubuntu:22.04", feats, ".devgo-features", optionValues, "")

	wantContains := []string{
		"FROM ubuntu:22.04",
		"USER root",
		"# ---- feature: ghcr.io/devcontainers/features/node:1 ----",
		"COPY .devgo-features/ghcr.io-devcontainers-features-node-1 /tmp/devgo-features/ghcr.io-devcontainers-features-node-1",
		"ENV NVM_DIR=/usr/local/share/nvm",
		"ENV VERSION=18",
		"RUN cd /tmp/devgo-features/ghcr.io-devcontainers-features-node-1 && chmod +x install.sh && ./install.sh",
		"RUN rm -rf /tmp/devgo-features",
	}
	for _, w := range wantContains {
		if !strings.Contains(got, w) {
			t.Errorf("generated Dockerfile missing %q\n---\n%s", w, got)
		}
	}

	// Option ENV must come before the RUN that executes install.sh.
	envIdx := strings.Index(got, "ENV VERSION=18")
	runIdx := strings.Index(got, "RUN cd /tmp/devgo-features")
	if envIdx == -1 || runIdx == -1 || envIdx > runIdx {
		t.Errorf("ENV must precede RUN; envIdx=%d runIdx=%d", envIdx, runIdx)
	}
}

func TestGenerateWrapperDockerfileMultipleFeatures(t *testing.T) {
	feats := []*PulledFeature{
		{Ref: &OCIRef{Registry: "ghcr.io", Repository: "a", Tag: "1", Raw: "ghcr.io/a:1"}, Metadata: &FeatureMetadata{}},
		{Ref: &OCIRef{Registry: "ghcr.io", Repository: "b", Tag: "1", Raw: "ghcr.io/b:1"}, Metadata: &FeatureMetadata{}},
	}
	got := GenerateWrapperDockerfile("base", feats, ".devgo-features", nil, "")

	aIdx := strings.Index(got, "feature: ghcr.io/a:1")
	bIdx := strings.Index(got, "feature: ghcr.io/b:1")
	if aIdx == -1 || bIdx == -1 || aIdx > bIdx {
		t.Errorf("features should appear in order; aIdx=%d bIdx=%d", aIdx, bIdx)
	}
}

func TestQuoteEnvValue(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"18", "18"},
		{"hello world", `"hello world"`},
		{"", `""`},
	}
	for _, tt := range tests {
		if got := quoteEnvValue(tt.in); got != tt.want {
			t.Errorf("quoteEnvValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGenerateWrapperDockerfileRestoresBaseUser(t *testing.T) {
	feats := []*PulledFeature{
		{
			Ref:      &OCIRef{Registry: "ghcr.io", Repository: "devcontainers/features/node", Tag: "1", Raw: "ghcr.io/devcontainers/features/node:1"},
			Metadata: &FeatureMetadata{},
		},
	}

	got := GenerateWrapperDockerfile("mcr.microsoft.com/devcontainers/base", feats,
		".devgo-features", nil, "vscode")

	if !strings.Contains(got, "\nUSER vscode\n") {
		t.Fatalf("generated Dockerfile does not restore the base user\n---\n%s", got)
	}

	// Installing must still happen as root, and the switch back must be the last
	// USER instruction in the file.
	rootIdx := strings.Index(got, "USER root")
	userIdx := strings.LastIndex(got, "USER vscode")
	installIdx := strings.Index(got, "install.sh")
	cleanupIdx := strings.Index(got, "RUN rm -rf")
	if rootIdx == -1 || userIdx == -1 || installIdx == -1 || cleanupIdx == -1 {
		t.Fatalf("generated Dockerfile is missing expected instructions\n---\n%s", got)
	}
	if !(rootIdx < installIdx && installIdx < cleanupIdx && cleanupIdx < userIdx) {
		t.Errorf("instructions are out of order: root=%d install=%d cleanup=%d user=%d\n---\n%s",
			rootIdx, installIdx, cleanupIdx, userIdx, got)
	}
	if strings.Count(got, "USER ") != 2 {
		t.Errorf("expected exactly two USER instructions, got %d\n---\n%s",
			strings.Count(got, "USER "), got)
	}
}

func TestGenerateWrapperDockerfileWithoutBaseUser(t *testing.T) {
	feats := []*PulledFeature{
		{
			Ref:      &OCIRef{Registry: "ghcr.io", Repository: "a", Tag: "1", Raw: "ghcr.io/a:1"},
			Metadata: &FeatureMetadata{},
		},
	}

	// A base image without a USER instruction already runs as root, so nothing
	// needs restoring.
	got := GenerateWrapperDockerfile("ubuntu:22.04", feats, ".devgo-features", nil, "")

	if strings.Count(got, "USER ") != 1 {
		t.Errorf("expected only the initial USER root, got %d USER instructions\n---\n%s",
			strings.Count(got, "USER "), got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "RUN rm -rf /tmp/devgo-features") {
		t.Errorf("generated Dockerfile should end with the cleanup step\n---\n%s", got)
	}
}
