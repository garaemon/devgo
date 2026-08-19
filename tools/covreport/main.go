// Command covreport turns Go coverage profiles into a markdown coverage
// report for pull requests: overall ("project") coverage with an optional
// delta against a base profile, and "patch" coverage over the lines added
// by a diff. It replaces the Codecov project/patch statuses with a report
// built entirely from go test -coverprofile output.
//
// The work is split across profile.go (reading and aggregating profiles),
// diff.go (which lines a change added), markdown.go (the pull request
// comment), html.go (the annotated source browser) and git.go (the git
// commands the -diff-base mode runs).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// config is the parsed command line.
//
// baseName and baseRev are deliberately separate: baseName is a free-form
// label such as "main@1a2b3c4" that CI shows in the report, while baseRev
// must stay a revision git can resolve, because the report puts it in a
// command the reader is told to run.
type config struct {
	coverPath     string
	baseCoverPath string
	diffPath      string
	module        string
	commit        string
	baseName      string
	baseRev       string
	format        string
	diffBase      string
	diffHead      string
	diffContext   int
}

// A flag that one format ignores is rejected rather than dropped, so a caller
// never believes a flag took effect when it did not.
var (
	markdownOnlyFlags = []string{"base-cover", "base-name", "base-rev"}
	htmlOnlyFlags     = []string{"diff-context"}
)

func main() {
	cfg, err := parseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "covreport: %v\n", err)
		os.Exit(2)
	}
	if err := runReport(*cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "covreport: %v\n", err)
		os.Exit(1)
	}
}

// parseFlags reads the command line and rejects unusable flag combinations.
// Every error it returns is a usage error, which main reports with exit
// status 2.
func parseFlags() (*config, error) {
	cfg := &config{}
	flag.StringVar(&cfg.coverPath, "cover", "", "head coverage profile (required)")
	flag.StringVar(&cfg.baseCoverPath, "base-cover", "",
		"base coverage profile for the project delta (markdown only)")
	flag.StringVar(&cfg.diffPath, "diff", "", "unified diff (git diff -U0) for patch coverage")
	flag.StringVar(&cfg.module, "module", "", "module path (default: read from go.mod)")
	flag.StringVar(&cfg.commit, "commit", "", "head commit SHA to show in the report")
	flag.StringVar(&cfg.baseName, "base-name", "main",
		"display label for the base, e.g. main@1a2b3c4 (markdown only)")
	flag.StringVar(&cfg.baseRev, "base-rev", "main",
		"revision the report names in `make coverage-diff BASE=...`; must resolve in git")
	flag.StringVar(&cfg.format, "format", "markdown",
		"output format: markdown (PR report) or html (annotated source browser)")
	flag.StringVar(&cfg.diffBase, "diff-base", "",
		"base revision; runs `git diff -U0 <base> [<head>]` for patch coverage")
	flag.StringVar(&cfg.diffHead, "diff-head", "",
		"head revision for -diff-base (default: working tree); sources read via git show")
	flag.IntVar(&cfg.diffContext, "diff-context", 3,
		"context lines shown around changed lines in the html diff view")
	// Spelled out rather than flag.Parse (which is the same call) so that a
	// parse failure returns like every other usage error. flag.CommandLine is
	// ExitOnError, so in production the flag package exits first; the tests
	// swap in a ContinueOnError set and reach the return.
	if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	// `make coverage-diff BASE=` expands to -diff-base '', which would
	// otherwise read as "no diff requested" and render the full listing.
	if empty := findEmptyFlags(); len(empty) > 0 {
		return nil, fmt.Errorf("%s: needs a value", strings.Join(empty, ", "))
	}
	if cfg.coverPath == "" {
		return nil, errors.New("-cover is required")
	}
	if cfg.format != "markdown" && cfg.format != "html" {
		return nil, fmt.Errorf("unknown format %q", cfg.format)
	}
	if cfg.format == "html" {
		if ignored := findSetFlags(markdownOnlyFlags); len(ignored) > 0 {
			return nil, fmt.Errorf("%s: only used with -format markdown",
				strings.Join(ignored, ", "))
		}
	}
	if cfg.format == "markdown" {
		if ignored := findSetFlags(htmlOnlyFlags); len(ignored) > 0 {
			return nil, fmt.Errorf("%s: only used with -format html",
				strings.Join(ignored, ", "))
		}
	}
	if cfg.diffContext < 0 {
		return nil, fmt.Errorf("-diff-context must not be negative, got %d", cfg.diffContext)
	}
	if cfg.diffPath != "" && cfg.diffBase != "" {
		return nil, errors.New("cannot use -diff with -diff-base")
	}
	if cfg.diffHead != "" && cfg.diffBase == "" {
		return nil, errors.New("-diff-head requires -diff-base")
	}
	return cfg, nil
}

// findSetFlags returns the named flags the caller actually passed, each
// written as it appeared on the command line.
func findSetFlags(names []string) []string {
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	var found []string
	flag.Visit(func(f *flag.Flag) {
		if wanted[f.Name] {
			found = append(found, "-"+f.Name)
		}
	})
	return found
}

// findEmptyFlags returns the flags the caller passed with an empty value.
// No flag of this command has a meaningful empty value.
func findEmptyFlags() []string {
	var found []string
	flag.Visit(func(f *flag.Flag) {
		if f.Value.String() == "" {
			found = append(found, "-"+f.Name)
		}
	})
	return found
}

// runReport parses the profiles and the diff, then writes the requested
// report to out.
func runReport(cfg config, out io.Writer) error {
	if cfg.module == "" {
		modulePath, err := readModulePath("go.mod")
		if err != nil {
			return fmt.Errorf("cannot determine module path: %v", err)
		}
		cfg.module = modulePath
	}

	head, err := parseProfile(cfg.coverPath, cfg.module)
	if err != nil {
		return err
	}
	added, haveDiff, err := loadAddedLines(cfg.diffPath, cfg.diffBase, cfg.diffHead)
	if err != nil {
		return err
	}

	if cfg.format == "html" {
		return renderHTML(out, head, htmlOptions{
			Module:       cfg.module,
			Commit:       cfg.commit,
			Added:        added,
			HaveDiff:     haveDiff,
			DiffBase:     cfg.diffBase,
			DiffHead:     cfg.diffHead,
			DiffFile:     cfg.diffPath,
			ContextLines: cfg.diffContext,
			Source:       newSourceReader(cfg.diffHead),
		})
	}

	var base profile
	if cfg.baseCoverPath != "" {
		base, err = parseProfile(cfg.baseCoverPath, cfg.module)
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(out, renderMarkdown(head, base, added, haveDiff, cfg))
	return err
}

// loadAddedLines resolves the added-line map from either a pre-generated diff
// file (-diff) or a revision range (-diff-base/-diff-head). The bool reports
// whether a diff was requested at all, independently of whether reading it
// succeeded.
func loadAddedLines(diffPath, diffBase, diffHead string) (map[string][]int, bool, error) {
	switch {
	case diffPath != "":
		added, err := parseDiff(diffPath)
		return added, true, err
	case diffBase != "":
		out, err := runGitDiff(diffBase, diffHead)
		if err != nil {
			return nil, true, err
		}
		head := diffHead
		if head == "" {
			head = "(working tree)"
		}
		added, err := parseDiffBytes(out, fmt.Sprintf("git diff %s %s", diffBase, head))
		return added, true, err
	}
	return nil, false, nil
}

// newSourceReader picks where head sources are read from: the working
// directory when no head revision was named, otherwise that revision.
func newSourceReader(rev string) sourceReader {
	if rev == "" {
		return func(rel string) ([]byte, error) { return os.ReadFile(rel) }
	}
	return func(rel string) ([]byte, error) { return runGitShow(rev, rel) }
}

// readModulePath returns the module path declared in a go.mod file.
func readModulePath(goModPath string) (string, error) {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("no module directive in %s", goModPath)
}
