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
-	old()
+	c()
diff --git a/cmd/old.go b/cmd/old.go
--- a/cmd/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package cmd
-
-func old() {}
diff --git a/cmd/up_test.go b/cmd/up_test.go
--- a/cmd/up_test.go
+++ b/cmd/up_test.go
@@ -1,0 +2 @@
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
-	x()
-	y()
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

func TestParseDiffBytesIgnoresHeaderLookalikeContent(t *testing.T) {
	// An added line reading "++ b/spoofed.go" reaches the diff as
	// "+++ b/spoofed.go", which is exactly a file header. Counting the hunk
	// body is what keeps the following lines attributed to cmd/real.go.
	diff := `diff --git a/cmd/real.go b/cmd/real.go
--- a/cmd/real.go
+++ b/cmd/real.go
@@ -2,0 +3 @@ package cmd
+++ b/spoofed.go
@@ -3,0 +5,2 @@ func f() {
+	x()
+	y()
`
	got, err := parseDiffBytes([]byte(diff), "test")
	if err != nil {
		t.Fatalf("parseDiffBytes returned error: %v", err)
	}
	want := map[string][]int{"cmd/real.go": {3, 5, 6}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDiffBytes = %v, want %v", got, want)
	}
}

func TestParseDiffBytesWithContextLines(t *testing.T) {
	// covreport generates -U0 diffs, but a hand-supplied -diff file can carry
	// context. Only the "+" lines are added lines.
	diff := `diff --git a/cmd/up.go b/cmd/up.go
--- a/cmd/up.go
+++ b/cmd/up.go
@@ -8,6 +8,7 @@ func up() {
 	first()
 	second()
-	removed()
+	inserted()
+	alsoInserted()
 	third()
 	fourth()
 	fifth()
`
	got, err := parseDiffBytes([]byte(diff), "test")
	if err != nil {
		t.Fatalf("parseDiffBytes returned error: %v", err)
	}
	want := map[string][]int{"cmd/up.go": {10, 11}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDiffBytes = %v, want %v", got, want)
	}
}

func TestParseDiffBytesWithNoNewlineMarker(t *testing.T) {
	// A file whose last line has no trailing newline makes git emit the
	// "\ No newline" marker, which belongs to neither side. Counting it as a
	// line would shift every later added line and drop the last one.
	diff := "diff --git a/cmd/up.go b/cmd/up.go\n" +
		"--- a/cmd/up.go\n" +
		"+++ b/cmd/up.go\n" +
		"@@ -9 +9,2 @@\n" +
		"-\told()\n" +
		"\\ No newline at end of file\n" +
		"+\ta()\n" +
		"+\tb()\n" +
		"\\ No newline at end of file\n"

	got, err := parseDiffBytes([]byte(diff), "test")
	if err != nil {
		t.Fatalf("parseDiffBytes returned error: %v", err)
	}
	want := map[string][]int{"cmd/up.go": {9, 10}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDiffBytes = %v, want %v", got, want)
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
		line         string
		wantNewStart int
		wantOldCount int
		wantNewCount int
	}{
		{"@@ -1 +1 @@", 1, 1, 1},
		{"@@ -11,0 +12 @@ help:", 12, 0, 1},
		{"@@ -42 +43,5 @@ test-coverage:", 43, 1, 5},
		{"@@ -7,2 +8,0 @@", 8, 2, 0},
		{"@@ -8,6 +8,7 @@ func up() {", 8, 6, 7},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			newStart, oldCount, newCount, err := parseHunkHeader(tt.line)
			if err != nil {
				t.Fatalf("parseHunkHeader returned error: %v", err)
			}
			if newStart != tt.wantNewStart {
				t.Errorf("new start = %d, want %d", newStart, tt.wantNewStart)
			}
			if oldCount != tt.wantOldCount {
				t.Errorf("old count = %d, want %d", oldCount, tt.wantOldCount)
			}
			if newCount != tt.wantNewCount {
				t.Errorf("new count = %d, want %d", newCount, tt.wantNewCount)
			}
		})
	}
}

func TestCoverablePathFromHeader(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"+++ b/cmd/up.go", "cmd/up.go"},
		{"+++ b/cmd/up.go\t2026-01-01 00:00:00", "cmd/up.go"},
		{"+++ /dev/null", ""},
		{"+++ b/cmd/up_test.go", ""},
		{"+++ b/test/integration/x.go", ""},
		{"+++ b/tools/covreport/main.go", ""},
		{"+++ b/README.md", ""},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			if got := coverablePathFromHeader(tt.header); got != tt.want {
				t.Errorf("coverablePathFromHeader(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}
