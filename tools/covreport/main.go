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
	format := flag.String("format", "markdown",
		"output format: markdown (PR report) or html (annotated source browser)")
	flag.Parse()

	if *coverPath == "" {
		fmt.Fprintln(os.Stderr, "covreport: -cover is required")
		os.Exit(2)
	}
	if *module == "" {
		m, err := moduleFromGoMod("go.mod")
		if err != nil {
			fmt.Fprintf(os.Stderr, "covreport: cannot determine module path: %v\n", err)
			os.Exit(2)
		}
		*module = m
	}

	head, err := parseProfile(*coverPath, *module)
	if err != nil {
		fmt.Fprintf(os.Stderr, "covreport: %v\n", err)
		os.Exit(1)
	}

	if *format == "html" {
		if err := renderHTML(os.Stdout, head, *module, *commit, nil); err != nil {
			fmt.Fprintf(os.Stderr, "covreport: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *format != "markdown" {
		fmt.Fprintf(os.Stderr, "covreport: unknown format %q\n", *format)
		os.Exit(2)
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

func moduleFromGoMod(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("no module directive in %s", path)
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
func parseProfile(path, module string) (profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	p := profile{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		file, b, err := parseProfileLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, i+1, err)
		}
		if !coverableFile(strings.TrimPrefix(file, module+"/")) {
			continue
		}
		p[file] = append(p[file], b)
	}
	return p, nil
}

// parseProfileLine splits one block line into its file name and block. The
// name is taken as everything before the *last* colon: it is a full import
// path, so only the trailing position field has a fixed shape.
func parseProfileLine(line string) (string, block, error) {
	colon := strings.LastIndex(line, ":")
	if colon < 0 {
		return "", block{}, fmt.Errorf("malformed profile line %q", line)
	}
	file := line[:colon]
	var sl, sc, el, ec, stmts, count int
	n, err := fmt.Sscanf(line[colon+1:], "%d.%d,%d.%d %d %d", &sl, &sc, &el, &ec, &stmts, &count)
	if err != nil || n != 6 {
		return "", block{}, fmt.Errorf("malformed profile line %q", line)
	}
	return file, block{startLine: sl, endLine: el, numStmts: stmts, count: count}, nil
}

// parseDiff extracts the added-line numbers per new-side file path from a
// unified diff. Only non-test .go files outside test/ are kept.
func parseDiff(path string) (map[string][]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseDiffBytes(data, path)
}

// parseDiffBytes is parseDiff over an in-memory diff; source only names the
// input in error messages.
func parseDiffBytes(data []byte, source string) (map[string][]int, error) {
	added := map[string][]int{}
	current := ""
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			current = ""
			name := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			if i := strings.IndexByte(name, '\t'); i >= 0 {
				name = name[:i]
			}
			if name == "/dev/null" {
				continue
			}
			name = strings.TrimPrefix(name, "b/")
			if coverableFile(name) {
				current = name
			}
		case strings.HasPrefix(line, "@@ ") && current != "":
			start, count, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", source, err)
			}
			for i := 0; i < count; i++ {
				added[current] = append(added[current], start+i)
			}
		}
	}
	return added, nil
}

func coverableFile(name string) bool {
	return strings.HasSuffix(name, ".go") &&
		!strings.HasSuffix(name, "_test.go") &&
		!strings.HasPrefix(name, "test/") &&
		!strings.HasPrefix(name, "tools/")
}

// parseHunkHeader returns the new-side start line and line count from a
// "@@ -a,b +c,d @@" header ("+c" alone means one line, "+c,0" means none).
func parseHunkHeader(line string) (start, count int, err error) {
	fields := strings.Fields(line)
	for _, f := range fields {
		if !strings.HasPrefix(f, "+") {
			continue
		}
		spec := strings.TrimPrefix(f, "+")
		count = 1
		if i := strings.IndexByte(spec, ','); i >= 0 {
			count, err = strconv.Atoi(spec[i+1:])
			if err != nil {
				return 0, 0, fmt.Errorf("malformed hunk header %q", line)
			}
			spec = spec[:i]
		}
		start, err = strconv.Atoi(spec)
		if err != nil {
			return 0, 0, fmt.Errorf("malformed hunk header %q", line)
		}
		return start, count, nil
	}
	return 0, 0, fmt.Errorf("malformed hunk header %q", line)
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

func (t *tally) add(b block) {
	t.total += b.numStmts
	if b.count > 0 {
		t.covered += b.numStmts
	}
}

// perPackage aggregates a profile into per-package statement tallies.
func perPackage(p profile) map[string]tally {
	pkgs := map[string]tally{}
	for file, blocks := range p {
		pkg := path.Dir(file)
		t := pkgs[pkg]
		for _, b := range blocks {
			t.add(b)
		}
		pkgs[pkg] = t
	}
	return pkgs
}

func totalTally(p profile) tally {
	var t tally
	for _, blocks := range p {
		for _, b := range blocks {
			t.add(b)
		}
	}
	return t
}

// patchTally computes per-file patch coverage: statements of blocks that
// overlap added lines, and how many of those statements ran.
type fileStat struct {
	file      string
	t         tally
	uncovered []int
}

func patchCoverage(head profile, added map[string][]int, module string) []fileStat {
	var stats []fileStat
	files := make([]string, 0, len(added))
	for f := range added {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, file := range files {
		lines := added[file]
		lineSet := map[int]bool{}
		for _, l := range lines {
			lineSet[l] = true
		}
		blocks := head[module+"/"+file]
		var st fileStat
		st.file = file
		for _, b := range blocks {
			touched := false
			for l := b.startLine; l <= b.endLine; l++ {
				if lineSet[l] {
					touched = true
					break
				}
			}
			if !touched {
				continue
			}
			st.t.add(b)
			if b.count == 0 {
				for l := b.startLine; l <= b.endLine; l++ {
					if lineSet[l] {
						st.uncovered = append(st.uncovered, l)
					}
				}
			}
		}
		if st.t.total > 0 {
			sort.Ints(st.uncovered)
			st.uncovered = dedupInts(st.uncovered)
			stats = append(stats, st)
		}
	}
	return stats
}

func dedupInts(s []int) []int {
	out := s[:0]
	prev := -1
	for _, v := range s {
		if v != prev {
			out = append(out, v)
		}
		prev = v
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
	for _, l := range lines[1:] {
		if l == end+1 {
			end = l
			continue
		}
		flush()
		start, end = l, l
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
	var b strings.Builder
	b.WriteString("<!-- devgo-coverage-report -->\n")
	b.WriteString("## Coverage report\n\n")
	if commit != "" {
		fmt.Fprintf(&b, "Commit: `%s`\n\n", commit)
	}

	headTotal := totalTally(head)
	projectLine := fmt.Sprintf("**Project:** %.1f%%", headTotal.percent())
	if base != nil {
		baseTotal := totalTally(base)
		delta := headTotal.percent() - baseTotal.percent()
		projectLine += fmt.Sprintf(" (%s: %.1f%%, %+.1f%%)", baseName, baseTotal.percent(), delta)
	}

	patchLine := ""
	var patchStats []fileStat
	if haveDiff {
		patchStats = patchCoverage(head, added, module)
		var patchTotal tally
		for _, st := range patchStats {
			patchTotal.covered += st.t.covered
			patchTotal.total += st.t.total
		}
		if patchTotal.total == 0 {
			patchLine = "**Patch:** n/a (no coverable changes)"
		} else {
			patchLine = fmt.Sprintf("**Patch:** %.1f%% (%d/%d statements)",
				patchTotal.percent(), patchTotal.covered, patchTotal.total)
		}
	}

	b.WriteString(projectLine)
	if patchLine != "" {
		b.WriteString(" &nbsp;&nbsp; " + patchLine)
	}
	b.WriteString("\n\n")
	if base == nil {
		b.WriteString("_Baseline unavailable — project delta omitted._\n\n")
	}

	if base != nil {
		writePackageDelta(&b, head, base, module, baseName)
	}
	if len(patchStats) > 0 {
		writePatchTable(&b, patchStats)
	}

	b.WriteString("---\n")
	b.WriteString("To browse the full HTML report locally, check out this branch and run " +
		"`make test-coverage`, then open `coverage.html`.\n")
	return b.String()
}

func writePackageDelta(b *strings.Builder, head, base profile, module, baseName string) {
	headPkgs := perPackage(head)
	basePkgs := perPackage(base)
	pkgSet := map[string]bool{}
	for p := range headPkgs {
		pkgSet[p] = true
	}
	for p := range basePkgs {
		pkgSet[p] = true
	}
	pkgs := make([]string, 0, len(pkgSet))
	for p := range pkgSet {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	type row struct {
		name     string
		headPct  string
		basePct  string
		deltaPct string
	}
	var rows []row
	for _, p := range pkgs {
		ht, inHead := headPkgs[p]
		bt, inBase := basePkgs[p]
		switch {
		case inHead && !inBase:
			rows = append(rows, row{shortPkg(p, module),
				fmt.Sprintf("%.1f%%", ht.percent()), "—", "new"})
		case !inHead && inBase:
			rows = append(rows, row{shortPkg(p, module),
				"—", fmt.Sprintf("%.1f%%", bt.percent()), "removed"})
		case ht.percent() != bt.percent():
			rows = append(rows, row{shortPkg(p, module),
				fmt.Sprintf("%.1f%%", ht.percent()),
				fmt.Sprintf("%.1f%%", bt.percent()),
				fmt.Sprintf("%+.1f%%", ht.percent()-bt.percent())})
		}
	}
	if len(rows) == 0 {
		b.WriteString("_No per-package coverage changes._\n\n")
		return
	}

	b.WriteString("<details>\n<summary>Per-package coverage changes</summary>\n\n")
	fmt.Fprintf(b, "| Package | HEAD | %s | Δ |\n", baseName)
	b.WriteString("| --- | ---: | ---: | ---: |\n")
	for _, r := range rows {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", r.name, r.headPct, r.basePct, r.deltaPct)
	}
	b.WriteString("\n</details>\n\n")
}

func writePatchTable(b *strings.Builder, stats []fileStat) {
	b.WriteString("<details>\n<summary>Patch coverage by file</summary>\n\n")
	b.WriteString("| File | Coverage | Statements | Uncovered new lines |\n")
	b.WriteString("| --- | ---: | ---: | --- |\n")
	for _, st := range stats {
		fmt.Fprintf(b, "| %s | %.1f%% | %d/%d | %s |\n",
			st.file, st.t.percent(), st.t.covered, st.t.total, compressRanges(st.uncovered))
	}
	b.WriteString("\n</details>\n\n")
}
