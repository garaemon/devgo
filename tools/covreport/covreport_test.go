package main

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestDiffRegions(t *testing.T) {
	tests := []struct {
		name    string
		added   []int
		total   int
		context int
		want    []region
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
			got := diffRegions(tt.added, tt.total, tt.context)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("diffRegions(%v, %d, %d) = %v, want %v",
					tt.added, tt.total, tt.context, got, tt.want)
			}
		})
	}
}

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

// source20 builds a 20-line file so line numbers match their content.
func source20() string {
	var b strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	// Trailing newline yields a final empty element, as os.ReadFile would.
	return strings.TrimSuffix(b.String(), "\n")
}

func TestAnnotateFileDiff(t *testing.T) {
	blocks := []block{
		{startLine: 5, endLine: 6, numStmts: 2, count: 1},
		{startLine: 12, endLine: 12, numStmts: 1, count: 0},
	}
	f := annotateFile("cmd/x.go", source20(), blocks, []int{5, 12}, 3)

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
	f := annotateFile("cmd/x.go", source20(), blocks, nil, 3)

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
	if f.Lines[4].Class != "cov" || f.Lines[6].Class != "" {
		t.Errorf("coverage classes misapplied: %+v", f.Lines[:7])
	}
}

func TestAnnotateFileUncoveredWins(t *testing.T) {
	blocks := []block{
		{startLine: 5, endLine: 7, numStmts: 3, count: 4},
		{startLine: 6, endLine: 6, numStmts: 1, count: 0},
	}
	f := annotateFile("cmd/x.go", source20(), blocks, nil, 3)
	if got := f.Lines[5].Class; got != "uncov" {
		t.Errorf("line 6 class = %q, want uncov", got)
	}
}

func TestHighlightLines(t *testing.T) {
	src := "package p\n" +
		"\n" +
		"// count returns 2 < 3 & more.\n" +
		"func count(s string) int {\n" +
		"\treturn len(s) + 42\n" +
		"}\n"

	got := highlightLines("p.go", src)
	if want := strings.Split(src, "\n"); len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	checks := []struct {
		line int
		want string
	}{
		{1, `<span class="k">package</span> p`},
		{3, `<span class="c">// count returns 2 &lt; 3 &amp; more.</span>`},
		{4, `<span class="k">func</span> <span class="f">count</span>(s ` +
			`<span class="b">string</span>) <span class="b">int</span> {`},
		{5, "\t<span class=\"k\">return</span> <span class=\"b\">len</span>(s) + " +
			`<span class="m">42</span>`},
	}
	for _, c := range checks {
		if string(got[c.line-1]) != c.want {
			t.Errorf("line %d =\n  %s\nwant\n  %s", c.line, got[c.line-1], c.want)
		}
	}
}

// Only declared functions and methods get the name colour. A func *type*
// looks the same lexically up to its closing paren, so the distinguishing
// signal is what follows the identifier.
func TestHighlightLinesFuncNames(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{
			name: "method",
			src:  "func (r *T) Do(a int) {}",
			want: `<span class="k">func</span> (r *T) <span class="f">Do</span>` +
				`(a <span class="b">int</span>) {}`,
		},
		{
			name: "generic function",
			src:  "func Map[K any](m K) {}",
			want: `<span class="k">func</span> <span class="f">Map</span>` +
				`[K <span class="b">any</span>](m K) {}`,
		},
		{
			name: "func type keeps its return type",
			src:  "var f func(int) int",
			want: `<span class="k">var</span> f <span class="k">func</span>` +
				`(<span class="b">int</span>) <span class="b">int</span>`,
		},
		{
			name: "func literal has no name",
			src:  "var g = func(x int) int { return x }",
			want: `<span class="k">var</span> g = <span class="k">func</span>` +
				`(x <span class="b">int</span>) <span class="b">int</span> { ` +
				`<span class="k">return</span> x }`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := highlightLines("p.go", "package p\n"+tt.src+"\n")
			if len(got) != 3 {
				t.Fatalf("got %d lines, want 3", len(got))
			}
			if string(got[1]) != tt.want {
				t.Errorf("got\n  %s\nwant\n  %s", got[1], tt.want)
			}
		})
	}
}

// Raw strings and block comments cover several rows, and each row is wrapped
// on its own so the coverage tint of the rows around them stays intact.
func TestHighlightLinesMultiLineTokens(t *testing.T) {
	src := "package p\n" +
		"\n" +
		"/* one\n" +
		"   two */\n" +
		"var s = `raw <\n" +
		"line`\n"

	got := highlightLines("p.go", src)
	want := []string{
		`<span class="k">package</span> p`,
		``,
		`<span class="c">/* one</span>`,
		`<span class="c">   two */</span>`,
		`<span class="k">var</span> s = <span class="s">` + "`raw &lt;" + `</span>`,
		`<span class="s">line` + "`" + `</span>`,
		``,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Errorf("line %d =\n  %s\nwant\n  %s", i+1, got[i], want[i])
		}
	}
}

// Anything that is not tokenizable Go still has to render, escaped and with
// its line count intact, since the rows carry the line numbers.
func TestHighlightLinesFallback(t *testing.T) {
	tests := []struct {
		name, rel, src string
		want           []string
	}{
		{
			name: "not a go file",
			rel:  "notes.txt",
			src:  "func x() <b>\nplain\n",
			want: []string{"func x() &lt;b&gt;", "plain", ""},
		},
		{
			name: "unscannable go source",
			rel:  "broken.go",
			src:  "package p\nvar s = \"unterminated & <\n",
			want: []string{"package p", `var s = &#34;unterminated &amp; &lt;`, ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := highlightLines(tt.rel, tt.src)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d lines, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if string(got[i]) != tt.want[i] {
					t.Errorf("line %d = %q, want %q", i+1, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPatchTallies(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {
			{startLine: 10, endLine: 12, numStmts: 2, count: 1},
			{startLine: 20, endLine: 22, numStmts: 3, count: 0},
			{startLine: 40, endLine: 42, numStmts: 5, count: 1},
		},
	}
	added := map[string][]int{
		"cmd/a.go": {11, 21},
		"cmd/b.go": {3}, // changed but absent from the profile
	}

	got := patchTallies(head, added, module)

	if want := (tally{covered: 2, total: 5}); got["cmd/a.go"] != want {
		t.Errorf("cmd/a.go tally = %+v, want %+v", got["cmd/a.go"], want)
	}
	if _, ok := got["cmd/b.go"]; !ok {
		t.Error("a changed file with no coverable statements must still be present")
	}
	if want := (tally{}); got["cmd/b.go"] != want {
		t.Errorf("cmd/b.go tally = %+v, want zero", got["cmd/b.go"])
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
		Module:   module,
		Added:    map[string][]int{"cmd/a.go": {5, 12}},
		HaveDiff: true,
		DiffBase: "main",
		Context:  3,
		Source:   func(rel string) ([]byte, error) { return []byte(source20()), nil },
	}

	var buf bytes.Buffer
	if err := renderHTML(&buf, head, opts); err != nil {
		t.Fatalf("renderHTML returned error: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<body class="mode-diff">`,
		`class="gap"`,
		` add"`,
		`data-mode="diff"`,
		`main … working tree`,
		`patch 66.7% (2/3 statements)`,
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

func TestRenderHTMLNoDiff(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {{startLine: 5, endLine: 6, numStmts: 2, count: 1}},
	}
	opts := htmlOptions{
		Module: module,
		Source: func(rel string) ([]byte, error) { return []byte(source20()), nil },
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
		Module:   module,
		Added:    map[string][]int{},
		HaveDiff: true,
		DiffBase: "main",
		Context:  3,
		Source:   func(rel string) ([]byte, error) { return []byte(source20()), nil },
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
