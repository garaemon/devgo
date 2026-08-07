package main

import (
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
