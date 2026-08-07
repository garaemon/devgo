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

// renderHTML writes a self-contained, readable coverage browser: a sidebar
// listing every file with its coverage percentage and a main pane showing
// the annotated source. It replaces the hard-to-read default theme of
// `go tool cover -html` and reads sources relative to the working
// directory, so it must run from the repository root.
func renderHTML(w io.Writer, p profile, module, commit string, source sourceReader) error {
	if source == nil {
		source = func(rel string) ([]byte, error) { return os.ReadFile(rel) }
	}

	var files []htmlFile
	paths := make([]string, 0, len(p))
	for f := range p {
		paths = append(paths, f)
	}
	sort.Strings(paths)

	for _, profPath := range paths {
		rel := strings.TrimPrefix(profPath, module+"/")
		src, err := source(rel)
		if err != nil {
			// Sources can be missing when the profile came from another
			// commit; skip the file rather than failing the whole report.
			fmt.Fprintf(os.Stderr, "covreport: skipping %s: %v\n", rel, err)
			continue
		}
		files = append(files, annotateFile(rel, string(src), p[profPath]))
	}

	total := totalTally(p)
	data := htmlData{
		Module:  module,
		Commit:  commit,
		Percent: total.percent(),
		Files:   files,
	}
	return htmlTemplate.Execute(w, data)
}

type htmlData struct {
	Module  string
	Commit  string
	Percent float64
	Files   []htmlFile
}

type htmlFile struct {
	Path    string
	ID      string
	Percent float64
	Lines   []htmlLine
}

type htmlLine struct {
	Num   int
	Class string // "", "cov", or "uncov"
	Text  string
}

// annotateFile classifies each source line: uncovered wins over covered
// when a line spans blocks of both kinds, so red always flags lines that
// still need a test.
func annotateFile(rel, src string, blocks []block) htmlFile {
	srcLines := strings.Split(src, "\n")
	classes := make([]string, len(srcLines)+1)
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
	}
	for i, text := range srcLines {
		f.Lines = append(f.Lines, htmlLine{Num: i + 1, Class: classes[i+1], Text: text})
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
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0d1117; --fg: #e6edf3; --muted: #8d96a0;
    --panel: #161b22; --border: #30363d; --accent: #4493f8;
    --cov-bg: #12261e; --cov-edge: #2ea043;
    --uncov-bg: #2d1517; --uncov-edge: #f85149;
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
.legend { margin-left: auto; font-size: 12px; color: var(--muted); }
.legend .cov, .legend .uncov { padding: 1px 8px; border-radius: 3px; }
.legend .cov { background: var(--cov-bg); }
.legend .uncov { background: var(--uncov-bg); }
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
/* Tabs are measured from the start of the line box, so the code must live
   in its own inline-block for indentation to survive the number prefix. */
pre .t { display: inline-block; white-space: pre; vertical-align: top; }
pre .cov { background: var(--cov-bg); box-shadow: inset 3px 0 var(--cov-edge); }
pre .uncov { background: var(--uncov-bg); box-shadow: inset 3px 0 var(--uncov-edge); }
</style>
</head>
<body>
<header>
  <h1>{{.Module}}</h1>
  <span class="total">{{printf "%.1f" .Percent}}%</span>
  {{if .Commit}}<span class="meta">commit {{.Commit}}</span>{{end}}
  <span class="legend">
    <span class="cov">covered</span> <span class="uncov">not covered</span>
    plain: not tracked
  </span>
</header>
<main>
<nav>
{{range .Files}}<a href="#{{.ID}}" data-target="{{.ID}}">
  <span>{{.Path}}</span><span class="pct">{{printf "%.1f" .Percent}}%</span>
</a>
{{end}}</nav>
{{range .Files}}<section id="{{.ID}}">
<h2>{{.Path}} — {{printf "%.1f" .Percent}}%</h2>
<pre>{{range .Lines}}<span{{if .Class}} class="{{.Class}}"{{end}}><span class="n">{{.Num}}</span><span class="t">{{.Text}}</span></span>{{end}}</pre>
</section>
{{end}}</main>
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
</script>
</body>
</html>
`))
