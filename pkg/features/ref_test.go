package features

import "testing"

func TestParseOCIRef(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    bool
		registry   string
		repository string
		tag        string
	}{
		{
			name:       "full reference with tag",
			input:      "ghcr.io/devcontainers/features/node:1",
			registry:   "ghcr.io",
			repository: "devcontainers/features/node",
			tag:        "1",
		},
		{
			name:       "reference without tag defaults to latest",
			input:      "ghcr.io/devcontainers/features/node",
			registry:   "ghcr.io",
			repository: "devcontainers/features/node",
			tag:        "latest",
		},
		{
			name:       "registry with port",
			input:      "localhost:5000/myfeature:2",
			registry:   "localhost:5000",
			repository: "myfeature",
			tag:        "2",
		},
		{
			name:    "local relative path is rejected",
			input:   "./my-feature",
			wantErr: true,
		},
		{
			name:    "absolute path is rejected",
			input:   "/opt/feature",
			wantErr: true,
		},
		{
			name:    "url is rejected",
			input:   "https://example.com/feature.tgz",
			wantErr: true,
		},
		{
			name:    "missing registry host is rejected",
			input:   "node:1",
			wantErr: true,
		},
		{
			name:    "empty reference is rejected",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := ParseOCIRef(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ref.Registry != tt.registry {
				t.Errorf("registry = %q, want %q", ref.Registry, tt.registry)
			}
			if ref.Repository != tt.repository {
				t.Errorf("repository = %q, want %q", ref.Repository, tt.repository)
			}
			if ref.Tag != tt.tag {
				t.Errorf("tag = %q, want %q", ref.Tag, tt.tag)
			}
			if ref.Raw != tt.input {
				t.Errorf("raw = %q, want %q", ref.Raw, tt.input)
			}
		})
	}
}

func TestOCIRefName(t *testing.T) {
	ref := &OCIRef{Registry: "ghcr.io", Repository: "devcontainers/features/node", Tag: "1"}
	if got, want := ref.Name(), "ghcr.io/devcontainers/features/node:1"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestOCIRefSanitizedID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ghcr.io/devcontainers/features/node:1", "ghcr.io-devcontainers-features-node-1"},
		{"localhost:5000/myfeature:2", "localhost-5000-myfeature-2"},
	}
	for _, tt := range tests {
		ref, err := ParseOCIRef(tt.input)
		if err != nil {
			t.Fatalf("ParseOCIRef(%q) error: %v", tt.input, err)
		}
		if got := ref.SanitizedID(); got != tt.want {
			t.Errorf("SanitizedID() for %q = %q, want %q", tt.input, got, tt.want)
		}
	}
}
