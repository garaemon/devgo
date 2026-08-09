// Command covreport turns Go coverage profiles into a markdown coverage
// report for pull requests: overall ("project") coverage with an optional
// delta against a base profile, and "patch" coverage over the lines added
// by a diff. It replaces the Codecov project/patch statuses with a report
// built entirely from go test -coverprofile output.
package main

import (
	"flag"
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

func main() {
	coverPath := flag.String("cover", "", "head coverage profile (required)")
	baseCoverPath := flag.String("base-cover", "", "base coverage profile for the project delta")
	diffPath := flag.String("diff", "", "unified diff (git diff -U0) for patch coverage")
	module := flag.String("module", "", "module path (default: read from go.mod)")
	commit := flag.String("commit", "", "head commit SHA to show in the report")
	baseName := flag.String("base-name", "main", "display name of the base branch")
	flag.Parse()

	if *coverPath == "" {
		fmt.Fprintln(os.Stderr, "covreport: -cover is required")
		os.Exit(2)
	}
	if *module == "" {
		modulePath, err := moduleFromGoMod("go.mod")
		if err != nil {
			fmt.Fprintf(os.Stderr, "covreport: cannot determine module path: %v\n", err)
			os.Exit(2)
		}
		*module = modulePath
	}

	head, err := parseProfile(*coverPath, *module)
	if err != nil {
		fmt.Fprintf(os.Stderr, "covreport: %v\n", err)
		os.Exit(1)
	}

	var base profile
	if *baseCoverPath != "" {
		base, err = parseProfile(*baseCoverPath, *module)
		if err != nil {
			fmt.Fprintf(os.Stderr, "covreport: %v\n", err)
			os.Exit(1)
		}
	}

	var added map[string][]int
	haveDiff := false
	if *diffPath != "" {
		added, err = parseDiff(*diffPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "covreport: %v\n", err)
			os.Exit(1)
		}
		haveDiff = true
	}

	fmt.Print(renderMarkdown(head, base, added, haveDiff, *module, *commit, *baseName))
}

func moduleFromGoMod(goModPath string) (string, error) {
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

// A coverage profile is the text file written by go test -coverprofile. The
// first line is a mode header and every line after it describes one coverage
// block:
//
//	mode: set
//	github.com/garaemon/devgo/pkg/config/config.go:58.39,59.52 1 1
//	github.com/garaemon/devgo/pkg/config/config.go:59.52,61.3 1 0
//
// A block line has the shape
//
//	<import path>/<file>.go:<startLine>.<startCol>,<endLine>.<endCol> <numStmts> <count>
//
// where numStmts is how many statements the block contains and count is how
// often it ran, so 0 means uncovered. The header says what count means: under
// "set" it is only 0 or 1, while "count" and "atomic" hold real execution
// counts. This tool only asks whether count is nonzero, so all three modes
// parse identically and the header is skipped.
//
// A block is a straight-line run of statements rather than a line range,
// which is why the positions carry columns and why a block can begin and end
// part-way through a line — above, line 59 ends the first block and starts
// the second. covreport works at line granularity and ignores the columns, so
// a single line may belong to several blocks. Blocks for one file are not
// guaranteed to be contiguous in the profile either, hence the append below
// rather than a single assignment.
//
// parseProfile reads such a profile, dropping files that are not coverable
// (tooling under tools/).
func parseProfile(profilePath, module string) (profile, error) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, err
	}

	coverageByFile := profile{}
	for lineIndex, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		file, coverageBlock, err := parseProfileLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %v", profilePath, lineIndex+1, err)
		}
		if !coverableFile(strings.TrimPrefix(file, module+"/")) {
			continue
		}
		coverageByFile[file] = append(coverageByFile[file], coverageBlock)
	}
	return coverageByFile, nil
}

// parseProfileLine splits one block line into its file name and block. The
// name is taken as everything before the *last* colon: it is a full import
// path, so only the trailing position field has a fixed shape.
func parseProfileLine(line string) (string, block, error) {
	lastColon := strings.LastIndex(line, ":")
	if lastColon < 0 {
		return "", block{}, fmt.Errorf("malformed profile line %q", line)
	}
	file := line[:lastColon]
	var startLine, startCol, endLine, endCol, numStmts, count int
	fieldsScanned, err := fmt.Sscanf(line[lastColon+1:], "%d.%d,%d.%d %d %d",
		&startLine, &startCol, &endLine, &endCol, &numStmts, &count)
	if err != nil || fieldsScanned != 6 {
		return "", block{}, fmt.Errorf("malformed profile line %q", line)
	}
	return file, block{
		startLine: startLine,
		endLine:   endLine,
		numStmts:  numStmts,
		count:     count,
	}, nil
}

// The diff is the unified-diff text written by git diff -U0. It is a sequence
// of per-file sections, each a few header lines followed by hunks:
//
//	diff --git a/pkg/config/config.go b/pkg/config/config.go
//	index 1a2b3c4..5d6e7f8 100644
//	--- a/pkg/config/config.go
//	+++ b/pkg/config/config.go
//	@@ -58,0 +59,3 @@ func Load() (*Config, error) {
//	+	if err := validate(cfg); err != nil {
//	+		return nil, err
//	+	}
//
// Only two of those line kinds carry what patch coverage needs. The "+++ "
// line names the file on the new side, normally prefixed with "b/" and
// possibly followed by a tab and a timestamp; it reads "/dev/null" when the
// file was deleted, in which case the section adds nothing. Hunk headers have
// the shape
//
//	@@ -<oldStart>[,<oldCount>] +<newStart>[,<newCount>] @@ [section heading]
//
// where an omitted count means 1 and a count of 0 means the hunk touches
// nothing on that side (a pure deletion has "+<newStart>,0").
//
// Because -U0 emits no context lines, every line inside a hunk is an added or
// removed line, so the new-side range of the header *is* the set of added
// line numbers — the parser reads the headers alone and never looks at hunk
// bodies. That also means a section with no hunks at all, as produced by a
// pure rename or mode change, contributes nothing. The flip side of matching
// on line prefixes is that an added line whose own text starts with "+++ "
// would be mistaken for a file header; Go sources do not produce such lines
// in practice.
//
// parseDiff extracts the added-line numbers per new-side file path. Only
// non-test .go files outside test/ are kept.
func parseDiff(diffPath string) (map[string][]int, error) {
	data, err := os.ReadFile(diffPath)
	if err != nil {
		return nil, err
	}
	return parseDiffBytes(data, diffPath)
}

// parseDiffBytes is parseDiff over an in-memory diff; source only names the
// input in error messages.
func parseDiffBytes(data []byte, source string) (map[string][]int, error) {
	added := map[string][]int{}
	currentFile := ""
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			currentFile = ""
			name := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			if tabIndex := strings.IndexByte(name, '\t'); tabIndex >= 0 {
				name = name[:tabIndex]
			}
			if name == "/dev/null" {
				continue
			}
			name = strings.TrimPrefix(name, "b/")
			if coverableFile(name) {
				currentFile = name
			}
		case strings.HasPrefix(line, "@@ ") && currentFile != "":
			startLine, lineCount, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", source, err)
			}
			for offset := 0; offset < lineCount; offset++ {
				added[currentFile] = append(added[currentFile], startLine+offset)
			}
		}
	}
	return added, nil
}

// coverableFile reports whether a repository-relative path is one whose
// coverage the report should account for. It is the single definition of
// "code that counts", applied to both inputs — profile entries and diff
// hunks — so a file can never be counted on one side and missing on the
// other, which would make patch coverage disagree with the project total.
//
// Excluded are non-Go files, which have no statements to cover; test sources
// (`_test.go` and everything under `test/`), which are the tests themselves
// rather than the code under test; and `tools/`, whose helpers — covreport
// among them — support the build rather than ship in the binary.
func coverableFile(name string) bool {
	return strings.HasSuffix(name, ".go") &&
		!strings.HasSuffix(name, "_test.go") &&
		!strings.HasPrefix(name, "test/") &&
		!strings.HasPrefix(name, "tools/")
}

// parseHunkHeader returns the new-side start line and line count from a
// "@@ -a,b +c,d @@" header ("+c" alone means one line, "+c,0" means none).
func parseHunkHeader(line string) (startLine, lineCount int, err error) {
	fields := strings.Fields(line)
	for _, field := range fields {
		if !strings.HasPrefix(field, "+") {
			continue
		}
		newSideSpec := strings.TrimPrefix(field, "+")
		lineCount = 1
		if commaIndex := strings.IndexByte(newSideSpec, ','); commaIndex >= 0 {
			lineCount, err = strconv.Atoi(newSideSpec[commaIndex+1:])
			if err != nil {
				return 0, 0, fmt.Errorf("malformed hunk header %q", line)
			}
			newSideSpec = newSideSpec[:commaIndex]
		}
		startLine, err = strconv.Atoi(newSideSpec)
		if err != nil {
			return 0, 0, fmt.Errorf("malformed hunk header %q", line)
		}
		return startLine, lineCount, nil
	}
	return 0, 0, fmt.Errorf("malformed hunk header %q", line)
}

// tally is a running count of statements over some scope — a file, a package,
// a patch or the whole project — used for every percentage in the report.
//
// The unit is statements, not lines, because that is what the profile counts:
// each block carries a statement count, and a block either ran or it did not.
// So total is the statements seen and covered the subset in blocks that ran,
// which makes coverage the ratio of the two and keeps a dense one-liner worth
// more than a line of punctuation. A tally with total 0 means nothing
// coverable was in scope, and reports as "n/a" rather than 0%.
type tally struct {
	covered int
	total   int
}

func (counts tally) percent() float64 {
	if counts.total == 0 {
		return 0
	}
	return 100 * float64(counts.covered) / float64(counts.total)
}

// add folds one coverage block into the tally. A block counts as covered in
// full or not at all, since the profile records a single execution count for
// all of its statements.
func (counts *tally) add(coverageBlock block) {
	counts.total += coverageBlock.numStmts
	if coverageBlock.count > 0 {
		counts.covered += coverageBlock.numStmts
	}
}

// aggregateByPackage sums a profile into one statement tally per package.
func aggregateByPackage(coverage profile) map[string]tally {
	packages := map[string]tally{}
	for file, blocks := range coverage {
		packageName := path.Dir(file)
		packageTally := packages[packageName]
		for _, coverageBlock := range blocks {
			packageTally.add(coverageBlock)
		}
		packages[packageName] = packageTally
	}
	return packages
}

// aggregateTotal sums a profile into a single statement tally for the whole
// module.
func aggregateTotal(coverage profile) tally {
	var total tally
	for _, blocks := range coverage {
		for _, coverageBlock := range blocks {
			total.add(coverageBlock)
		}
	}
	return total
}

// fileStat is one file's patch coverage: statements of blocks that overlap
// added lines, how many of those statements ran, and the added lines left
// uncovered.
type fileStat struct {
	file      string
	coverage  tally
	uncovered []int
}

func patchCoverage(head profile, added map[string][]int, module string) []fileStat {
	var stats []fileStat
	files := make([]string, 0, len(added))
	for file := range added {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		addedLines := added[file]
		isAddedLine := map[int]bool{}
		for _, line := range addedLines {
			isAddedLine[line] = true
		}
		blocks := head[module+"/"+file]
		var stat fileStat
		stat.file = file
		for _, coverageBlock := range blocks {
			touched := false
			for line := coverageBlock.startLine; line <= coverageBlock.endLine; line++ {
				if isAddedLine[line] {
					touched = true
					break
				}
			}
			if !touched {
				continue
			}
			stat.coverage.add(coverageBlock)
			if coverageBlock.count == 0 {
				for line := coverageBlock.startLine; line <= coverageBlock.endLine; line++ {
					if isAddedLine[line] {
						stat.uncovered = append(stat.uncovered, line)
					}
				}
			}
		}
		if stat.coverage.total > 0 {
			sort.Ints(stat.uncovered)
			stat.uncovered = dedupInts(stat.uncovered)
			stats = append(stats, stat)
		}
	}
	return stats
}

func dedupInts(sorted []int) []int {
	deduped := sorted[:0]
	previous := -1
	for _, value := range sorted {
		if value != previous {
			deduped = append(deduped, value)
		}
		previous = value
	}
	return deduped
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
	for _, line := range lines[1:] {
		if line == end+1 {
			end = line
			continue
		}
		flush()
		start, end = line, line
	}
	flush()
	return strings.Join(parts, ", ")
}

func shortPkg(pkg, module string) string {
	if pkg == module {
		return "."
	}
	return strings.TrimPrefix(pkg, module+"/")
}

func renderMarkdown(head, base profile, added map[string][]int, haveDiff bool,
	module, commit, baseName string) string {
	var report strings.Builder
	report.WriteString("<!-- devgo-coverage-report -->\n")
	report.WriteString("## Coverage report\n\n")
	if commit != "" {
		fmt.Fprintf(&report, "Commit: `%s`\n\n", commit)
	}

	headTotal := aggregateTotal(head)
	projectLine := fmt.Sprintf("**Project:** %.1f%%", headTotal.percent())
	if base != nil {
		baseTotal := aggregateTotal(base)
		delta := headTotal.percent() - baseTotal.percent()
		projectLine += fmt.Sprintf(" (%s: %.1f%%, %+.1f%%)", baseName, baseTotal.percent(), delta)
	}

	patchLine := ""
	var patchStats []fileStat
	if haveDiff {
		patchStats = patchCoverage(head, added, module)
		var patchTotal tally
		for _, stat := range patchStats {
			patchTotal.covered += stat.coverage.covered
			patchTotal.total += stat.coverage.total
		}
		if patchTotal.total == 0 {
			patchLine = "**Patch:** n/a (no coverable changes)"
		} else {
			patchLine = fmt.Sprintf("**Patch:** %.1f%% (%d/%d statements)",
				patchTotal.percent(), patchTotal.covered, patchTotal.total)
		}
	}

	report.WriteString(projectLine)
	if patchLine != "" {
		report.WriteString(" &nbsp;&nbsp; " + patchLine)
	}
	report.WriteString("\n\n")
	if base == nil {
		report.WriteString("_Baseline unavailable — project delta omitted._\n\n")
	}

	if base != nil {
		writePackageDelta(&report, head, base, module, baseName)
	}
	if len(patchStats) > 0 {
		writePatchTable(&report, patchStats)
	}

	report.WriteString("---\n")
	report.WriteString("To browse the full HTML report locally, check out this branch and run " +
		"`make test-coverage`, then open `coverage.html`.\n")
	return report.String()
}

func writePackageDelta(report *strings.Builder, head, base profile, module, baseName string) {
	headPackages := aggregateByPackage(head)
	basePackages := aggregateByPackage(base)
	seenPackages := map[string]bool{}
	for packageName := range headPackages {
		seenPackages[packageName] = true
	}
	for packageName := range basePackages {
		seenPackages[packageName] = true
	}
	packages := make([]string, 0, len(seenPackages))
	for packageName := range seenPackages {
		packages = append(packages, packageName)
	}
	sort.Strings(packages)

	type row struct {
		name     string
		headPct  string
		basePct  string
		deltaPct string
	}
	var rows []row
	for _, packageName := range packages {
		headTally, inHead := headPackages[packageName]
		baseTally, inBase := basePackages[packageName]
		switch {
		case inHead && !inBase:
			rows = append(rows, row{shortPkg(packageName, module),
				fmt.Sprintf("%.1f%%", headTally.percent()), "—", "new"})
		case !inHead && inBase:
			rows = append(rows, row{shortPkg(packageName, module),
				"—", fmt.Sprintf("%.1f%%", baseTally.percent()), "removed"})
		case headTally.percent() != baseTally.percent():
			rows = append(rows, row{shortPkg(packageName, module),
				fmt.Sprintf("%.1f%%", headTally.percent()),
				fmt.Sprintf("%.1f%%", baseTally.percent()),
				fmt.Sprintf("%+.1f%%", headTally.percent()-baseTally.percent())})
		}
	}
	if len(rows) == 0 {
		report.WriteString("_No per-package coverage changes._\n\n")
		return
	}

	report.WriteString("<details>\n<summary>Per-package coverage changes</summary>\n\n")
	fmt.Fprintf(report, "| Package | HEAD | %s | Δ |\n", baseName)
	report.WriteString("| --- | ---: | ---: | ---: |\n")
	for _, packageRow := range rows {
		fmt.Fprintf(report, "| %s | %s | %s | %s |\n",
			packageRow.name, packageRow.headPct, packageRow.basePct, packageRow.deltaPct)
	}
	report.WriteString("\n</details>\n\n")
}

func writePatchTable(report *strings.Builder, stats []fileStat) {
	report.WriteString("<details>\n<summary>Patch coverage by file</summary>\n\n")
	report.WriteString("| File | Coverage | Statements | Uncovered new lines |\n")
	report.WriteString("| --- | ---: | ---: | --- |\n")
	for _, stat := range stats {
		fmt.Fprintf(report, "| %s | %.1f%% | %d/%d | %s |\n",
			stat.file, stat.coverage.percent(), stat.coverage.covered, stat.coverage.total,
			compressRanges(stat.uncovered))
	}
	report.WriteString("\n</details>\n\n")
}
