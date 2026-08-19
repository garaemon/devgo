package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestComputeDiffRegions(t *testing.T) {
	tests := []struct {
		name         string
		added        []int
		total        int
		contextLines int
		want         []region
	}{
		{"no added lines", nil, 20, 3, nil},
		{"empty file", []int{1}, 0, 3, nil},
		{"clamps at start", []int{2}, 20, 3, []region{{1, 5}}},
		{"clamps at end", []int{19}, 20, 3, []region{{16, 20}}},
		{"windows merge when touching", []int{5, 12}, 20, 3, []region{{2, 15}}},
		{"windows stay apart", []int{5, 20}, 30, 3, []region{{2, 8}, {17, 23}}},
		{"line past eof ignored", []int{5, 99}, 20, 3, []region{{2, 8}}},
		{"line before start ignored", []int{0, 5}, 20, 3, []region{{2, 8}}},
		{"zero context", []int{5, 6, 10}, 20, 0, []region{{5, 6}, {10, 10}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeDiffRegions(tt.added, tt.total, tt.contextLines)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("computeDiffRegions(%v, %d, %d) = %v, want %v",
					tt.added, tt.total, tt.contextLines, got, tt.want)
			}
		})
	}
}

// buildNumberedSource returns a file whose text names each line, so an
// assertion about Lines[i] reads as line i+1. It has no trailing newline; the
// tests that care about the on-disk form add one, as
// TestAnnotateFileTrailingNewline does.
func buildNumberedSource(lineCount int) string {
	var b strings.Builder
	for i := 1; i <= lineCount; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func TestAnnotateFileDiff(t *testing.T) {
	blocks := []block{
		{startLine: 5, endLine: 6, numStmts: 2, count: 1},
		{startLine: 12, endLine: 12, numStmts: 1, count: 0},
	}
	f := annotateFile("cmd/x.go", buildNumberedSource(20), blocks, []int{5, 12}, 3)

	if !f.Changed {
		t.Error("file with added lines should be Changed")
	}

	var gaps []int
	added := map[int]bool{}
	visible := map[int]bool{}
	for _, l := range f.Lines {
		if l.Gap > 0 {
			gaps = append(gaps, l.Gap)
			continue
		}
		if l.Added {
			added[l.Num] = true
		}
		if l.Visible {
			visible[l.Num] = true
		}
	}

	// Windows are 2-8 and 9-15 with context 3, which touch and merge into
	// 2-15, leaving line 1 and lines 16-20 hidden.
	if want := []int{1, 5}; !reflect.DeepEqual(gaps, want) {
		t.Errorf("gap rows = %v, want %v", gaps, want)
	}
	if want := map[int]bool{5: true, 12: true}; !reflect.DeepEqual(added, want) {
		t.Errorf("added lines = %v, want %v", added, want)
	}
	for n := 2; n <= 15; n++ {
		if !visible[n] {
			t.Errorf("line %d should be visible", n)
		}
	}
	for _, n := range []int{1, 16, 20} {
		if visible[n] {
			t.Errorf("line %d should be hidden", n)
		}
	}
	// Hidden lines stay in the slice for the all-files view, so the separator
	// sits between the hidden run and the first visible line.
	if got := f.Lines[0]; got.Gap != 0 || got.Num != 1 || got.Visible {
		t.Errorf("row 0 should be hidden line 1, got %+v", got)
	}
	if got := f.Lines[1]; got.Gap != 1 {
		t.Errorf("row 1 should be the leading gap, got %+v", got)
	}
	if got := f.Lines[2]; got.Num != 2 || !got.Visible {
		t.Errorf("row 2 should be visible line 2, got %+v", got)
	}
	if got := f.Lines[len(f.Lines)-1]; got.Gap != 5 {
		t.Errorf("last row should be the trailing gap, got %+v", got)
	}
}

func TestAnnotateFileNoDiff(t *testing.T) {
	blocks := []block{{startLine: 5, endLine: 6, numStmts: 2, count: 1}}
	f := annotateFile("cmd/x.go", buildNumberedSource(20), blocks, nil, 3)

	if f.Changed {
		t.Error("file without added lines should not be Changed")
	}
	if len(f.Lines) != 20 {
		t.Fatalf("got %d rows, want 20", len(f.Lines))
	}
	for _, l := range f.Lines {
		if l.Gap > 0 {
			t.Errorf("line %d: no gap rows expected without a diff", l.Num)
		}
		if !l.Visible {
			t.Errorf("line %d should be visible in the all-files view", l.Num)
		}
		if l.Added {
			t.Errorf("line %d should not be marked added", l.Num)
		}
	}
	if got := f.Lines[4].Class; got != "cov" {
		t.Errorf("line 5 class = %q, want cov", got)
	}
	if got := f.Lines[6].Class; got != "" {
		t.Errorf("line 7 class = %q, want empty: no block covers it", got)
	}
}

func TestAnnotateFileTrailingNewline(t *testing.T) {
	// os.ReadFile hands annotateFile the trailing newline every text file
	// ends with. It closes the last line rather than starting a new one.
	f := annotateFile("cmd/x.go", buildNumberedSource(20)+"\n", nil, nil, 3)

	if len(f.Lines) != 20 {
		t.Fatalf("got %d rows, want 20", len(f.Lines))
	}
	last := f.Lines[len(f.Lines)-1]
	if last.Num != 20 || last.Text != "line 20" {
		t.Errorf("last row = %+v, want line 20", last)
	}
}

func TestAnnotateFileUncoveredWins(t *testing.T) {
	blocks := []block{
		{startLine: 5, endLine: 7, numStmts: 3, count: 4},
		{startLine: 6, endLine: 6, numStmts: 1, count: 0},
	}
	f := annotateFile("cmd/x.go", buildNumberedSource(20), blocks, nil, 3)
	if got := f.Lines[5].Class; got != "uncov" {
		t.Errorf("line 6 class = %q, want uncov", got)
	}
}

func TestAnnotateFileWithProfileSourceSkew(t *testing.T) {
	// The -diff-head path reads sources at one revision and the profile from
	// another, so a block can name lines the file no longer has.
	blocks := []block{{startLine: 19, endLine: 40, numStmts: 2, count: 1}}
	f := annotateFile("cmd/x.go", buildNumberedSource(20), blocks, nil, 3)

	if len(f.Lines) != 20 {
		t.Fatalf("got %d rows, want 20", len(f.Lines))
	}
	for _, n := range []int{19, 20} {
		if got := f.Lines[n-1].Class; got != "cov" {
			t.Errorf("line %d class = %q, want cov", n, got)
		}
	}
	if got := f.Lines[17].Class; got != "" {
		t.Errorf("line 18 class = %q, want empty: the block starts at 19", got)
	}
}

func TestMakeUniqueID(t *testing.T) {
	used := map[string]bool{}
	// Both paths flatten to the same id, and a repeated id would make one
	// sidebar link open the other file.
	first := makeUniqueID("cmd-a-go", used)
	second := makeUniqueID("cmd-a-go", used)
	if first != "cmd-a-go" {
		t.Errorf("first id = %q, want cmd-a-go", first)
	}
	if second == first {
		t.Errorf("second id = %q, want it to differ from %q", second, first)
	}
}

func TestRenderHTMLDiff(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {
			{startLine: 5, endLine: 6, numStmts: 2, count: 1},
			{startLine: 12, endLine: 12, numStmts: 1, count: 0},
		},
		module + "/cmd/untouched.go": {
			{startLine: 1, endLine: 2, numStmts: 1, count: 1},
		},
	}
	opts := htmlOptions{
		Module:       module,
		Added:        map[string][]int{"cmd/a.go": {5, 12}},
		HaveDiff:     true,
		DiffBase:     "main",
		ContextLines: 3,
		Source: func(rel string) ([]byte, error) {
			return []byte(buildNumberedSource(20)), nil
		},
	}

	var buf bytes.Buffer
	if err := renderHTML(&buf, head, opts); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<body class="mode-diff">`,
		`class="gap"`,
		`class="cov add"`,   // line 5: added and covered
		`class="uncov add"`, // line 12: added and never run
		`data-mode="diff"`,
		`main … working tree`,
		`patch 66.7% (2/3 statements)`,               // header total
		`<span class="pct diff-only">66.7%</span>`,   // cmd/a.go in the sidebar
		`<span class="diff-only">patch 66.7%</span>`, // and above its source
		// The untouched file has no added line, so it has no patch coverage.
		`<span class="pct diff-only">n/a</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
	// The untouched file is still emitted, just filtered by CSS in diff mode.
	if !strings.Contains(out, "cmd/untouched.go") {
		t.Error("all-files view should still contain untouched files")
	}
}

func TestRenderHTMLKeepsCallerAddedLines(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {{startLine: 5, endLine: 6, numStmts: 2, count: 1}},
	}
	added := map[string][]int{"cmd/a.go": {12, 5, 5}}
	opts := htmlOptions{
		Module:       module,
		Added:        added,
		HaveDiff:     true,
		DiffBase:     "main",
		ContextLines: 3,
		Source: func(rel string) ([]byte, error) {
			return []byte(buildNumberedSource(20)), nil
		},
	}

	if err := renderHTML(io.Discard, head, opts); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	// Sorting and deduping in place would leave the caller holding
	// [5 12 12], which silently changes any later patch calculation.
	if want := []int{12, 5, 5}; !reflect.DeepEqual(added["cmd/a.go"], want) {
		t.Errorf("caller's added lines = %v, want %v untouched", added["cmd/a.go"], want)
	}
}

func TestRenderHTMLPatchTotalCountsUnreadableFiles(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {{startLine: 5, endLine: 6, numStmts: 2, count: 1}},
		module + "/cmd/gone.go": {
			{startLine: 5, endLine: 6, numStmts: 2, count: 0},
		},
	}
	opts := htmlOptions{
		Module:       module,
		Added:        map[string][]int{"cmd/a.go": {5}, "cmd/gone.go": {5}},
		HaveDiff:     true,
		DiffBase:     "main",
		ContextLines: 3,
		Source: func(rel string) ([]byte, error) {
			if rel == "cmd/gone.go" {
				return nil, os.ErrNotExist
			}
			return []byte(buildNumberedSource(20)), nil
		},
	}

	var buf bytes.Buffer
	if err := renderHTML(&buf, head, opts); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	out := buf.String()

	// The markdown report counts cmd/gone.go, so the html header must too,
	// or the two reports disagree on the same profile.
	if !strings.Contains(out, "patch 50.0% (2/4 statements)") {
		t.Errorf("patch total should include the unreadable file\n---\n%s", out)
	}
	if !strings.Contains(out, "1 source(s) unavailable") {
		t.Error("the header should say a source could not be read")
	}
}

func TestRenderHTMLEmptyStateNamesUnreadableSources(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/gone.go": {{startLine: 5, endLine: 6, numStmts: 2, count: 0}},
	}
	opts := htmlOptions{
		Module:       module,
		Added:        map[string][]int{"cmd/gone.go": {5}},
		HaveDiff:     true,
		DiffBase:     "main",
		ContextLines: 3,
		Source:       func(rel string) ([]byte, error) { return nil, os.ErrNotExist },
	}

	var buf bytes.Buffer
	if err := renderHTML(&buf, head, opts); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	out := buf.String()

	// Saying "no coverable changes" here would contradict the header, which
	// still reports a patch percentage for the file it could not read.
	if strings.Contains(out, "No coverable changes") {
		t.Errorf("the empty state must not claim there were no changes\n---\n%s", out)
	}
	if !strings.Contains(out, "changed source(s) could not be read") {
		t.Errorf("the empty state should name the real reason\n---\n%s", out)
	}
}

func TestRenderHTMLNoDiff(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {{startLine: 5, endLine: 6, numStmts: 2, count: 1}},
	}
	opts := htmlOptions{
		Module: module,
		Source: func(rel string) ([]byte, error) {
			return []byte(buildNumberedSource(20)), nil
		},
	}

	var buf bytes.Buffer
	if err := renderHTML(&buf, head, opts); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `<body class="mode-all">`) {
		t.Error("a report without a diff should open in all-files mode")
	}
	if strings.Contains(out, "data-mode=") {
		t.Error("the mode toggle should not render without a diff")
	}
	if strings.Contains(out, `class="gap"`) {
		t.Error("no elision separators should render without a diff")
	}
}

func TestRenderHTMLEmptyDiff(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {{startLine: 5, endLine: 6, numStmts: 2, count: 1}},
	}
	opts := htmlOptions{
		Module:       module,
		Added:        map[string][]int{},
		HaveDiff:     true,
		DiffBase:     "main",
		ContextLines: 3,
		Source: func(rel string) ([]byte, error) {
			return []byte(buildNumberedSource(20)), nil
		},
	}

	var buf bytes.Buffer
	if err := renderHTML(&buf, head, opts); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `<body class="mode-all">`) {
		t.Error("a diff with no coverable changes should open in all-files mode")
	}
	if !strings.Contains(out, "No coverable changes") {
		t.Error("the empty state should be rendered")
	}
}

func TestRenderHTMLEmptyStateIgnoresUnchangedUnreadableFiles(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/unchanged.go": {{startLine: 5, endLine: 6, numStmts: 2, count: 1}},
	}
	opts := htmlOptions{
		Module:       module,
		Added:        map[string][]int{},
		HaveDiff:     true,
		DiffBase:     "main",
		ContextLines: 3,
		Source:       func(rel string) ([]byte, error) { return nil, os.ErrNotExist },
	}

	var buf bytes.Buffer
	if err := renderHTML(&buf, head, opts); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	out := buf.String()

	// The unreadable file is not part of the diff, so the diff view really
	// does have nothing to show, and saying otherwise would mislead.
	if !strings.Contains(out, "No coverable changes") {
		t.Errorf("the empty state should report the absence of changes\n---\n%s", out)
	}
	if strings.Contains(out, "changed source(s) could not be read") {
		t.Errorf("an unchanged file must not be described as changed\n---\n%s", out)
	}
	// The header still accounts for it, so the omission is visible.
	if !strings.Contains(out, "1 source(s) unavailable") {
		t.Errorf("the header should still count the unreadable file\n---\n%s", out)
	}
}

func TestRenderHTMLLabelsStatementlessFiles(t *testing.T) {
	const module = "example.com/m"
	// A changed file holding only declarations never reaches the profile.
	// Showing 0.0% would read as "covered nothing", which is not true.
	opts := htmlOptions{
		Module:       module,
		Added:        map[string][]int{"cmd/consts.go": {4}},
		HaveDiff:     true,
		DiffBase:     "main",
		ContextLines: 3,
		Source: func(rel string) ([]byte, error) {
			return []byte("package cmd\n\nconst A = 1\nconst B = 2\n"), nil
		},
	}

	var buf bytes.Buffer
	if err := renderHTML(&buf, profile{}, opts); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, `class="pct all-only">0.0%`) {
		t.Errorf("a file with no statements must not read as 0%%\n---\n%s", out)
	}
	if !strings.Contains(out, `class="pct all-only">n/a`) {
		t.Errorf("a file with no statements should read n/a\n---\n%s", out)
	}
	// The same file is part of the diff, so the diff view labels it too.
	if !strings.Contains(out, `class="pct diff-only">n/a`) {
		t.Errorf("the patch column should read n/a as well\n---\n%s", out)
	}
}

func TestAnnotateFileLabelsCoveragePercent(t *testing.T) {
	blocks := []block{
		{startLine: 5, endLine: 6, numStmts: 2, count: 1},
		{startLine: 7, endLine: 8, numStmts: 2, count: 0},
	}
	f := annotateFile("cmd/x.go", buildNumberedSource(20), blocks, nil, 3)
	if f.PercentLabel != "50.0%" {
		t.Errorf("PercentLabel = %q, want 50.0%%", f.PercentLabel)
	}

	empty := annotateFile("cmd/consts.go", buildNumberedSource(20), nil, nil, 3)
	if empty.PercentLabel != "n/a" {
		t.Errorf("PercentLabel = %q, want n/a for a file with no statements", empty.PercentLabel)
	}
}
