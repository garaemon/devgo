package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseDiffBytes(t *testing.T) {
	diff := `diff --git a/cmd/up.go b/cmd/up.go
--- a/cmd/up.go
+++ b/cmd/up.go
@@ -10,0 +11,2 @@ func up() {
+	a()
+	b()
@@ -30 +32 @@ func down() {
+	c()
diff --git a/cmd/old.go b/cmd/old.go
--- a/cmd/old.go
+++ /dev/null
@@ -1,5 +0,0 @@
diff --git a/cmd/up_test.go b/cmd/up_test.go
--- a/cmd/up_test.go
+++ b/cmd/up_test.go
@@ -1,0 +2,3 @@
+ignored
diff --git a/tools/covreport/main.go b/tools/covreport/main.go
--- a/tools/covreport/main.go
+++ b/tools/covreport/main.go
@@ -1,0 +2 @@
+ignored
diff --git a/pkg/config/config.go b/pkg/config/config.go
--- a/pkg/config/config.go
+++ b/pkg/config/config.go
@@ -7,2 +8,0 @@ func load() {
`
	got, err := parseDiffBytes([]byte(diff), "test")
	if err != nil {
		t.Fatalf("parseDiffBytes returned error: %v", err)
	}
	want := map[string][]int{
		"cmd/up.go": {11, 12, 32},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDiffBytes = %v, want %v", got, want)
	}
	if _, ok := got["pkg/config/config.go"]; ok {
		t.Error("deletion-only hunk should contribute no added lines")
	}
}

func TestParseDiffBytesMalformed(t *testing.T) {
	diff := "+++ b/cmd/up.go\n@@ nonsense @@\n"
	_, err := parseDiffBytes([]byte(diff), "mysource")
	if err == nil {
		t.Fatal("expected an error for a malformed hunk header")
	}
	if !strings.Contains(err.Error(), "mysource") {
		t.Errorf("error %q should name the diff source", err)
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		line      string
		wantStart int
		wantCount int
	}{
		{"@@ -1 +1 @@", 1, 1},
		{"@@ -11,0 +12 @@ help:", 12, 1},
		{"@@ -42 +43,5 @@ test-coverage:", 43, 5},
		{"@@ -7,2 +8,0 @@", 8, 0},
	}
	for _, testCase := range tests {
		t.Run(testCase.line, func(t *testing.T) {
			start, count, err := parseHunkHeader(testCase.line)
			if err != nil {
				t.Fatalf("parseHunkHeader returned error: %v", err)
			}
			if start != testCase.wantStart || count != testCase.wantCount {
				t.Errorf("got (%d, %d), want (%d, %d)",
					start, count, testCase.wantStart, testCase.wantCount)
			}
		})
	}
}

func TestCompressRanges(t *testing.T) {
	tests := []struct {
		name  string
		lines []int
		want  string
	}{
		{"empty", nil, ""},
		{"single", []int{7}, "7"},
		{"run", []int{12, 13, 14, 15}, "12-15"},
		{"mixed", []int{12, 13, 14, 15, 22}, "12-15, 22"},
		{"pairs", []int{1, 2, 5, 6}, "1-2, 5-6"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := compressRanges(testCase.lines); got != testCase.want {
				t.Errorf("compressRanges(%v) = %q, want %q", testCase.lines, got, testCase.want)
			}
		})
	}
}

func TestPatchCoverageDropsEmptyFiles(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {{startLine: 10, endLine: 12, numStmts: 2, count: 1}},
	}
	added := map[string][]int{"cmd/a.go": {11}, "cmd/b.go": {3}}

	stats := patchCoverage(head, added, module)
	if len(stats) != 1 || stats[0].file != "cmd/a.go" {
		t.Fatalf("patchCoverage = %+v, want only cmd/a.go", stats)
	}
}

// source20 builds a 20-line file so line numbers match their content.
func source20() string {
	var source strings.Builder
	for lineNumber := 1; lineNumber <= 20; lineNumber++ {
		fmt.Fprintf(&source, "line %d\n", lineNumber)
	}
	// Trailing newline yields a final empty element, as os.ReadFile would.
	return strings.TrimSuffix(source.String(), "\n")
}

func TestAnnotateFile(t *testing.T) {
	blocks := []block{{startLine: 5, endLine: 6, numStmts: 2, count: 1}}
	file := annotateFile("cmd/x.go", source20(), blocks)

	if len(file.Lines) != 20 {
		t.Fatalf("got %d rows, want 20", len(file.Lines))
	}
	for index, row := range file.Lines {
		if row.Num != index+1 {
			t.Fatalf("row %d has line number %d", index, row.Num)
		}
	}
	if file.Lines[4].Class != "cov" || file.Lines[5].Class != "cov" {
		t.Errorf("lines 5-6 should be covered: %+v", file.Lines[4:6])
	}
	if file.Lines[6].Class != "" {
		t.Errorf("line 7 is outside every block, got class %q", file.Lines[6].Class)
	}
	if file.ID != "cmd-x-go" {
		t.Errorf("ID = %q, want cmd-x-go", file.ID)
	}
}

// Uncovered has to win over covered when a line is inside blocks of both
// kinds, so red always flags a line that still needs a test.
func TestAnnotateFileUncoveredWins(t *testing.T) {
	blocks := []block{
		{startLine: 5, endLine: 7, numStmts: 3, count: 4},
		{startLine: 6, endLine: 6, numStmts: 1, count: 0},
	}
	file := annotateFile("cmd/x.go", source20(), blocks)
	if got := file.Lines[5].Class; got != "uncov" {
		t.Errorf("line 6 class = %q, want uncov", got)
	}
}

func TestRenderHTML(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {{startLine: 5, endLine: 6, numStmts: 2, count: 1}},
	}
	readSource := func(relPath string) ([]byte, error) { return []byte(source20()), nil }

	var rendered bytes.Buffer
	if err := renderHTML(&rendered, head, module, "abc1234", readSource); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	page := rendered.String()

	for _, want := range []string{
		`<title>example.com/m coverage</title>`,
		`commit abc1234`,
		`href="#cmd-a-go"`,
		`<span class="cov">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("report should contain %q", want)
		}
	}
}
func TestParseProfileLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantFile  string
		wantBlock block
	}{
		{
			name:     "covered block",
			line:     "github.com/garaemon/devgo/pkg/config/config.go:58.39,59.52 1 1",
			wantFile: "github.com/garaemon/devgo/pkg/config/config.go",
			wantBlock: block{
				startLine: 58, endLine: 59, numStmts: 1, count: 1,
			},
		},
		{
			name:     "uncovered block keeps its statement count",
			line:     "example.com/m/cmd/up.go:41.24,49.16 5 0",
			wantFile: "example.com/m/cmd/up.go",
			wantBlock: block{
				startLine: 41, endLine: 49, numStmts: 5, count: 0,
			},
		},
		{
			name:     "count mode records real execution counts",
			line:     "example.com/m/cmd/up.go:10.2,10.20 1 47",
			wantFile: "example.com/m/cmd/up.go",
			wantBlock: block{
				startLine: 10, endLine: 10, numStmts: 1, count: 47,
			},
		},
		{
			// A module path may carry a port, so the position has to be split
			// off at the last colon rather than the first.
			name:     "module path containing a colon",
			line:     "localhost:8080/m/cmd/up.go:3.1,4.2 2 1",
			wantFile: "localhost:8080/m/cmd/up.go",
			wantBlock: block{
				startLine: 3, endLine: 4, numStmts: 2, count: 1,
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			file, gotBlock, err := parseProfileLine(testCase.line)
			if err != nil {
				t.Fatalf("parseProfileLine(%q) returned error: %v", testCase.line, err)
			}
			if file != testCase.wantFile {
				t.Errorf("file = %q, want %q", file, testCase.wantFile)
			}
			if gotBlock != testCase.wantBlock {
				t.Errorf("block = %+v, want %+v", gotBlock, testCase.wantBlock)
			}
		})
	}
}

func TestParseProfileLineMalformed(t *testing.T) {
	lines := []string{
		"",
		"mode set",
		"example.com/m/cmd/up.go 1.2,3.4 5 6",
		"example.com/m/cmd/up.go:1.2,3.4 5",
		"example.com/m/cmd/up.go:1.2,3.4 x 6",
		"example.com/m/cmd/up.go:1-2,3-4 5 6",
	}
	for _, line := range lines {
		if _, _, err := parseProfileLine(line); err == nil {
			t.Errorf("parseProfileLine(%q) = nil error, want a malformed-line error", line)
		}
	}
}

func TestParseProfile(t *testing.T) {
	const module = "example.com/m"
	// Blocks of one file are not necessarily contiguous, the header and blank
	// lines carry no data, and tooling and test sources are not coverable.
	text := `mode: set
example.com/m/cmd/up.go:41.24,49.16 5 1
example.com/m/pkg/config/config.go:58.39,59.52 1 1
example.com/m/cmd/up.go:49.16,51.4 1 0
example.com/m/cmd/up_test.go:7.13,9.2 1 1
example.com/m/tools/covreport/main.go:12.20,14.3 2 1

`
	path := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("writing the profile: %v", err)
	}

	got, err := parseProfile(path, module)
	if err != nil {
		t.Fatalf("parseProfile returned error: %v", err)
	}
	want := profile{
		module + "/cmd/up.go": {
			{startLine: 41, endLine: 49, numStmts: 5, count: 1},
			{startLine: 49, endLine: 51, numStmts: 1, count: 0},
		},
		module + "/pkg/config/config.go": {
			{startLine: 58, endLine: 59, numStmts: 1, count: 1},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseProfile = %+v, want %+v", got, want)
	}

	if total := totalTally(got); total.covered != 6 || total.total != 7 {
		t.Errorf("totalTally = %+v, want 6 covered of 7", total)
	}
}

func TestParseProfileModesAgree(t *testing.T) {
	const module = "example.com/m"
	const blocks = `example.com/m/cmd/up.go:41.24,49.16 5 1
example.com/m/cmd/up.go:49.16,51.4 1 0
`
	dir := t.TempDir()

	// "set" writes 0/1 while "count" and "atomic" write real counts; covreport
	// only asks whether the count is nonzero, so all three tally alike.
	var first tally
	for index, header := range []string{"mode: set\n", "mode: count\n", "mode: atomic\n"} {
		text := header + blocks
		if strings.HasPrefix(header, "mode: count") ||
			strings.HasPrefix(header, "mode: atomic") {
			text = header + strings.Replace(blocks, "5 1", "5 12", 1)
		}
		path := filepath.Join(dir, fmt.Sprintf("coverage%d.out", index))
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatalf("writing the profile: %v", err)
		}
		parsed, err := parseProfile(path, module)
		if err != nil {
			t.Fatalf("parseProfile(%q) returned error: %v", header, err)
		}
		total := totalTally(parsed)
		if index == 0 {
			first = total
			continue
		}
		if total != first {
			t.Errorf("%q tallied %+v, want %+v as in mode: set", header, total, first)
		}
	}
}

func TestParseProfileMalformedNamesTheLine(t *testing.T) {
	text := "mode: set\nexample.com/m/cmd/up.go:41.24,49.16 5 1\nnonsense\n"
	path := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("writing the profile: %v", err)
	}

	_, err := parseProfile(path, "example.com/m")
	if err == nil {
		t.Fatal("expected an error for a malformed profile line")
	}
	if !strings.Contains(err.Error(), path+":3") {
		t.Errorf("error %q should name the profile and the line number", err)
	}
}

func TestParseProfileMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.out")
	if _, err := parseProfile(path, "example.com/m"); err == nil {
		t.Fatal("expected an error for a profile that does not exist")
	}
}
