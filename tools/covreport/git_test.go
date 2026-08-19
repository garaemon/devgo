package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// runGitFor runs git in dir with a fixed identity, so the tests do not depend
// on the developer's global git configuration.
func runGitFor(t *testing.T, dir string, args ...string) {
	t.Helper()
	fixed := []string{"-C", dir,
		"-c", "user.email=covreport@example.com",
		"-c", "user.name=covreport test",
		"-c", "commit.gpgsign=false",
	}
	cmd := exec.Command("git", append(fixed, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeFileFor(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("cannot create %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("cannot write %s: %v", full, err)
	}
}

// newRepoWithChanges commits two Go files, then edits them in the working
// tree, and makes that repository the process working directory. runGit has
// no directory argument because covreport always runs from the repository
// root.
func newRepoWithChanges(t *testing.T) {
	t.Helper()
	// devgo is built for container work, where a minimal image may carry no
	// git at all. A missing tool is not a failing wrapper.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := chdirTemp(t)
	runGitFor(t, dir, "init", "--quiet")
	writeFileFor(t, dir, "pkg/plain.go", "package pkg\n\nfunc First() {\n}\n")
	writeFileFor(t, dir, "pkg/café.go", "package pkg\n\nfunc Second() {\n}\n")
	runGitFor(t, dir, "add", "-A")
	runGitFor(t, dir, "commit", "--quiet", "-m", "initial")
	writeFileFor(t, dir, "pkg/plain.go", "package pkg\n\nfunc First() {\n\tprintln(1)\n}\n")
	writeFileFor(t, dir, "pkg/café.go", "package pkg\n\nfunc Second() {\n\tprintln(2)\n}\n")
}

func TestRunGitDiffAgainstWorkingTree(t *testing.T) {
	newRepoWithChanges(t)

	out, err := runGitDiff("HEAD", "")
	if err != nil {
		t.Fatalf("runGitDiff returned error: %v", err)
	}
	diff := string(out)

	// The "b/" prefix is what parseDiffBytes strips to recover the path.
	if !strings.Contains(diff, "+++ b/pkg/plain.go") {
		t.Errorf("diff should name the new side with a b/ prefix\n---\n%s", diff)
	}
	// -U0 keeps the hunks to the changed lines alone.
	if strings.Contains(diff, "\n func First() {") {
		t.Errorf("diff should carry no context lines\n---\n%s", diff)
	}

	added, err := parseDiffBytes(out, "test")
	if err != nil {
		t.Fatalf("parseDiffBytes returned error: %v", err)
	}
	if want := []int{4}; !reflect.DeepEqual(added["pkg/plain.go"], want) {
		t.Errorf("added lines = %v, want %v", added["pkg/plain.go"], want)
	}
}

func TestRunGitDiffKeepsNonASCIIPathsUsable(t *testing.T) {
	newRepoWithChanges(t)

	out, err := runGitDiff("HEAD", "")
	if err != nil {
		t.Fatalf("runGitDiff returned error: %v", err)
	}
	// Without core.quotePath=false git writes "b/pkg/caf\303\251.go", which
	// matches no coverage profile entry and drops the file from the report.
	if !strings.Contains(string(out), "+++ b/pkg/café.go") {
		t.Errorf("non-ASCII path should stay unquoted\n---\n%s", out)
	}
	added, err := parseDiffBytes(out, "test")
	if err != nil {
		t.Fatalf("parseDiffBytes returned error: %v", err)
	}
	if _, ok := added["pkg/café.go"]; !ok {
		t.Errorf("non-ASCII path missing from the added lines: %v", added)
	}
}

func TestRunGitDiffOverridesLocalPrefixSetting(t *testing.T) {
	newRepoWithChanges(t)
	runGitFor(t, ".", "config", "diff.noprefix", "true")

	out, err := runGitDiff("HEAD", "")
	if err != nil {
		t.Fatalf("runGitDiff returned error: %v", err)
	}
	if !strings.Contains(string(out), "+++ b/pkg/plain.go") {
		t.Errorf("diff.noprefix must not reach the output\n---\n%s", out)
	}
}

func TestRunGitShowReadsCommittedContent(t *testing.T) {
	newRepoWithChanges(t)

	out, err := runGitShow("HEAD", "pkg/plain.go")
	if err != nil {
		t.Fatalf("runGitShow returned error: %v", err)
	}
	// The working tree has the println line; HEAD does not.
	if strings.Contains(string(out), "println") {
		t.Errorf("runGitShow returned the working tree, not HEAD\n---\n%s", out)
	}
	if !strings.Contains(string(out), "func First()") {
		t.Errorf("runGitShow returned unexpected content\n---\n%s", out)
	}
}

func TestRunGitReportsStderr(t *testing.T) {
	newRepoWithChanges(t)

	_, err := runGitShow("no-such-revision", "pkg/plain.go")
	if err == nil {
		t.Fatal("expected an error for an unknown revision")
	}
	// A bare exit status would leave the user guessing.
	if !strings.Contains(err.Error(), "no-such-revision") {
		t.Errorf("error %q should carry git's own message", err)
	}
}

func TestLoadAddedLinesFromRevision(t *testing.T) {
	newRepoWithChanges(t)

	added, haveDiff, err := loadAddedLines("", "HEAD", "")
	if err != nil {
		t.Fatalf("loadAddedLines returned error: %v", err)
	}
	if !haveDiff {
		t.Error("a -diff-base was supplied, so haveDiff must be true")
	}
	if want := []int{4}; !reflect.DeepEqual(added["pkg/plain.go"], want) {
		t.Errorf("added lines = %v, want %v", added["pkg/plain.go"], want)
	}
}

func TestLoadAddedLinesReportsBadRevision(t *testing.T) {
	newRepoWithChanges(t)

	_, _, err := loadAddedLines("", "no-such-revision", "")
	if err == nil {
		t.Fatal("expected an error for an unknown base revision")
	}
}
