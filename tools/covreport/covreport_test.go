package main

import (
	"bytes"
	"fmt"
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
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			start, count, err := parseHunkHeader(tt.line)
			if err != nil {
				t.Fatalf("parseHunkHeader returned error: %v", err)
			}
			if start != tt.wantStart || count != tt.wantCount {
				t.Errorf("got (%d, %d), want (%d, %d)",
					start, count, tt.wantStart, tt.wantCount)
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compressRanges(tt.lines); got != tt.want {
				t.Errorf("compressRanges(%v) = %q, want %q", tt.lines, got, tt.want)
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
	var b strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	// Trailing newline yields a final empty element, as os.ReadFile would.
	return strings.TrimSuffix(b.String(), "\n")
}

func TestAnnotateFile(t *testing.T) {
	blocks := []block{{startLine: 5, endLine: 6, numStmts: 2, count: 1}}
	f := annotateFile("cmd/x.go", source20(), blocks)

	if len(f.Lines) != 20 {
		t.Fatalf("got %d rows, want 20", len(f.Lines))
	}
	for i, l := range f.Lines {
		if l.Num != i+1 {
			t.Fatalf("row %d has line number %d", i, l.Num)
		}
	}
	if f.Lines[4].Class != "cov" || f.Lines[5].Class != "cov" {
		t.Errorf("lines 5-6 should be covered: %+v", f.Lines[4:6])
	}
	if f.Lines[6].Class != "" {
		t.Errorf("line 7 is outside every block, got class %q", f.Lines[6].Class)
	}
	if f.ID != "cmd-x-go" {
		t.Errorf("ID = %q, want cmd-x-go", f.ID)
	}
}

// Uncovered has to win over covered when a line is inside blocks of both
// kinds, so red always flags a line that still needs a test.
func TestAnnotateFileUncoveredWins(t *testing.T) {
	blocks := []block{
		{startLine: 5, endLine: 7, numStmts: 3, count: 4},
		{startLine: 6, endLine: 6, numStmts: 1, count: 0},
	}
	f := annotateFile("cmd/x.go", source20(), blocks)
	if got := f.Lines[5].Class; got != "uncov" {
		t.Errorf("line 6 class = %q, want uncov", got)
	}
}

func TestRenderHTML(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {{startLine: 5, endLine: 6, numStmts: 2, count: 1}},
	}
	source := func(rel string) ([]byte, error) { return []byte(source20()), nil }

	var buf bytes.Buffer
	if err := renderHTML(&buf, head, module, "abc1234", source); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<title>example.com/m coverage</title>`,
		`commit abc1234`,
		`href="#cmd-a-go"`,
		`<span class="cov">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report should contain %q", want)
		}
	}
}
