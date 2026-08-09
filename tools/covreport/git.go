package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// runGit runs git and returns its stdout, folding stderr into the error so a
// bad revision or a non-repository working directory surfaces git's own
// message rather than a bare exit status.
func runGit(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}
	return stdout, nil
}

// gitDiff produces the unified diff parseDiffBytes expects. An empty head
// diffs base against the working tree.
//
// The prefixes are set explicitly because a user with diff.noprefix=true would
// otherwise get "+++ pkg/x.go", and stripping "b/" would silently yield a path
// that never matches the coverage profile.
func gitDiff(base, head string) ([]byte, error) {
	args := []string{"diff", "-U0", "--no-color", "--no-ext-diff",
		"--src-prefix=a/", "--dst-prefix=b/", base}
	if head != "" {
		args = append(args, head)
	}
	return runGit(args...)
}

// gitShow returns a file's contents at revision. relPath is
// repository-relative, so covreport must run from the repository root, as
// renderHTML already requires.
func gitShow(revision, relPath string) ([]byte, error) {
	return runGit("show", revision+":"+relPath)
}
