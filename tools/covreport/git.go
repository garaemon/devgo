// git.go wraps the git commands the -diff-base mode runs. Paths are
// repository-relative throughout, so covreport must run from the repository
// root.

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
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}

// runGitDiff produces the unified diff parseDiffBytes expects. An empty head
// diffs base against the working tree.
//
// The prefixes are set explicitly because a user with diff.noprefix=true would
// otherwise get "+++ pkg/x.go", and stripping "b/" would silently yield a path
// that never matches the coverage profile. For the same reason quotePath is
// turned off: git otherwise writes a non-ASCII path as "b/pkg/caf\303\251.go",
// which matches no profile entry and drops the file from patch coverage.
//
// --end-of-options keeps a revision that starts with "-" from being read as
// an option such as --output=<file>.
func runGitDiff(base, head string) ([]byte, error) {
	args := []string{"-c", "core.quotePath=false",
		"diff", "-U0", "--no-color", "--no-ext-diff",
		"--src-prefix=a/", "--dst-prefix=b/", "--end-of-options", base}
	if head != "" {
		args = append(args, head)
	}
	return runGit(args...)
}

// runGitShow returns a file's contents at rev. path is repository-relative, so
// covreport must run from the repository root, as renderHTML already requires.
func runGitShow(rev, path string) ([]byte, error) {
	return runGit("show", "--end-of-options", rev+":"+path)
}
