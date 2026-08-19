package main

import (
	"reflect"
	"strings"
	"testing"
)

func parseProfileOrFail(t *testing.T, module, content string) profile {
	t.Helper()
	p, err := parseProfile(writeTempFile(t, "coverage.out", content), module)
	if err != nil {
		t.Fatalf("parseProfile returned error: %v", err)
	}
	return p
}

func TestParseProfile(t *testing.T) {
	const module = "example.com/m"
	p := parseProfileOrFail(t, module, `mode: set
example.com/m/cmd/a.go:10.2,12.3 2 1

example.com/m/cmd/a_test.go:1.1,2.2 1 1
example.com/m/tools/covreport/main.go:1.1,2.2 1 1
example.com/m/test/integration/x.go:1.1,2.2 1 1
example.com/m/pkg/b.go:5.1,6.2 3 0
`)

	files := make([]string, 0, len(p))
	for f := range p {
		files = append(files, f)
	}
	want := []string{module + "/cmd/a.go", module + "/pkg/b.go"}
	if len(files) != len(want) {
		t.Fatalf("parsed files = %v, want %v (tests and tooling are excluded)", files, want)
	}
	for _, f := range want {
		if _, ok := p[f]; !ok {
			t.Errorf("%s missing from the profile", f)
		}
	}
	if got := p[module+"/cmd/a.go"]; !reflect.DeepEqual(got,
		[]block{{startLine: 10, endLine: 12, numStmts: 2, count: 1}}) {
		t.Errorf("cmd/a.go blocks = %+v", got)
	}
}

func TestParseProfileMalformed(t *testing.T) {
	path := writeTempFile(t, "coverage.out", "mode: set\nexample.com/m/cmd/a.go:oops 2 1\n")
	_, err := parseProfile(path, "example.com/m")
	if err == nil {
		t.Fatal("expected an error for a malformed profile line")
	}
	if !strings.Contains(err.Error(), "coverage.out:2") {
		t.Errorf("error %q should name the file and the line", err)
	}
}

func TestParseProfileRejectsImpossibleGeometry(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"negative start", "example.com/m/cmd/a.go:-1.1,-1.2 1 1"},
		{"end before start", "example.com/m/cmd/a.go:9.1,4.2 1 1"},
		{"negative statements", "example.com/m/cmd/a.go:1.1,2.2 -1 1"},
		{"negative count", "example.com/m/cmd/a.go:1.1,2.2 1 -1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, "coverage.out", "mode: set\n"+tt.line+"\n")
			// annotateFile indexes a per-line slice with these numbers, so a
			// profile nobody generated must fail here instead of panicking.
			if _, err := parseProfile(path, "example.com/m"); err == nil {
				t.Errorf("parseProfile accepted %q", tt.line)
			}
		})
	}
}

func TestSumProfileTally(t *testing.T) {
	const module = "example.com/m"
	p := profile{
		module + "/cmd/a.go": {
			{startLine: 1, endLine: 2, numStmts: 2, count: 1},
			{startLine: 4, endLine: 5, numStmts: 3, count: 0},
		},
		module + "/pkg/b.go": {{startLine: 1, endLine: 1, numStmts: 5, count: 7}},
	}
	if got, want := sumProfileTally(p), (tally{covered: 7, total: 10}); got != want {
		t.Errorf("sumProfileTally = %+v, want %+v", got, want)
	}
}

func TestTallyPerPackage(t *testing.T) {
	const module = "example.com/m"
	p := profile{
		module + "/cmd/a.go": {{startLine: 1, endLine: 2, numStmts: 2, count: 1}},
		module + "/cmd/b.go": {{startLine: 1, endLine: 2, numStmts: 2, count: 0}},
		module + "/pkg/c.go": {{startLine: 1, endLine: 1, numStmts: 1, count: 1}},
	}
	got := tallyPerPackage(p)
	if want := (tally{covered: 2, total: 4}); got[module+"/cmd"] != want {
		t.Errorf("cmd tally = %+v, want %+v", got[module+"/cmd"], want)
	}
	if want := (tally{covered: 1, total: 1}); got[module+"/pkg"] != want {
		t.Errorf("pkg tally = %+v, want %+v", got[module+"/pkg"], want)
	}
}

func TestShortenPackagePath(t *testing.T) {
	const module = "example.com/m"
	if got := shortenPackagePath(module, module); got != "." {
		t.Errorf("the module itself = %q, want .", got)
	}
	if got := shortenPackagePath(module+"/pkg/config", module); got != "pkg/config" {
		t.Errorf("nested package = %q, want pkg/config", got)
	}
}

func TestDedupInts(t *testing.T) {
	tests := []struct {
		name   string
		sorted []int
		want   []int
	}{
		{"empty", nil, nil},
		{"no repeats", []int{1, 2, 3}, []int{1, 2, 3}},
		{"repeats", []int{1, 1, 2, 2, 2, 3}, []int{1, 2, 3}},
		// Line numbers are always positive, but a general-purpose helper
		// must not treat any value as a sentinel.
		{"negatives kept", []int{-1, -1, 0, 1}, []int{-1, 0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupInts(append([]int(nil), tt.sorted...))
			if len(got) != len(tt.want) {
				t.Fatalf("dedupInts(%v) = %v, want %v", tt.sorted, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("dedupInts(%v) = %v, want %v", tt.sorted, got, tt.want)
				}
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

func TestComputePatchTallies(t *testing.T) {
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

	got := computePatchTallies(head, added, module)

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

func TestComputePatchCoverageDropsEmptyFiles(t *testing.T) {
	const module = "example.com/m"
	head := profile{
		module + "/cmd/a.go": {{startLine: 10, endLine: 12, numStmts: 2, count: 1}},
	}
	added := map[string][]int{"cmd/a.go": {11}, "cmd/b.go": {3}}

	stats := computePatchCoverage(head, added, module)
	if len(stats) != 1 || stats[0].file != "cmd/a.go" {
		t.Fatalf("computePatchCoverage = %+v, want only cmd/a.go", stats)
	}
}

func TestComputeFileStatReportsUncoveredAddedLines(t *testing.T) {
	blocks := []block{
		{startLine: 10, endLine: 12, numStmts: 2, count: 1},
		{startLine: 20, endLine: 22, numStmts: 3, count: 0},
	}
	st := computeFileStat("cmd/a.go", []int{11, 20, 21}, blocks)

	if want := (tally{covered: 2, total: 5}); st.t != want {
		t.Errorf("tally = %+v, want %+v", st.t, want)
	}
	// Only added lines inside an uncovered block are reported, so line 22 is
	// left out even though its block never ran.
	if want := []int{20, 21}; !reflect.DeepEqual(st.uncovered, want) {
		t.Errorf("uncovered = %v, want %v", st.uncovered, want)
	}
}
