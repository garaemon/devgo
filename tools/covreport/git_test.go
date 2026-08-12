package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// chdir switches to dir for the duration of the test. testing.T.Chdir would
// do this, but it landed in Go 1.24 and CI still builds on 1.21.
func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("reading the working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("entering %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("returning to %s: %v", previous, err)
		}
	})
}

// setupRepo builds a repository whose second commit changes a file, and
// returns its path. When branchName is not empty a branch of that name is
// created pointing at the first commit, which is what makes a name that is
// both a revision and a path ambiguous to git.
func setupRepo(t *testing.T, fileName, branchName string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", ".")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")

	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", fileName, err)
	}
	run("add", ".")
	run("commit", "-qm", "first")
	if branchName != "" {
		run("branch", branchName)
	}
	if err := os.WriteFile(path, []byte("first\nsecond\n"), 0o600); err != nil {
		t.Fatalf("rewriting %s: %v", fileName, err)
	}
	run("add", ".")
	run("commit", "-qm", "second")
	return dir
}

// A base whose name is also a path in the tree is ambiguous to git unless the
// revisions are terminated with "--". Branch names like cmd, test and tools
// all collide with directories in this repository.
func TestGitDiffAcceptsRefMatchingAPath(t *testing.T) {
	chdir(t, setupRepo(t, "release", "release"))

	out, err := gitDiff("release", "")
	if err != nil {
		t.Fatalf("gitDiff on a name that is both a revision and a path: %v", err)
	}
	if !strings.Contains(string(out), "+++ b/release") {
		t.Errorf("diff should name the changed file on the new side:\n%s", out)
	}
}

func TestGitDiffWithExplicitHead(t *testing.T) {
	chdir(t, setupRepo(t, "app.go", ""))

	out, err := gitDiff("HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("gitDiff returned error: %v", err)
	}
	added, err := parseDiffBytes(out, "test")
	if err != nil {
		t.Fatalf("parseDiffBytes on gitDiff output: %v", err)
	}
	if want := []int{2}; len(added["app.go"]) != 1 || added["app.go"][0] != want[0] {
		t.Errorf("added lines = %v, want %v", added["app.go"], want)
	}
}

func TestGitDiffUnknownRevisionReportsGitsMessage(t *testing.T) {
	chdir(t, setupRepo(t, "app.go", ""))

	_, err := gitDiff("no-such-revision", "")
	if err == nil {
		t.Fatal("expected an error for an unknown revision")
	}
	// runGit exists to fold stderr into the error; a bare exit status would
	// leave the caller with nothing to act on.
	if !strings.Contains(err.Error(), "no-such-revision") {
		t.Errorf("error %q should carry git's own message", err)
	}
	if strings.Contains(err.Error(), "exit status") {
		t.Errorf("error %q is the bare exit status, not git's message", err)
	}
}

func TestGitShowReadsAFileAtARevision(t *testing.T) {
	chdir(t, setupRepo(t, "app.go", ""))

	atHead, err := gitShow("HEAD", "app.go")
	if err != nil {
		t.Fatalf("gitShow returned error: %v", err)
	}
	if got := string(atHead); got != "first\nsecond\n" {
		t.Errorf("HEAD contents = %q", got)
	}

	atParent, err := gitShow("HEAD~1", "app.go")
	if err != nil {
		t.Fatalf("gitShow on the parent commit: %v", err)
	}
	if got := string(atParent); got != "first\n" {
		t.Errorf("HEAD~1 contents = %q", got)
	}

	if _, err := gitShow("HEAD", "absent.go"); err == nil {
		t.Error("expected an error for a path absent at that revision")
	}
}
