// markdown.go renders the report CI posts as a sticky pull request comment.
// Everything it interpolates — file names, package names, the base branch
// name — comes from the pull request being reported on, so each value goes
// through the escaping helpers below before it reaches the comment.

package main

import (
	"fmt"
	"sort"
	"strings"
)

// stripBackticks removes the one character a markdown code span cannot hold.
// A branch or file name carrying a backtick would otherwise close the span
// and spill the rest of the report into running text. The character is
// replaced rather than dropped so the reader can see the name was altered.
func stripBackticks(value string) string {
	return strings.ReplaceAll(value, "`", "?")
}

// escapeCell wraps a value for a markdown table cell. An unescaped "|" would
// end the column and shift every cell after it.
func escapeCell(value string) string {
	// GitHub unescapes "\|" before it reads the code span, so the pipe
	// survives inside the backticks as a literal.
	return "`" + strings.ReplaceAll(stripBackticks(value), "|", `\|`) + "`"
}

// renderMarkdown builds the whole report. A nil base means no baseline was
// available, which drops the project delta; haveDiff reports whether a diff
// was supplied at all, so that a diff touching no coverable line still says
// "n/a" instead of omitting patch coverage.
func renderMarkdown(head, base profile, added map[string][]int, haveDiff bool,
	cfg config) string {
	var out strings.Builder
	baseName := stripBackticks(cfg.baseName)
	out.WriteString("<!-- devgo-coverage-report -->\n")
	out.WriteString("## Coverage report\n\n")
	if cfg.commit != "" {
		fmt.Fprintf(&out, "Commit: `%s`\n\n", stripBackticks(cfg.commit))
	}

	headTotal := sumProfileTally(head)
	projectLine := fmt.Sprintf("**Project:** %.1f%%", headTotal.percent())
	if base != nil {
		baseTotal := sumProfileTally(base)
		delta := headTotal.percent() - baseTotal.percent()
		projectLine += fmt.Sprintf(" (%s: %.1f%%, %+.1f%%)",
			baseName, baseTotal.percent(), delta)
	}

	patchLine := ""
	var patchStats []fileStat
	if haveDiff {
		patchStats = computePatchCoverage(head, added, cfg.module)
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

	out.WriteString(projectLine)
	if patchLine != "" {
		out.WriteString(" &nbsp;&nbsp; " + patchLine)
	}
	out.WriteString("\n\n")
	if base == nil {
		out.WriteString("_Baseline unavailable — project delta omitted._\n\n")
	}

	if base != nil {
		writePackageDelta(&out, head, base, cfg)
	}
	if len(patchStats) > 0 {
		writePatchTable(&out, patchStats, stripBackticks(cfg.baseRev))
	}

	out.WriteString("---\n")
	out.WriteString("To browse the full HTML report locally, check out this branch and run " +
		"`make test-coverage`, then open `coverage.html`. For a view of just these " +
		"changed lines, run `make coverage-diff BASE=" + stripBackticks(cfg.baseRev) + "`.\n")
	return out.String()
}

// writePackageDelta appends the collapsed table of packages whose displayed
// coverage moved. Packages that read the same on both sides are left out.
func writePackageDelta(out *strings.Builder, head, base profile, cfg config) {
	headPkgs := tallyPerPackage(head)
	basePkgs := tallyPerPackage(base)
	pkgSet := map[string]bool{}
	for pkgPath := range headPkgs {
		pkgSet[pkgPath] = true
	}
	for pkgPath := range basePkgs {
		pkgSet[pkgPath] = true
	}
	pkgPaths := make([]string, 0, len(pkgSet))
	for pkgPath := range pkgSet {
		pkgPaths = append(pkgPaths, pkgPath)
	}
	sort.Strings(pkgPaths)

	type row struct {
		name     string
		headPct  string
		basePct  string
		deltaPct string
	}
	var rows []row
	for _, pkgPath := range pkgPaths {
		headTally, inHead := headPkgs[pkgPath]
		baseTally, inBase := basePkgs[pkgPath]
		name := escapeCell(shortenPackagePath(pkgPath, cfg.module))
		headPct := fmt.Sprintf("%.1f%%", headTally.percent())
		basePct := fmt.Sprintf("%.1f%%", baseTally.percent())
		switch {
		case inHead && !inBase:
			rows = append(rows, row{name, headPct, "—", "new"})
		case !inHead && inBase:
			rows = append(rows, row{name, "—", basePct, "removed"})
		// Compare the rendered percentages, not the raw floats: a row whose
		// two columns read the same tells the reader nothing.
		case headPct != basePct:
			rows = append(rows, row{name, headPct, basePct,
				fmt.Sprintf("%+.1f%%", headTally.percent()-baseTally.percent())})
		}
	}
	if len(rows) == 0 {
		out.WriteString("_No per-package coverage changes._\n\n")
		return
	}

	out.WriteString("<details>\n<summary>Per-package coverage changes</summary>\n\n")
	fmt.Fprintf(out, "| Package | HEAD | %s | Δ |\n", escapeCell(cfg.baseName))
	out.WriteString("| --- | ---: | ---: | ---: |\n")
	for _, r := range rows {
		fmt.Fprintf(out, "| %s | %s | %s | %s |\n", r.name, r.headPct, r.basePct, r.deltaPct)
	}
	out.WriteString("\n</details>\n\n")
}

// maxPatchTableRows caps the only table that grows with the size of the pull
// request. A GitHub comment body over 65536 characters is rejected outright,
// and truncating the rendered markdown afterwards would cut the table inside
// its <details> block, hiding everything below it.
const maxPatchTableRows = 50

// writePatchTable appends the collapsed per-file patch coverage table.
// baseRev names the revision a reader passes to make coverage-diff, so the
// omission notice stays copy-pasteable on a pull request whose base is not
// main.
func writePatchTable(out *strings.Builder, stats []fileStat, baseRev string) {
	out.WriteString("<details>\n<summary>Patch coverage by file</summary>\n\n")
	out.WriteString("| File | Coverage | Statements | Uncovered new lines |\n")
	out.WriteString("| --- | ---: | ---: | --- |\n")
	shown := stats
	if len(shown) > maxPatchTableRows {
		shown = shown[:maxPatchTableRows]
	}
	for _, st := range shown {
		fmt.Fprintf(out, "| %s | %.1f%% | %d/%d | %s |\n",
			escapeCell(st.file), st.t.percent(), st.t.covered, st.t.total,
			compressRanges(st.uncovered))
	}
	if omitted := len(stats) - len(shown); omitted > 0 {
		fmt.Fprintf(out, "\n_%d more file(s) omitted; run `make coverage-diff BASE=%s` "+
			"for the full list._\n", omitted, baseRev)
	}
	out.WriteString("\n</details>\n\n")
}
