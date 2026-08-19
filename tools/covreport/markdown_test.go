package main

import (
	"fmt"
	"strings"
	"testing"
)

// newMarkdownFixture returns a profile whose head half is covered, so the
// rendered numbers are easy to read back out of the report.
func newMarkdownFixture(module string) (head, base profile, added map[string][]int) {
	head = profile{
		module + "/cmd/a.go": {
			{startLine: 10, endLine: 12, numStmts: 2, count: 1},
			{startLine: 20, endLine: 22, numStmts: 2, count: 0},
		},
	}
	base = profile{
		module + "/cmd/a.go": {{startLine: 10, endLine: 12, numStmts: 2, count: 1}},
	}
	return head, base, map[string][]int{"cmd/a.go": {11, 21}}
}

func TestRenderMarkdownWithBaseline(t *testing.T) {
	const module = "example.com/m"
	head, base, added := newMarkdownFixture(module)
	cfg := config{
		module:   module,
		commit:   "1a2b3c4",
		baseName: "main@9f8e7d6",
		baseRev:  "main",
	}

	out := renderMarkdown(head, base, added, true, cfg)

	for _, want := range []string{
		"<!-- devgo-coverage-report -->",
		"Commit: `1a2b3c4`",
		"**Project:** 50.0% (main@9f8e7d6: 100.0%, -50.0%)",
		"**Patch:** 50.0% (2/4 statements)",
		"| `cmd` | 50.0% | 100.0% | -50.0% |",
		"| `cmd/a.go` | 50.0% | 2/4 | 21 |",
		"make coverage-diff BASE=main`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "Baseline unavailable") {
		t.Error("the baseline was supplied, so the report must not say it is missing")
	}
}

func TestRenderMarkdownWithoutBaseline(t *testing.T) {
	const module = "example.com/m"
	head, _, _ := newMarkdownFixture(module)
	cfg := config{module: module, baseName: "main", baseRev: "main"}

	out := renderMarkdown(head, nil, nil, false, cfg)

	if !strings.Contains(out, "_Baseline unavailable — project delta omitted._") {
		t.Errorf("report should explain the missing baseline\n---\n%s", out)
	}
	if strings.Contains(out, "**Patch:**") {
		t.Error("no diff was supplied, so no patch coverage should be reported")
	}
	if strings.Contains(out, "Per-package coverage changes") {
		t.Error("without a baseline there is nothing to compare per package")
	}
}

func TestRenderMarkdownWithoutCoverableChanges(t *testing.T) {
	const module = "example.com/m"
	head, base, _ := newMarkdownFixture(module)
	cfg := config{module: module, baseName: "main", baseRev: "main"}

	out := renderMarkdown(head, base, map[string][]int{}, true, cfg)

	if !strings.Contains(out, "**Patch:** n/a (no coverable changes)") {
		t.Errorf("report should mark patch coverage as n/a\n---\n%s", out)
	}
	if strings.Contains(out, "Patch coverage by file") {
		t.Error("an empty patch needs no per-file table")
	}
}

func TestRenderMarkdownHidesUnchangedPackages(t *testing.T) {
	const module = "example.com/m"
	// 2/4 and 3/6 both read 50.0%, so the package has nothing to report.
	head := profile{
		module + "/cmd/a.go": {
			{startLine: 1, endLine: 2, numStmts: 3, count: 1},
			{startLine: 4, endLine: 5, numStmts: 3, count: 0},
		},
	}
	base := profile{
		module + "/cmd/a.go": {
			{startLine: 1, endLine: 2, numStmts: 2, count: 1},
			{startLine: 4, endLine: 5, numStmts: 2, count: 0},
		},
	}
	cfg := config{module: module, baseName: "main", baseRev: "main"}

	out := renderMarkdown(head, base, nil, false, cfg)

	if !strings.Contains(out, "_No per-package coverage changes._") {
		t.Errorf("a package that reads the same on both sides needs no row\n---\n%s", out)
	}
}

func TestRenderMarkdownEscapesFileNames(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/we|ird.go": {{startLine: 10, endLine: 12, numStmts: 2, count: 1}},
	}
	cfg := config{module: module, baseName: "main", baseRev: "main"}

	out := renderMarkdown(head, nil, map[string][]int{"cmd/we|ird.go": {11}}, true, cfg)

	// An unescaped "|" would end the column and shift every later cell.
	if !strings.Contains(out, "| `cmd/we\\|ird.go` | 100.0% |") {
		t.Errorf("file name should be escaped for a table cell\n---\n%s", out)
	}
}

func TestRenderMarkdownEscapesBaseName(t *testing.T) {
	const module = "example.com/m"
	head, base, _ := newMarkdownFixture(module)
	// Git allows both characters in a branch name, and the base branch name
	// reaches the report straight from the pull request.
	cfg := config{module: module, baseName: "feat|x`y", baseRev: "feat|x`y"}

	out := renderMarkdown(head, base, nil, false, cfg)

	if strings.Contains(out, "feat|x`y") {
		t.Errorf("the raw base name must not reach the comment\n---\n%s", out)
	}
	if !strings.Contains(out, "| `feat\\|x?y` |") {
		t.Errorf("the table header should hold the escaped name\n---\n%s", out)
	}
	if !strings.Contains(out, "make coverage-diff BASE=feat|x?y`") {
		t.Errorf("the footer command should hold the backtick-free name\n---\n%s", out)
	}
}

func TestEscapeCell(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"plain", "cmd/up.go", "`cmd/up.go`"},
		{"pipe", "cmd/a|b.go", "`cmd/a\\|b.go`"},
		// A backtick cannot survive inside a code span, so it is replaced
		// rather than dropped: the reader must see the name was altered.
		{"backtick", "cmd/a`b.go", "`cmd/a?b.go`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeCell(tt.value); got != tt.want {
				t.Errorf("escapeCell(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestRenderMarkdownCapsThePatchTable(t *testing.T) {
	const module = "example.com/m"
	head := profile{}
	added := map[string][]int{}
	for i := 0; i < maxPatchTableRows+5; i++ {
		file := fmt.Sprintf("cmd/file%02d.go", i)
		head[module+"/"+file] = []block{
			{startLine: 1, endLine: 2, numStmts: 1, count: 1},
		}
		added[file] = []int{1}
	}
	cfg := config{module: module, baseName: "main", baseRev: "main"}

	out := renderMarkdown(head, nil, added, true, cfg)

	// Every row is one line, so counting them is enough to see the cap.
	if got := strings.Count(out, "| `cmd/file"); got != maxPatchTableRows {
		t.Errorf("rendered %d rows, want %d", got, maxPatchTableRows)
	}
	if !strings.Contains(out, "_5 more file(s) omitted; run `make coverage-diff BASE=main`") {
		t.Errorf("the omitted files should be counted\n---\n%s", out)
	}
	// The patch percentage still covers every changed file, capped or not.
	if !strings.Contains(out, "**Patch:** 100.0% (55/55 statements)") {
		t.Errorf("the summary must count the omitted files too\n---\n%s", out)
	}
}

func TestRenderMarkdownMarksNewAndRemovedPackages(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go":       {{startLine: 1, endLine: 2, numStmts: 2, count: 1}},
		module + "/pkg/added/x.go": {{startLine: 1, endLine: 2, numStmts: 4, count: 1}},
	}
	base := profile{
		module + "/cmd/a.go":         {{startLine: 1, endLine: 2, numStmts: 2, count: 1}},
		module + "/pkg/dropped/y.go": {{startLine: 1, endLine: 2, numStmts: 4, count: 0}},
	}
	cfg := config{module: module, baseName: "main", baseRev: "main"}

	out := renderMarkdown(head, base, nil, false, cfg)

	// The em dash marks the side the package is missing from, so the row
	// pins the column order as well as the label.
	if !strings.Contains(out, "| `pkg/added` | 100.0% | — | new |") {
		t.Errorf("a package only in HEAD should be marked new\n---\n%s", out)
	}
	if !strings.Contains(out, "| `pkg/dropped` | — | 0.0% | removed |") {
		t.Errorf("a package only in the base should be marked removed\n---\n%s", out)
	}
	if strings.Contains(out, "| `cmd` |") {
		t.Errorf("a package that reads the same on both sides needs no row\n---\n%s", out)
	}
}
