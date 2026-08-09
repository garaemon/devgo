package main

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"sort"
	"strings"
)

// sourceReader returns the contents of a repository-relative source file.
type sourceReader func(relPath string) ([]byte, error)

// htmlOptions configures renderHTML. When Added is non-empty the report gains
// a diff view alongside the full listing, toggled client-side.
type htmlOptions struct {
	Module   string
	Commit   string
	Added    map[string][]int // repo-relative path -> added line numbers
	HaveDiff bool
	DiffBase string
	DiffHead string
	DiffFile string // set when the diff came from -diff instead of a revision
	Context  int
	Source   sourceReader
}

// renderHTML writes a self-contained coverage browser: a sidebar listing every
// file with its coverage percentage and a main pane showing the annotated
// source. It replaces the hard-to-read default theme of `go tool cover -html`.
//
// When a diff is supplied the page carries both views — the changed hunks with
// surrounding context, and the complete listing — and switches between them by
// toggling a class on <body>, so the source text is emitted only once.
func renderHTML(out io.Writer, coverage profile, options htmlOptions) error {
	if options.Source == nil {
		options.Source = func(relPath string) ([]byte, error) { return os.ReadFile(relPath) }
	}

	// Changed files that never appear in the profile still belong in the
	// report, so the path set is the union of both sides.
	inReport := map[string]bool{}
	for profileKey := range coverage {
		inReport[profileKey] = true
	}
	if options.HaveDiff {
		for changedFile := range options.Added {
			inReport[options.Module+"/"+changedFile] = true
		}
	}
	profileKeys := make([]string, 0, len(inReport))
	for profileKey := range inReport {
		profileKeys = append(profileKeys, profileKey)
	}
	sort.Strings(profileKeys)

	tallies := patchTallies(coverage, options.Added, options.Module)

	var files []htmlFile
	changedCount := 0
	for _, profileKey := range profileKeys {
		relPath := strings.TrimPrefix(profileKey, options.Module+"/")
		sourceText, err := options.Source(relPath)
		if err != nil {
			// Sources can be missing when the profile came from another
			// commit; skip the file rather than failing the whole report.
			fmt.Fprintf(os.Stderr, "covreport: skipping %s: %v\n", relPath, err)
			continue
		}
		addedLines := options.Added[relPath]
		sort.Ints(addedLines)
		addedLines = dedupInts(addedLines)

		file := annotateFile(relPath, string(sourceText), coverage[profileKey],
			addedLines, options.Context)
		if file.Changed {
			changedCount++
			// Patch numbers come from the profile, not the annotated source,
			// so the header always matches the markdown report.
			patchTally := tallies[relPath]
			file.PatchCovered, file.PatchTotal = patchTally.covered, patchTally.total
			file.PatchPercent = patchTally.percent()
		}
		file.PatchLabel = "n/a"
		if file.PatchTotal > 0 {
			file.PatchLabel = fmt.Sprintf("%.1f%%", file.PatchPercent)
		}
		files = append(files, file)
	}

	total := totalTally(coverage)
	data := htmlData{
		Module:       options.Module,
		Commit:       options.Commit,
		Percent:      total.percent(),
		Diff:         options.HaveDiff,
		BodyClass:    "mode-all",
		RangeLabel:   rangeLabel(options),
		PatchLabel:   "n/a",
		ChangedFiles: changedCount,
		Files:        files,
	}
	if options.HaveDiff && changedCount > 0 {
		data.BodyClass = "mode-diff"
	}
	var patchTotal tally
	for _, file := range files {
		patchTotal.covered += file.PatchCovered
		patchTotal.total += file.PatchTotal
	}
	if patchTotal.total > 0 {
		data.PatchLabel = fmt.Sprintf("%.1f%% (%d/%d statements)",
			patchTotal.percent(), patchTotal.covered, patchTotal.total)
	}
	return htmlTemplate.Execute(out, data)
}

// rangeLabel describes what the diff was taken against, for the header.
func rangeLabel(options htmlOptions) string {
	switch {
	case options.DiffBase != "":
		head := options.DiffHead
		if head == "" {
			head = "working tree"
		}
		return options.DiffBase + " … " + head
	case options.DiffFile != "":
		return options.DiffFile
	}
	return ""
}

type htmlData struct {
	Module       string
	Commit       string
	Percent      float64
	Diff         bool
	BodyClass    string
	RangeLabel   string
	PatchLabel   string
	ChangedFiles int
	Files        []htmlFile
}

type htmlFile struct {
	Path         string
	ID           string
	Percent      float64
	Lines        []htmlLine
	Changed      bool
	PatchPercent float64
	PatchCovered int
	PatchTotal   int
	PatchLabel   string
}

// htmlLine is one rendered row. Coverage, addedness and diff-view visibility
// are kept separate so the template composes the CSS classes and the tests can
// assert each property on its own. A row with Gap > 0 is an elision separator
// rather than a source line.
type htmlLine struct {
	Num     int
	Class   string // "", "cov", or "uncov"
	Text    template.HTML
	Added   bool
	Visible bool
	Gap     int
}

// region is an inclusive run of source lines.
type region struct{ start, end int }

// diffRegions returns the line runs visible in the diff view: every added line
// grown by context lines on each side, clamped to [1,totalLines] and merged
// when the expanded runs touch or overlap. addedLines must be sorted and
// deduped.
func diffRegions(addedLines []int, totalLines, context int) []region {
	if totalLines <= 0 || len(addedLines) == 0 {
		return nil
	}
	if context < 0 {
		context = 0
	}
	var regions []region
	for _, line := range addedLines {
		if line < 1 || line > totalLines {
			continue // profile/source skew, or a line past EOF
		}
		start, end := line-context, line+context
		if start < 1 {
			start = 1
		}
		if end > totalLines {
			end = totalLines
		}
		// Merge on touching, not just overlapping, so no "0 lines" separator
		// is ever emitted between adjacent windows.
		if count := len(regions); count > 0 && start <= regions[count-1].end+1 {
			if end > regions[count-1].end {
				regions[count-1].end = end
			}
			continue
		}
		regions = append(regions, region{start, end})
	}
	return regions
}

// annotateFile classifies each source line and, when addedLines is non-empty,
// marks the diff-view context windows and inserts elision separators between
// them. Uncovered wins over covered when a line spans blocks of both kinds, so
// red always flags lines that still need a test.
func annotateFile(relPath, sourceText string, blocks []block,
	addedLines []int, context int) htmlFile {
	sourceLines := highlightLines(relPath, sourceText)
	totalLines := len(sourceLines)

	classByLine := make([]string, totalLines+1)
	var fileTally tally
	for _, coverageBlock := range blocks {
		fileTally.add(coverageBlock)
		class := "cov"
		if coverageBlock.count == 0 {
			class = "uncov"
		}
		for line := coverageBlock.startLine; line <= coverageBlock.endLine &&
			line < len(classByLine); line++ {
			if classByLine[line] != "uncov" {
				classByLine[line] = class
			}
		}
	}

	file := htmlFile{
		Path:    relPath,
		ID:      strings.NewReplacer("/", "-", ".", "-").Replace(relPath),
		Percent: fileTally.percent(),
		Changed: len(addedLines) > 0,
	}

	isAddedLine := map[int]bool{}
	isVisible := make([]bool, totalLines+1)
	if !file.Changed {
		for index := range isVisible {
			isVisible[index] = true
		}
	} else {
		for _, line := range addedLines {
			isAddedLine[line] = true
		}
		for _, visibleRegion := range diffRegions(addedLines, totalLines, context) {
			for line := visibleRegion.start; line <= visibleRegion.end; line++ {
				isVisible[line] = true
			}
		}
	}

	// Hidden lines stay in the slice — they are what the all-files view
	// renders — and only the separator rows are new.
	hiddenCount := 0
	for index, text := range sourceLines {
		lineNumber := index + 1
		if !isVisible[lineNumber] {
			hiddenCount++
			file.Lines = append(file.Lines, htmlLine{
				Num: lineNumber, Class: classByLine[lineNumber], Text: text,
			})
			continue
		}
		if hiddenCount > 0 {
			file.Lines = append(file.Lines, htmlLine{Gap: hiddenCount})
			hiddenCount = 0
		}
		file.Lines = append(file.Lines, htmlLine{
			Num: lineNumber, Class: classByLine[lineNumber], Text: text,
			Added: isAddedLine[lineNumber], Visible: true,
		})
	}
	if hiddenCount > 0 {
		file.Lines = append(file.Lines, htmlLine{Gap: hiddenCount})
	}
	return file
}

var htmlTemplate = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Module}} coverage</title>
<style>
:root {
  --bg: #ffffff; --fg: #1f2328; --muted: #656d76;
  --panel: #f6f8fa; --border: #d0d7de; --accent: #0969da;
  --cov-bg: #e6f4ea; --cov-edge: #1a7f37;
  --uncov-bg: #ffebe9; --uncov-edge: #cf222e;
  --add-bg: #eef4ff; --add-edge: #0969da; --gap-fg: #656d76;
  --tk-key: #cf222e; --tk-str: #0a3069; --tk-num: #0550ae;
  --tk-com: #57606a; --tk-bui: #0550ae; --tk-fun: #6639ba;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0d1117; --fg: #e6edf3; --muted: #8d96a0;
    --panel: #161b22; --border: #30363d; --accent: #4493f8;
    --cov-bg: #12261e; --cov-edge: #2ea043;
    --uncov-bg: #2d1517; --uncov-edge: #f85149;
    --add-bg: #10203a; --add-edge: #4493f8; --gap-fg: #8d96a0;
    --tk-key: #ff7b72; --tk-str: #a5d6ff; --tk-num: #79c0ff;
    --tk-com: #9198a1; --tk-bui: #79c0ff; --tk-fun: #d2a8ff;
  }
}
* { box-sizing: border-box; }
body {
  margin: 0; background: var(--bg); color: var(--fg);
  font: 14px/1.5 -apple-system, "Segoe UI", Roboto, sans-serif;
  display: flex; flex-direction: column; height: 100vh;
}
header {
  padding: 10px 16px; border-bottom: 1px solid var(--border);
  background: var(--panel); display: flex; gap: 16px; align-items: baseline;
  flex-wrap: wrap;
}
header h1 { font-size: 15px; margin: 0; }
header .total { color: var(--accent); font-weight: 600; }
header .meta { color: var(--muted); font-size: 12px; }
.modes { display: flex; gap: 6px; }
.modes button {
  font: inherit; font-size: 12px; padding: 2px 10px; cursor: pointer;
  background: var(--bg); color: var(--fg);
  border: 1px solid var(--border); border-radius: 4px;
}
.modes button.active {
  background: var(--accent); color: #fff; border-color: var(--accent);
}
.legend { margin-left: auto; font-size: 12px; color: var(--muted); }
.legend .cov, .legend .uncov { padding: 1px 8px; border-radius: 3px; }
.legend .cov { background: var(--cov-bg); }
.legend .uncov { background: var(--uncov-bg); }
.legend .plus { color: var(--add-edge); font-weight: 600; }
main { display: flex; flex: 1; min-height: 0; }
nav {
  width: 320px; overflow-y: auto; border-right: 1px solid var(--border);
  background: var(--panel); padding: 8px 0; flex-shrink: 0;
}
nav a {
  display: flex; justify-content: space-between; gap: 8px;
  padding: 4px 16px; color: var(--fg); text-decoration: none;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px; white-space: nowrap;
}
nav a:hover { background: var(--border); }
nav a.active { background: var(--accent); color: #fff; }
nav a.active .pct { color: #fff; }
nav .pct { color: var(--muted); }
section { display: none; overflow: auto; flex: 1; padding: 12px 0; }
section.active { display: block; }
section h2 {
  font-size: 13px; margin: 0 16px 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
pre {
  margin: 0; font: 12px/1.45 ui-monospace, SFMono-Regular, Menlo,
  Consolas, monospace; min-width: max-content; tab-size: 4;
}
/* Direct children only: the rows. Token spans nested inside must stay inline. */
pre > span { display: block; padding: 0 16px 0 0; }
pre .n {
  display: inline-block; width: 4.5em; text-align: right;
  padding-right: 12px; color: var(--muted); user-select: none;
}
pre .g {
  display: inline-block; width: 1.3em; text-align: center;
  color: var(--add-edge); user-select: none;
}
/* Tabs are measured from the start of the line box, so the code must live
   in its own inline-block for indentation to survive the number prefix. */
pre .t { display: inline-block; white-space: pre; vertical-align: top; }
/* Syntax colours, chosen to stay legible on the coverage tints. */
pre .t .k { color: var(--tk-key); }
pre .t .s { color: var(--tk-str); }
pre .t .m { color: var(--tk-num); }
pre .t .b { color: var(--tk-bui); }
pre .t .f { color: var(--tk-fun); }
pre .t .c { color: var(--tk-com); font-style: italic; }
pre .cov { background: var(--cov-bg); box-shadow: inset 3px 0 var(--cov-edge); }
pre .uncov { background: var(--uncov-bg); box-shadow: inset 3px 0 var(--uncov-edge); }
pre .gap {
  color: var(--gap-fg); background: var(--panel); font-size: 11px;
  padding: 2px 0 2px 16px; margin: 4px 0;
  border-top: 1px solid var(--border); border-bottom: 1px solid var(--border);
}
/* Two stacked inset bars: the first shadow paints on top, so 0-3px marks the
   line as added and 3-6px keeps its coverage colour. Spelled out per
   combination because .add already outranks the bare .cov/.uncov rules. */
body.mode-diff pre .add .g::before { content: "+"; font-weight: 600; }
body.mode-diff pre .add {
  background: var(--add-bg); box-shadow: inset 3px 0 var(--add-edge);
}
body.mode-diff pre .add.cov {
  background: var(--cov-bg);
  box-shadow: inset 3px 0 var(--add-edge), inset 6px 0 var(--cov-edge);
}
body.mode-diff pre .add.uncov {
  background: var(--uncov-bg);
  box-shadow: inset 3px 0 var(--add-edge), inset 6px 0 var(--uncov-edge);
}
/* Context lines keep their coverage colours; only the gutter dims, so the
   surrounding code stays fully legible. */
body.mode-diff pre span:not(.add) > .n { opacity: .55; }
body.mode-diff pre .off { display: none; }
body.mode-all pre .gap { display: none; }
body.mode-all .diff-only { display: none; }
body.mode-diff .all-only { display: none; }
body.mode-diff nav a:not(.changed) { display: none; }
body.mode-diff section:not(.changed) { display: none !important; }
.empty { color: var(--muted); padding: 24px; }
</style>
</head>
<body class="{{.BodyClass}}">
<header>
  <h1>{{.Module}}</h1>
  <span class="total">{{printf "%.1f" .Percent}}%</span>
  {{if .Commit}}<span class="meta">commit {{.Commit}}</span>{{end}}
  {{if .Diff}}<span class="meta diff-only">patch {{.PatchLabel}} · {{.RangeLabel}}</span>
  <span class="modes">
    <button type="button" data-mode="diff">Changed only</button>
    <button type="button" data-mode="all">All files</button>
  </span>{{end}}
  <span class="legend">
    <span class="cov">covered</span> <span class="uncov">not covered</span>
    plain: not tracked{{if .Diff}}
    <span class="diff-only"><span class="plus">+</span> added</span>{{end}}
  </span>
</header>
<main>
<nav>
{{range .Files}}<a href="#{{.ID}}" data-target="{{.ID}}"{{if .Changed}} class="changed"{{end}}>
  <span>{{.Path}}</span><span class="pct all-only">{{printf "%.1f" .Percent}}%</span>
  <span class="pct diff-only">{{.PatchLabel}}</span>
</a>
{{end}}</nav>
{{range .Files}}<section id="{{.ID}}"{{if .Changed}} class="changed"{{end}}>
<h2>{{.Path}} — <span class="all-only">{{printf "%.1f" .Percent}}%</span><span class="diff-only">patch {{.PatchLabel}}</span></h2>
<pre>{{range .Lines}}{{if .Gap}}<span class="gap">&hellip; {{.Gap}} unchanged lines &hellip;</span>{{else}}<span class="{{.Class}}{{if .Added}} add{{end}}{{if not .Visible}} off{{end}}"><span class="n">{{.Num}}</span><span class="g"></span><span class="t">{{.Text}}</span></span>{{end}}{{end}}</pre>
</section>
{{end}}{{if .Diff}}<p class="empty diff-only"{{if .ChangedFiles}} hidden{{end}}>No coverable changes in {{.RangeLabel}}.</p>{{end}}</main>
<script>
const fileLinks = document.querySelectorAll('nav a');
const showFile = fileId => {
  document.querySelectorAll('section').forEach(section =>
    section.classList.toggle('active', section.id === fileId));
  fileLinks.forEach(link =>
    link.classList.toggle('active', link.dataset.target === fileId));
};
fileLinks.forEach(link =>
  link.addEventListener('click', () => showFile(link.dataset.target)));
const initialFile = location.hash.slice(1);
showFile(document.getElementById(initialFile) ? initialFile
  : (fileLinks[0] && fileLinks[0].dataset.target));
const setMode = mode => {
  document.body.classList.toggle('mode-diff', mode === 'diff');
  document.body.classList.toggle('mode-all', mode !== 'diff');
  document.querySelectorAll('.modes button').forEach(button =>
    button.classList.toggle('active', button.dataset.mode === mode));
  // The deep-linked or previously selected file may not exist in the new
  // mode; fall back to the first one that does.
  const selector = mode === 'diff' ? 'nav a.changed' : 'nav a';
  const activeLink = document.querySelector('nav a.active');
  if (!activeLink || !activeLink.matches(selector)) {
    const firstLink = document.querySelector(selector);
    if (firstLink) showFile(firstLink.dataset.target);
  }
};
document.querySelectorAll('.modes button').forEach(button =>
  button.addEventListener('click', () => setMode(button.dataset.mode)));
setMode(document.body.classList.contains('mode-diff') ? 'diff' : 'all');
</script>
</body>
</html>
`))
