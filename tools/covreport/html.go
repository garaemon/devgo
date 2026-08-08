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
type sourceReader func(rel string) ([]byte, error)

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
func renderHTML(w io.Writer, p profile, opts htmlOptions) error {
	if opts.Source == nil {
		opts.Source = func(rel string) ([]byte, error) { return os.ReadFile(rel) }
	}

	// Changed files that never appear in the profile still belong in the
	// report, so the path set is the union of both sides.
	paths := map[string]bool{}
	for f := range p {
		paths[f] = true
	}
	if opts.HaveDiff {
		for f := range opts.Added {
			paths[opts.Module+"/"+f] = true
		}
	}
	sorted := make([]string, 0, len(paths))
	for f := range paths {
		sorted = append(sorted, f)
	}
	sort.Strings(sorted)

	tallies := patchTallies(p, opts.Added, opts.Module)

	var files []htmlFile
	changed := 0
	for _, profPath := range sorted {
		rel := strings.TrimPrefix(profPath, opts.Module+"/")
		src, err := opts.Source(rel)
		if err != nil {
			// Sources can be missing when the profile came from another
			// commit; skip the file rather than failing the whole report.
			fmt.Fprintf(os.Stderr, "covreport: skipping %s: %v\n", rel, err)
			continue
		}
		added := opts.Added[rel]
		sort.Ints(added)
		added = dedupInts(added)

		f := annotateFile(rel, string(src), p[profPath], added, opts.Context)
		if f.Changed {
			changed++
			// Patch numbers come from the profile, not the annotated source,
			// so the header always matches the markdown report.
			t := tallies[rel]
			f.PatchCovered, f.PatchTotal = t.covered, t.total
			f.PatchPercent = t.percent()
		}
		f.PatchLabel = "n/a"
		if f.PatchTotal > 0 {
			f.PatchLabel = fmt.Sprintf("%.1f%%", f.PatchPercent)
		}
		files = append(files, f)
	}

	total := totalTally(p)
	data := htmlData{
		Module:       opts.Module,
		Commit:       opts.Commit,
		Percent:      total.percent(),
		Diff:         opts.HaveDiff,
		BodyClass:    "mode-all",
		RangeLabel:   rangeLabel(opts),
		PatchLabel:   "n/a",
		ChangedFiles: changed,
		Files:        files,
	}
	if opts.HaveDiff && changed > 0 {
		data.BodyClass = "mode-diff"
	}
	var patch tally
	for _, f := range files {
		patch.covered += f.PatchCovered
		patch.total += f.PatchTotal
	}
	if patch.total > 0 {
		data.PatchLabel = fmt.Sprintf("%.1f%% (%d/%d statements)",
			patch.percent(), patch.covered, patch.total)
	}
	return htmlTemplate.Execute(w, data)
}

// rangeLabel describes what the diff was taken against, for the header.
func rangeLabel(opts htmlOptions) string {
	switch {
	case opts.DiffBase != "":
		head := opts.DiffHead
		if head == "" {
			head = "working tree"
		}
		return opts.DiffBase + " … " + head
	case opts.DiffFile != "":
		return opts.DiffFile
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
	Text    string
	Added   bool
	Visible bool
	Gap     int
}

// region is an inclusive run of source lines.
type region struct{ start, end int }

// diffRegions returns the line runs visible in the diff view: every added line
// grown by context lines on each side, clamped to [1,totalLines] and merged
// when the expanded runs touch or overlap. added must be sorted and deduped.
func diffRegions(added []int, totalLines, context int) []region {
	if totalLines <= 0 || len(added) == 0 {
		return nil
	}
	if context < 0 {
		context = 0
	}
	var out []region
	for _, l := range added {
		if l < 1 || l > totalLines {
			continue // profile/source skew, or a line past EOF
		}
		s, e := l-context, l+context
		if s < 1 {
			s = 1
		}
		if e > totalLines {
			e = totalLines
		}
		// Merge on touching, not just overlapping, so no "0 lines" separator
		// is ever emitted between adjacent windows.
		if n := len(out); n > 0 && s <= out[n-1].end+1 {
			if e > out[n-1].end {
				out[n-1].end = e
			}
			continue
		}
		out = append(out, region{s, e})
	}
	return out
}

// annotateFile classifies each source line and, when added is non-empty, marks
// the diff-view context windows and inserts elision separators between them.
// Uncovered wins over covered when a line spans blocks of both kinds, so red
// always flags lines that still need a test.
func annotateFile(rel, src string, blocks []block, added []int, context int) htmlFile {
	srcLines := strings.Split(src, "\n")
	total := len(srcLines)

	classes := make([]string, total+1)
	var t tally
	for _, b := range blocks {
		t.add(b)
		class := "cov"
		if b.count == 0 {
			class = "uncov"
		}
		for l := b.startLine; l <= b.endLine && l < len(classes); l++ {
			if classes[l] != "uncov" {
				classes[l] = class
			}
		}
	}

	f := htmlFile{
		Path:    rel,
		ID:      strings.NewReplacer("/", "-", ".", "-").Replace(rel),
		Percent: t.percent(),
		Changed: len(added) > 0,
	}

	addedSet := map[int]bool{}
	visible := make([]bool, total+1)
	if !f.Changed {
		for i := range visible {
			visible[i] = true
		}
	} else {
		for _, l := range added {
			addedSet[l] = true
		}
		for _, r := range diffRegions(added, total, context) {
			for l := r.start; l <= r.end; l++ {
				visible[l] = true
			}
		}
	}

	// Hidden lines stay in the slice — they are what the all-files view
	// renders — and only the separator rows are new.
	hidden := 0
	for i, text := range srcLines {
		n := i + 1
		if !visible[n] {
			hidden++
			f.Lines = append(f.Lines, htmlLine{Num: n, Class: classes[n], Text: text})
			continue
		}
		if hidden > 0 {
			f.Lines = append(f.Lines, htmlLine{Gap: hidden})
			hidden = 0
		}
		f.Lines = append(f.Lines, htmlLine{
			Num: n, Class: classes[n], Text: text, Added: addedSet[n], Visible: true,
		})
	}
	if hidden > 0 {
		f.Lines = append(f.Lines, htmlLine{Gap: hidden})
	}
	return f
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
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0d1117; --fg: #e6edf3; --muted: #8d96a0;
    --panel: #161b22; --border: #30363d; --accent: #4493f8;
    --cov-bg: #12261e; --cov-edge: #2ea043;
    --uncov-bg: #2d1517; --uncov-edge: #f85149;
    --add-bg: #10203a; --add-edge: #4493f8; --gap-fg: #8d96a0;
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
pre span { display: block; padding: 0 16px 0 0; }
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
const links = document.querySelectorAll('nav a');
const show = id => {
  document.querySelectorAll('section').forEach(s =>
    s.classList.toggle('active', s.id === id));
  links.forEach(a =>
    a.classList.toggle('active', a.dataset.target === id));
};
links.forEach(a => a.addEventListener('click', () => show(a.dataset.target)));
const initial = location.hash.slice(1);
show(document.getElementById(initial) ? initial : (links[0] && links[0].dataset.target));
const setMode = m => {
  document.body.classList.toggle('mode-diff', m === 'diff');
  document.body.classList.toggle('mode-all', m !== 'diff');
  document.querySelectorAll('.modes button').forEach(b =>
    b.classList.toggle('active', b.dataset.mode === m));
  // The deep-linked or previously selected file may not exist in the new
  // mode; fall back to the first one that does.
  const sel = m === 'diff' ? 'nav a.changed' : 'nav a';
  const active = document.querySelector('nav a.active');
  if (!active || !active.matches(sel)) {
    const first = document.querySelector(sel);
    if (first) show(first.dataset.target);
  }
};
document.querySelectorAll('.modes button').forEach(b =>
  b.addEventListener('click', () => setMode(b.dataset.mode)));
setMode(document.body.classList.contains('mode-diff') ? 'diff' : 'all');
</script>
</body>
</html>
`))
