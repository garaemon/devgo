package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeTempFile returns the path of a file holding content, for the parsers
// that take a path rather than bytes.
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("cannot write %s: %v", p, err)
	}
	return p
}

// chdirTemp makes a fresh directory the working directory for one test.
// covreport reads go.mod and sources relative to it, so the wiring can only
// be exercised from inside a directory the test controls.
//
// The working directory is process-wide, so a test that calls this must not
// declare t.Parallel(). (testing.T.Chdir would carry the same constraint and
// needs a newer go directive than go.mod names.)
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot read the working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("cannot enter %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("cannot return to %s: %v", previous, err)
		}
	})
	return dir
}

func TestReadModulePath(t *testing.T) {
	path := writeTempFile(t, "go.mod", "module example.com/m\n\ngo 1.23.0\n")
	got, err := readModulePath(path)
	if err != nil {
		t.Fatalf("readModulePath returned error: %v", err)
	}
	if got != "example.com/m" {
		t.Errorf("readModulePath = %q, want example.com/m", got)
	}
}

func TestReadModulePathWithoutDirective(t *testing.T) {
	path := writeTempFile(t, "go.mod", "go 1.23.0\n")
	if _, err := readModulePath(path); err == nil {
		t.Fatal("expected an error when go.mod declares no module")
	}
}

// parseFlagsFor runs parseFlags against args, restoring the process-wide flag
// state afterwards so the tests stay independent of each other.
func parseFlagsFor(t *testing.T, args ...string) (*config, error) {
	t.Helper()
	oldArgs, oldFlags := os.Args, flag.CommandLine
	t.Cleanup(func() { os.Args, flag.CommandLine = oldArgs, oldFlags })
	flag.CommandLine = flag.NewFlagSet("covreport", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = append([]string{"covreport"}, args...)
	return parseFlags()
}

func TestParseFlagsRejectsUnusableCombinations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"cover is required", []string{}},
		{"unknown format", []string{"-cover", "c.out", "-format", "json"}},
		{"base-cover with html", []string{"-cover", "c.out", "-format", "html",
			"-base-cover", "b.out"}},
		{"base-name with html", []string{"-cover", "c.out", "-format", "html",
			"-base-name", "main"}},
		{"base-rev with html", []string{"-cover", "c.out", "-format", "html",
			"-base-rev", "main"}},
		{"diff with diff-base", []string{"-cover", "c.out", "-diff", "p.diff",
			"-diff-base", "main"}},
		{"diff-head without diff-base", []string{"-cover", "c.out", "-diff-head", "HEAD"}},
		// `make coverage-diff BASE=` expands to an empty -diff-base, which
		// used to render the full listing and exit 0.
		{"empty diff-base", []string{"-cover", "c.out", "-format", "html", "-diff-base", ""}},
		{"empty cover", []string{"-cover", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseFlagsFor(t, tt.args...); err == nil {
				t.Errorf("parseFlags(%v) returned no error", tt.args)
			}
		})
	}
}

func TestParseFlagsAcceptsHTMLWithDiffBase(t *testing.T) {
	cfg, err := parseFlagsFor(t, "-cover", "c.out", "-format", "html", "-diff-base", "main")
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if cfg.diffContext != 3 {
		t.Errorf("diffContext = %d, want the documented default of 3", cfg.diffContext)
	}
}

func TestParseFlagsKeepsBaseRevResolvable(t *testing.T) {
	cfg, err := parseFlagsFor(t, "-cover", "c.out", "-base-name", "main@1a2b3c4")
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	// The display label must not leak into the revision the report tells
	// readers to pass to `make coverage-diff BASE=`.
	if cfg.baseRev != "main" {
		t.Errorf("baseRev = %q, want main", cfg.baseRev)
	}
}

func TestParseFlagsRejectsNegativeDiffContext(t *testing.T) {
	_, err := parseFlagsFor(t, "-cover", "c.out", "-format", "html", "-diff-context", "-1")
	if err == nil {
		t.Fatal("expected an error for a negative -diff-context")
	}
}

func TestLoadAddedLinesFromDiffFile(t *testing.T) {
	diff := writeTempFile(t, "pr.diff", `--- a/cmd/up.go
+++ b/cmd/up.go
@@ -10,0 +11,2 @@
+	a()
+	b()
`)
	added, haveDiff, err := loadAddedLines(diff, "", "")
	if err != nil {
		t.Fatalf("loadAddedLines returned error: %v", err)
	}
	if !haveDiff {
		t.Error("a -diff file was supplied, so haveDiff must be true")
	}
	if want := []int{11, 12}; !reflect.DeepEqual(added["cmd/up.go"], want) {
		t.Errorf("added lines = %v, want %v", added["cmd/up.go"], want)
	}
}

func TestLoadAddedLinesWithoutDiff(t *testing.T) {
	added, haveDiff, err := loadAddedLines("", "", "")
	if err != nil {
		t.Fatalf("loadAddedLines returned error: %v", err)
	}
	if haveDiff {
		t.Error("no diff was requested, so haveDiff must be false")
	}
	if added != nil {
		t.Errorf("added = %v, want nil", added)
	}
}

func TestRunReportWritesMarkdown(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/m\n\ngo 1.23.0\n"), 0o600); err != nil {
		t.Fatalf("cannot write go.mod: %v", err)
	}
	cover := writeTempFile(t, "coverage.out",
		"mode: set\nexample.com/m/cmd/a.go:1.1,2.2 2 1\n")

	var out bytes.Buffer
	// module is left empty on purpose: runReport must find it in go.mod.
	cfg := config{coverPath: cover, format: "markdown", baseName: "main", baseRev: "main"}
	if err := runReport(cfg, &out); err != nil {
		t.Fatalf("runReport returned error: %v", err)
	}
	if !strings.Contains(out.String(), "**Project:** 100.0%") {
		t.Errorf("markdown report missing the project line\n---\n%s", out.String())
	}
}

func TestRunReportWritesHTML(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o755); err != nil {
		t.Fatalf("cannot create cmd/: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "a.go"),
		[]byte("package cmd\n\nfunc A() {\n}\n"), 0o600); err != nil {
		t.Fatalf("cannot write cmd/a.go: %v", err)
	}
	cover := writeTempFile(t, "coverage.out",
		"mode: set\nexample.com/m/cmd/a.go:3.1,4.2 1 1\n")

	var out bytes.Buffer
	cfg := config{coverPath: cover, module: "example.com/m", format: "html", diffContext: 3}
	if err := runReport(cfg, &out); err != nil {
		t.Fatalf("runReport returned error: %v", err)
	}
	html := out.String()
	if !strings.Contains(html, "<title>example.com/m coverage</title>") {
		t.Errorf("html report missing its title\n---\n%s", html)
	}
	// The source is read from the working directory when no head revision
	// was named, so the file's own text must appear in the page.
	if !strings.Contains(html, "func A()") {
		t.Errorf("html report missing the annotated source\n---\n%s", html)
	}
}

func TestRunReportReportsMissingProfile(t *testing.T) {
	chdirTemp(t)
	cfg := config{coverPath: "absent.out", module: "example.com/m", format: "markdown"}
	if err := runReport(cfg, io.Discard); err == nil {
		t.Fatal("expected an error for a profile that does not exist")
	}
}
