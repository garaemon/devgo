package features

import (
	"reflect"
	"testing"
)

func TestOptionEnv(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"version", "VERSION"},
		{"installZsh", "INSTALLZSH"},
		{"node-version", "NODE_VERSION"},
		{"some.option", "SOME_OPTION"},
		{"1pass", "_PASS"},
		{"_leading", "_LEADING"},
	}
	for _, tt := range tests {
		if got := OptionEnv(tt.id); got != tt.want {
			t.Errorf("OptionEnv(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestResolveOptionValues(t *testing.T) {
	meta := &FeatureMetadata{
		Options: map[string]FeatureOption{
			"version":    {Type: "string", Default: "lts"},
			"installZsh": {Type: "boolean", Default: true},
		},
	}

	t.Run("defaults applied when no user options", func(t *testing.T) {
		got := meta.ResolveOptionValues(nil)
		want := map[string]string{"VERSION": "lts", "INSTALLZSH": "true"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("user options override defaults", func(t *testing.T) {
		got := meta.ResolveOptionValues(map[string]interface{}{
			"version":    "18",
			"installZsh": false,
		})
		want := map[string]string{"VERSION": "18", "INSTALLZSH": "false"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("integer option does not gain trailing .0", func(t *testing.T) {
		got := meta.ResolveOptionValues(map[string]interface{}{
			"version": float64(18),
		})
		if got["VERSION"] != "18" {
			t.Errorf("VERSION = %q, want %q", got["VERSION"], "18")
		}
	})

	t.Run("user option not declared is still passed", func(t *testing.T) {
		got := meta.ResolveOptionValues(map[string]interface{}{
			"extraFlag": true,
		})
		if got["EXTRAFLAG"] != "true" {
			t.Errorf("EXTRAFLAG = %q, want %q", got["EXTRAFLAG"], "true")
		}
	})
}

func TestParseFeatureMetadata(t *testing.T) {
	data := []byte(`{
		// devcontainer-feature.json with comments (json5)
		"id": "node",
		"version": "1.0.0",
		"options": {
			"version": { "type": "string", "default": "lts" },
		},
		"containerEnv": {
			"NVM_DIR": "/usr/local/share/nvm",
		},
	}`)

	meta, err := ParseFeatureMetadata(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.ID != "node" {
		t.Errorf("ID = %q, want %q", meta.ID, "node")
	}
	if meta.Options["version"].Default != "lts" {
		t.Errorf("default = %v, want lts", meta.Options["version"].Default)
	}
	if meta.ContainerEnv["NVM_DIR"] != "/usr/local/share/nvm" {
		t.Errorf("NVM_DIR = %q", meta.ContainerEnv["NVM_DIR"])
	}
}
