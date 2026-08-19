// profile.go reads Go coverage profiles and aggregates them. Every number the
// reports show is a statement count taken from here: whole-module totals,
// per-package totals, and the "patch" subset restricted to changed lines.

package main

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// block is one record from a coverage profile: a range of source lines with
// a statement count and how many times those statements ran.
type block struct {
	startLine int
	endLine   int
	numStmts  int
	count     int
}

// profile maps a module-path-qualified file name (as it appears in the
// profile, e.g. github.com/garaemon/devgo/pkg/config/config.go) to its
// coverage blocks.
type profile map[string][]block

// parseProfile reads a coverage profile, dropping files that are not
// coverable (tooling under tools/). Each line after the "mode:" header
// looks like: name.go:startLine.startCol,endLine.endCol numStmts count
func parseProfile(profilePath, module string) (profile, error) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, err
	}

	parsed := profile{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		file, parsedBlock, err := parseProfileLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %v", profilePath, i+1, err)
		}
		if !isCoverableFile(strings.TrimPrefix(file, module+"/")) {
			continue
		}
		parsed[file] = append(parsed[file], parsedBlock)
	}
	return parsed, nil
}

func parseProfileLine(line string) (string, block, error) {
	colon := strings.LastIndex(line, ":")
	if colon < 0 {
		return "", block{}, fmt.Errorf("malformed profile line %q", line)
	}
	file := line[:colon]
	var startLine, startColumn, endLine, endColumn, statementCount, executionCount int
	fields, err := fmt.Sscanf(line[colon+1:], "%d.%d,%d.%d %d %d",
		&startLine, &startColumn, &endLine, &endColumn, &statementCount, &executionCount)
	if err != nil || fields != 6 {
		return "", block{}, fmt.Errorf("malformed profile line %q", line)
	}
	// Line numbers index into a per-line slice later on, so a profile that
	// nobody generated but somebody supplied must be rejected here rather
	// than crash the annotator.
	if startLine < 1 || endLine < startLine || statementCount < 0 || executionCount < 0 {
		return "", block{}, fmt.Errorf("malformed profile line %q", line)
	}
	return file, block{
		startLine: startLine,
		endLine:   endLine,
		numStmts:  statementCount,
		count:     executionCount,
	}, nil
}

// isCoverableFile reports whether a repository path holds production code the
// reports should account for.
func isCoverableFile(name string) bool {
	return strings.HasSuffix(name, ".go") &&
		!strings.HasSuffix(name, "_test.go") &&
		!strings.HasPrefix(name, "test/") &&
		!strings.HasPrefix(name, "tools/")
}

type tally struct {
	covered int
	total   int
}

func (t tally) percent() float64 {
	if t.total == 0 {
		return 0
	}
	return 100 * float64(t.covered) / float64(t.total)
}

func (t *tally) add(profileBlock block) {
	t.total += profileBlock.numStmts
	if profileBlock.count > 0 {
		t.covered += profileBlock.numStmts
	}
}

// tallyPerPackage aggregates a profile into per-package statement tallies.
func tallyPerPackage(prof profile) map[string]tally {
	pkgs := map[string]tally{}
	for file, blocks := range prof {
		pkgPath := path.Dir(file)
		t := pkgs[pkgPath]
		for _, b := range blocks {
			t.add(b)
		}
		pkgs[pkgPath] = t
	}
	return pkgs
}

// sumProfileTally totals the statements of a whole profile, which is the
// "project" coverage the reports lead with.
func sumProfileTally(prof profile) tally {
	var t tally
	for _, blocks := range prof {
		for _, b := range blocks {
			t.add(b)
		}
	}
	return t
}

// fileStat is one file's patch coverage: the statements of the blocks that
// overlap added lines, and the added lines that stayed uncovered.
type fileStat struct {
	file      string
	t         tally
	uncovered []int
}

// computePatchCoverage returns the per-file patch coverage of every changed
// file that has coverable statements.
func computePatchCoverage(head profile, added map[string][]int, module string) []fileStat {
	var stats []fileStat
	files := make([]string, 0, len(added))
	for f := range added {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, file := range files {
		st := computeFileStat(file, added[file], head[module+"/"+file])
		if st.t.total > 0 {
			stats = append(stats, st)
		}
	}
	return stats
}

// computeFileStat computes one file's patch tally: the statements of every
// block overlapping an added line, plus the added lines that fall in
// uncovered blocks.
func computeFileStat(file string, added []int, blocks []block) fileStat {
	lineSet := map[int]bool{}
	for _, lineNumber := range added {
		lineSet[lineNumber] = true
	}
	st := fileStat{file: file}
	for _, b := range blocks {
		touched := false
		for lineNumber := b.startLine; lineNumber <= b.endLine; lineNumber++ {
			if lineSet[lineNumber] {
				touched = true
				break
			}
		}
		if !touched {
			continue
		}
		st.t.add(b)
		if b.count == 0 {
			for lineNumber := b.startLine; lineNumber <= b.endLine; lineNumber++ {
				if lineSet[lineNumber] {
					st.uncovered = append(st.uncovered, lineNumber)
				}
			}
		}
	}
	sort.Ints(st.uncovered)
	st.uncovered = dedupInts(st.uncovered)
	return st
}

// computePatchTallies returns the patch tally of every changed file,
// including files with no coverable statements, which the html sidebar
// still needs to list.
func computePatchTallies(head profile, added map[string][]int, module string) map[string]tally {
	out := make(map[string]tally, len(added))
	for file, lines := range added {
		out[file] = computeFileStat(file, lines, head[module+"/"+file]).t
	}
	return out
}

// dedupInts drops repeated values from a sorted slice. It reuses the input's
// backing array, so the caller must pass a slice nobody else reads.
func dedupInts(sorted []int) []int {
	out := sorted[:0]
	for _, value := range sorted {
		if len(out) == 0 || value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

// compressRanges renders sorted line numbers as "12-15, 22".
func compressRanges(lines []int) string {
	if len(lines) == 0 {
		return ""
	}
	var parts []string
	start, end := lines[0], lines[0]
	flush := func() {
		if start == end {
			parts = append(parts, strconv.Itoa(start))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", start, end))
		}
	}
	for _, lineNumber := range lines[1:] {
		if lineNumber == end+1 {
			end = lineNumber
			continue
		}
		flush()
		start, end = lineNumber, lineNumber
	}
	flush()
	return strings.Join(parts, ", ")
}

// shortenPackagePath turns a module-qualified package path into the form the
// reports show, with the module itself rendered as ".".
func shortenPackagePath(pkgPath, module string) string {
	if pkgPath == module {
		return "."
	}
	return strings.TrimPrefix(pkgPath, module+"/")
}
