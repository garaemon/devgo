package features

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// devcontainerLayerMediaType is the media type of the tar layer carrying the
// feature files within an OCI feature artifact.
const devcontainerLayerMediaType = "application/vnd.devcontainers.layer.v1+tar"

// PulledFeature is a feature that has been downloaded and extracted locally.
type PulledFeature struct {
	Ref      *OCIRef
	Dir      string // directory containing the extracted feature files
	Metadata *FeatureMetadata
}

// Pull downloads the feature artifact referenced by ref from its OCI registry,
// extracts it into destBase/<SanitizedID>, and parses its metadata.
func Pull(ref *OCIRef, destBase string) (*PulledFeature, error) {
	img, err := crane.Pull(ref.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to pull feature %q: %w", ref.Raw, err)
	}

	layer, err := pickFeatureLayer(img)
	if err != nil {
		return nil, fmt.Errorf("failed to select feature layer for %q: %w", ref.Raw, err)
	}

	rc, err := layer.Uncompressed()
	if err != nil {
		return nil, fmt.Errorf("failed to read feature layer for %q: %w", ref.Raw, err)
	}
	defer func() { _ = rc.Close() }()

	destDir := filepath.Join(destBase, ref.SanitizedID())
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create feature directory: %w", err)
	}

	if err := extractTar(rc, destDir); err != nil {
		return nil, fmt.Errorf("failed to extract feature %q: %w", ref.Raw, err)
	}

	metaPath := filepath.Join(destDir, "devcontainer-feature.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("feature %q is missing devcontainer-feature.json: %w", ref.Raw, err)
	}
	meta, err := ParseFeatureMetadata(data)
	if err != nil {
		return nil, err
	}

	return &PulledFeature{Ref: ref, Dir: destDir, Metadata: meta}, nil
}

// pickFeatureLayer selects the tar layer that carries the feature files. It
// prefers a layer with the devcontainer layer media type, and otherwise falls
// back to the last layer (feature artifacts are typically single-layer).
func pickFeatureLayer(img v1.Image) (v1.Layer, error) {
	layers, err := img.Layers()
	if err != nil {
		return nil, err
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("feature artifact has no layers")
	}

	for _, layer := range layers {
		mt, err := layer.MediaType()
		if err != nil {
			continue
		}
		if string(mt) == devcontainerLayerMediaType {
			return layer, nil
		}
	}

	return layers[len(layers)-1], nil
}

// extractTar extracts a tar stream into destDir, rejecting entries that would
// escape the destination directory.
func extractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		name := filepath.Clean(hdr.Name)
		if name == "." {
			continue
		}
		if isUnsafePath(name) {
			return fmt.Errorf("refusing to extract unsafe path: %q", hdr.Name)
		}

		target := filepath.Join(destDir, name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFile(target, tr, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		default:
			// Skip symlinks, devices, and other entry types for safety.
		}
	}
}

// isUnsafePath reports whether a cleaned tar entry path would escape the
// destination directory.
func isUnsafePath(name string) bool {
	return filepath.IsAbs(name) ||
		name == ".." ||
		strings.HasPrefix(name, ".."+string(os.PathSeparator))
}

func writeFile(target string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	//nolint:gosec // feature artifacts come from trusted registries
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	return nil
}
