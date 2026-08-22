package features

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// tarEntry describes a single entry to write into a test tarball.
type tarEntry struct {
	name     string
	typeflag byte
	mode     int64
	body     string
	linkname string
}

// buildTar renders the entries as an uncompressed tar stream.
func buildTar(t *testing.T, entries []tarEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Mode:     mode,
			Size:     int64(len(e.body)),
			Linkname: e.linkname,
		}
		if e.typeflag != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write tar header for %q: %v", e.name, err)
		}
		if e.typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("failed to write tar body for %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	return buf.Bytes()
}

func TestExtractTar(t *testing.T) {
	data := buildTar(t, []tarEntry{
		{name: "./", typeflag: tar.TypeDir, mode: 0o755},
		{name: "devcontainer-feature.json", typeflag: tar.TypeReg, mode: 0o644, body: `{"id":"node"}`},
		{name: "install.sh", typeflag: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\necho hi\n"},
		{name: "bin", typeflag: tar.TypeDir, mode: 0o755},
		{name: "bin/helper.sh", typeflag: tar.TypeReg, mode: 0o755, body: "helper\n"},
		{name: "docs/nested/notes.md", typeflag: tar.TypeReg, mode: 0o644, body: "notes\n"},
	})

	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(data), dest); err != nil {
		t.Fatalf("extractTar returned an unexpected error: %v", err)
	}

	wantFiles := map[string]string{
		"devcontainer-feature.json": `{"id":"node"}`,
		"install.sh":                "#!/bin/sh\necho hi\n",
		"bin/helper.sh":             "helper\n",
		"docs/nested/notes.md":      "notes\n",
	}
	for name, want := range wantFiles {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("failed to read extracted file %q: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%q content = %q, want %q", name, got, want)
		}
	}

	// Parent directories are created even when the tar has no explicit entry
	// for them (docs/ and docs/nested/ are implied by docs/nested/notes.md).
	for _, dir := range []string{"bin", "docs", "docs/nested"} {
		info, err := os.Stat(filepath.Join(dest, filepath.FromSlash(dir)))
		if err != nil {
			t.Errorf("expected directory %q to exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", dir)
		}
	}

	// The "./" entry must not create a stray file or directory named ".".
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("failed to read destination directory: %v", err)
	}
	if len(entries) != 4 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("destination has %d top-level entries (%v), want 4", len(entries), names)
	}
}

func TestExtractTarPreservesExecutableBit(t *testing.T) {
	data := buildTar(t, []tarEntry{
		{name: "install.sh", typeflag: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\n"},
	})

	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(data), dest); err != nil {
		t.Fatalf("extractTar returned an unexpected error: %v", err)
	}

	info, err := os.Stat(filepath.Join(dest, "install.sh"))
	if err != nil {
		t.Fatalf("failed to stat extracted file: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("install.sh mode = %v, want the owner executable bit set", info.Mode().Perm())
	}
}

func TestExtractTarRejectsUnsafePaths(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{name: "parent traversal", entry: "../escaped.txt"},
		{name: "traversal after a directory", entry: "feature/../../escaped.txt"},
		{name: "deep traversal", entry: "a/b/../../../../escaped.txt"},
		{name: "absolute path", entry: "/tmp/escaped.txt"},
		{name: "bare parent directory", entry: ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := buildTar(t, []tarEntry{
				{name: tt.entry, typeflag: tar.TypeReg, mode: 0o644, body: "pwned"},
			})

			parent := t.TempDir()
			dest := filepath.Join(parent, "feature")
			if err := os.MkdirAll(dest, 0o755); err != nil {
				t.Fatalf("failed to create destination directory: %v", err)
			}

			err := extractTar(bytes.NewReader(data), dest)
			if err == nil {
				t.Fatalf("extractTar accepted unsafe path %q", tt.entry)
			}
			if !strings.Contains(err.Error(), "unsafe path") {
				t.Errorf("error = %q, want it to mention an unsafe path", err)
			}

			// Nothing may be written outside (or inside) the destination.
			if _, statErr := os.Stat(filepath.Join(parent, "escaped.txt")); !os.IsNotExist(statErr) {
				t.Errorf("entry %q escaped the destination directory", tt.entry)
			}
			entries, readErr := os.ReadDir(dest)
			if readErr != nil {
				t.Fatalf("failed to read destination directory: %v", readErr)
			}
			if len(entries) != 0 {
				t.Errorf("destination is not empty after rejecting %q", tt.entry)
			}
		})
	}
}

func TestExtractTarSkipsNonRegularEntries(t *testing.T) {
	data := buildTar(t, []tarEntry{
		{name: "install.sh", typeflag: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\n"},
		{name: "link.sh", typeflag: tar.TypeSymlink, mode: 0o777, linkname: "install.sh"},
		{name: "escape", typeflag: tar.TypeSymlink, mode: 0o777, linkname: "/etc/passwd"},
		{name: "hard.sh", typeflag: tar.TypeLink, mode: 0o644, linkname: "install.sh"},
		{name: "fifo", typeflag: tar.TypeFifo, mode: 0o644},
		{name: "after.txt", typeflag: tar.TypeReg, mode: 0o644, body: "still extracted\n"},
	})

	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(data), dest); err != nil {
		t.Fatalf("extractTar returned an unexpected error: %v", err)
	}

	// Symlinks, hard links and devices are skipped silently rather than failing
	// the extraction.
	for _, skipped := range []string{"link.sh", "escape", "hard.sh", "fifo"} {
		if _, err := os.Lstat(filepath.Join(dest, skipped)); !os.IsNotExist(err) {
			t.Errorf("expected %q to be skipped, but it exists", skipped)
		}
	}

	// Entries following a skipped one are still extracted.
	got, err := os.ReadFile(filepath.Join(dest, "after.txt"))
	if err != nil {
		t.Fatalf("failed to read file written after a skipped entry: %v", err)
	}
	if string(got) != "still extracted\n" {
		t.Errorf("after.txt content = %q, want %q", got, "still extracted\n")
	}
}

func TestExtractTarEmptyArchive(t *testing.T) {
	dest := t.TempDir()
	if err := extractTar(bytes.NewReader(buildTar(t, nil)), dest); err != nil {
		t.Fatalf("extractTar returned an unexpected error: %v", err)
	}

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("failed to read destination directory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("destination has %d entries, want 0", len(entries))
	}
}

func TestExtractTarTruncatedStream(t *testing.T) {
	// The body is large enough that cutting the stream lands inside the file
	// contents rather than on a clean 512-byte block boundary.
	data := buildTar(t, []tarEntry{
		{name: "install.sh", typeflag: tar.TypeReg, mode: 0o755, body: strings.Repeat("x", 4096)},
	})

	dest := t.TempDir()
	err := extractTar(bytes.NewReader(data[:1000]), dest)
	if err == nil {
		t.Fatal("extractTar accepted a truncated tar stream")
	}
}

func TestIsUnsafePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "plain file", path: "install.sh", want: false},
		{name: "nested file", path: filepath.Join("bin", "helper.sh"), want: false},
		{name: "dot prefixed file", path: ".devcontainer", want: false},
		{name: "name starting with two dots", path: "..hidden", want: false},
		{name: "parent directory", path: "..", want: true},
		{name: "parent traversal", path: filepath.Join("..", "escaped.txt"), want: true},
		{name: "absolute path", path: string(os.PathSeparator) + "etc", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnsafePath(tt.path); got != tt.want {
				t.Errorf("isUnsafePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// stubImage implements just enough of v1.Image for pickFeatureLayer. The
// embedded interface satisfies the remaining methods, which are never called.
type stubImage struct {
	v1.Image
	layers []v1.Layer
	err    error
}

func (s stubImage) Layers() ([]v1.Layer, error) { return s.layers, s.err }

func mustDigest(t *testing.T, layer v1.Layer) v1.Hash {
	t.Helper()
	h, err := layer.Digest()
	if err != nil {
		t.Fatalf("failed to compute layer digest: %v", err)
	}
	return h
}

func TestPickFeatureLayerPrefersDevcontainerMediaType(t *testing.T) {
	other := static.NewLayer([]byte("other"), types.DockerLayer)
	want := static.NewLayer([]byte("feature"), devcontainerLayerMediaType)
	trailing := static.NewLayer([]byte("trailing"), types.DockerLayer)

	got, err := pickFeatureLayer(stubImage{layers: []v1.Layer{other, want, trailing}})
	if err != nil {
		t.Fatalf("pickFeatureLayer returned an unexpected error: %v", err)
	}
	if mustDigest(t, got) != mustDigest(t, want) {
		t.Error("pickFeatureLayer did not select the devcontainer layer")
	}
}

func TestPickFeatureLayerFallsBackToLastLayer(t *testing.T) {
	first := static.NewLayer([]byte("first"), types.DockerLayer)
	last := static.NewLayer([]byte("last"), types.OCILayer)

	got, err := pickFeatureLayer(stubImage{layers: []v1.Layer{first, last}})
	if err != nil {
		t.Fatalf("pickFeatureLayer returned an unexpected error: %v", err)
	}
	if mustDigest(t, got) != mustDigest(t, last) {
		t.Error("pickFeatureLayer did not fall back to the last layer")
	}
}

func TestPickFeatureLayerNoLayers(t *testing.T) {
	_, err := pickFeatureLayer(stubImage{layers: nil})
	if err == nil {
		t.Fatal("pickFeatureLayer accepted an artifact with no layers")
	}
	if !strings.Contains(err.Error(), "no layers") {
		t.Errorf("error = %q, want it to mention missing layers", err)
	}
}

func TestPickFeatureLayerPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := pickFeatureLayer(stubImage{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap %v", err, wantErr)
	}
}

// TestPickFeatureLayerExtractRoundTrip covers the seam between layer selection
// and extraction: the selected layer's uncompressed stream is what extractTar
// consumes.
func TestPickFeatureLayerExtractRoundTrip(t *testing.T) {
	data := buildTar(t, []tarEntry{
		{name: "devcontainer-feature.json", typeflag: tar.TypeReg, mode: 0o644, body: `{"id":"node"}`},
		{name: "install.sh", typeflag: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\n"},
	})
	img := stubImage{layers: []v1.Layer{
		static.NewLayer([]byte("config"), types.DockerConfigJSON),
		static.NewLayer(data, devcontainerLayerMediaType),
	}}

	layer, err := pickFeatureLayer(img)
	if err != nil {
		t.Fatalf("pickFeatureLayer returned an unexpected error: %v", err)
	}
	rc, err := layer.Uncompressed()
	if err != nil {
		t.Fatalf("failed to open the layer: %v", err)
	}
	defer func() { _ = rc.Close() }()

	dest := t.TempDir()
	if err := extractTar(rc, dest); err != nil {
		t.Fatalf("extractTar returned an unexpected error: %v", err)
	}

	meta, err := os.ReadFile(filepath.Join(dest, "devcontainer-feature.json"))
	if err != nil {
		t.Fatalf("failed to read the extracted metadata: %v", err)
	}
	parsed, err := ParseFeatureMetadata(meta)
	if err != nil {
		t.Fatalf("failed to parse the extracted metadata: %v", err)
	}
	if parsed.ID != "node" {
		t.Errorf("parsed feature id = %q, want %q", parsed.ID, "node")
	}
}

func TestWriteFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "install.sh")

	if err := writeFile(target, strings.NewReader("first\n"), 0o644); err != nil {
		t.Fatalf("writeFile returned an unexpected error: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read the written file: %v", err)
	}
	if string(got) != "first\n" {
		t.Errorf("content = %q, want %q", got, "first\n")
	}

	// A second write truncates rather than appending to the existing contents.
	if err := writeFile(target, strings.NewReader("second\n"), 0o644); err != nil {
		t.Fatalf("writeFile returned an unexpected error on rewrite: %v", err)
	}
	got, err = os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read the rewritten file: %v", err)
	}
	if string(got) != "second\n" {
		t.Errorf("content after rewrite = %q, want %q", got, "second\n")
	}
}

func TestWriteFileForcesOwnerReadWrite(t *testing.T) {
	target := filepath.Join(t.TempDir(), "readonly-in-tar")

	// Feature tarballs occasionally carry modes that would leave the extracted
	// file unwritable; writeFile ORs in 0600 so the build context stays usable.
	if err := writeFile(target, strings.NewReader("body\n"), 0o400); err != nil {
		t.Fatalf("writeFile returned an unexpected error: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("failed to stat the written file: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o600 != 0o600 {
		t.Errorf("mode = %v, want at least owner read/write", perm)
	}
}

func TestWriteFileMissingParentDirectory(t *testing.T) {
	target := filepath.Join(t.TempDir(), "missing", "install.sh")

	err := writeFile(target, strings.NewReader("body\n"), 0o644)
	if err == nil {
		t.Fatal("writeFile succeeded despite a missing parent directory")
	}
}

// errReader fails partway through, standing in for a registry connection that
// drops mid-layer.
type errReader struct {
	remaining int
	err       error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, r.err
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	return n, nil
}

func TestWriteFilePropagatesReadError(t *testing.T) {
	target := filepath.Join(t.TempDir(), "install.sh")
	wantErr := errors.New("connection reset")

	err := writeFile(target, &errReader{remaining: 8, err: wantErr}, 0o644)
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap %v", err, wantErr)
	}
}

// startTestRegistry runs an in-memory OCI registry and returns its host:port.
// "localhost" is used rather than the loopback address so that
// go-containerregistry talks plain HTTP to it.
func startTestRegistry(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	_, port, err := net.SplitHostPort(host)
	if err != nil {
		t.Fatalf("failed to parse the test registry address %q: %v", host, err)
	}
	return net.JoinHostPort("localhost", port)
}

// pushTestFeature publishes a single-layer feature artifact and returns its
// reference.
func pushTestFeature(t *testing.T, host, repo, tag string, entries []tarEntry) string {
	t.Helper()

	layer := static.NewLayer(buildTar(t, entries), devcontainerLayerMediaType)
	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		t.Fatalf("failed to build the test feature image: %v", err)
	}

	ref := host + "/" + repo + ":" + tag
	if err := crane.Push(img, ref); err != nil {
		t.Fatalf("failed to push the test feature to %q: %v", ref, err)
	}
	return ref
}

func TestPull(t *testing.T) {
	host := startTestRegistry(t)
	ref := pushTestFeature(t, host, "devcontainers/features/node", "1", []tarEntry{
		{
			name:     "devcontainer-feature.json",
			typeflag: tar.TypeReg,
			mode:     0o644,
			body:     `{"id":"node","version":"1.2.3","containerEnv":{"NVM_DIR":"/usr/local/share/nvm"}}`,
		},
		{name: "install.sh", typeflag: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\necho installing\n"},
		{name: "bin/helper.sh", typeflag: tar.TypeReg, mode: 0o755, body: "helper\n"},
	})

	parsed, err := ParseOCIRef(ref)
	if err != nil {
		t.Fatalf("failed to parse the test reference %q: %v", ref, err)
	}

	dest := t.TempDir()
	pf, err := Pull(parsed, dest)
	if err != nil {
		t.Fatalf("Pull returned an unexpected error: %v", err)
	}

	// The feature lands in its own directory, named after the sanitized ref.
	wantDir := filepath.Join(dest, parsed.SanitizedID())
	if pf.Dir != wantDir {
		t.Errorf("Dir = %q, want %q", pf.Dir, wantDir)
	}
	if pf.Ref != parsed {
		t.Error("Ref does not point at the reference that was passed in")
	}

	if pf.Metadata.ID != "node" {
		t.Errorf("Metadata.ID = %q, want %q", pf.Metadata.ID, "node")
	}
	if pf.Metadata.Version != "1.2.3" {
		t.Errorf("Metadata.Version = %q, want %q", pf.Metadata.Version, "1.2.3")
	}
	if got := pf.Metadata.ContainerEnv["NVM_DIR"]; got != "/usr/local/share/nvm" {
		t.Errorf("ContainerEnv[NVM_DIR] = %q, want %q", got, "/usr/local/share/nvm")
	}

	// The whole layer is extracted, not just the metadata file.
	for _, name := range []string{"install.sh", filepath.Join("bin", "helper.sh")} {
		if _, err := os.Stat(filepath.Join(pf.Dir, name)); err != nil {
			t.Errorf("expected %q to be extracted: %v", name, err)
		}
	}
}

func TestPullMissingMetadata(t *testing.T) {
	host := startTestRegistry(t)
	ref := pushTestFeature(t, host, "devcontainers/features/broken", "1", []tarEntry{
		{name: "install.sh", typeflag: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\n"},
	})

	parsed, err := ParseOCIRef(ref)
	if err != nil {
		t.Fatalf("failed to parse the test reference %q: %v", ref, err)
	}

	_, err = Pull(parsed, t.TempDir())
	if err == nil {
		t.Fatal("Pull accepted a feature without devcontainer-feature.json")
	}
	if !strings.Contains(err.Error(), "devcontainer-feature.json") {
		t.Errorf("error = %q, want it to name the missing metadata file", err)
	}
}

func TestPullInvalidMetadata(t *testing.T) {
	host := startTestRegistry(t)
	ref := pushTestFeature(t, host, "devcontainers/features/malformed", "1", []tarEntry{
		{name: "devcontainer-feature.json", typeflag: tar.TypeReg, mode: 0o644, body: "{not json"},
	})

	parsed, err := ParseOCIRef(ref)
	if err != nil {
		t.Fatalf("failed to parse the test reference %q: %v", ref, err)
	}

	_, err = Pull(parsed, t.TempDir())
	if err == nil {
		t.Fatal("Pull accepted malformed feature metadata")
	}
}

func TestPullUnknownFeature(t *testing.T) {
	host := startTestRegistry(t)

	parsed, err := ParseOCIRef(host + "/devcontainers/features/absent:1")
	if err != nil {
		t.Fatalf("failed to parse the test reference: %v", err)
	}

	_, err = Pull(parsed, t.TempDir())
	if err == nil {
		t.Fatal("Pull succeeded for a feature that was never pushed")
	}
	if !strings.Contains(err.Error(), "failed to pull feature") {
		t.Errorf("error = %q, want it to mention the failed pull", err)
	}
}
