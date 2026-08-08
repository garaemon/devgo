package main

import (
	"go/scanner"
	"go/token"
	"html/template"
	"strings"
)

// Token classes used in the rendered HTML. Kept to one letter because every
// token in the report carries one, and none of them collide with the row
// classes (n, g, t, cov, uncov, add, off, gap).
const (
	classKeyword    = "k"
	classString     = "s"
	classNumber     = "m"
	classComment    = "c"
	classPredeclare = "b"
	classFuncName   = "f"
)

// predeclared are the identifiers in Go's universe block. They are not
// keywords, so the scanner reports them as plain identifiers.
var predeclared = map[string]bool{
	"any": true, "bool": true, "byte": true, "comparable": true,
	"complex64": true, "complex128": true, "error": true, "float32": true,
	"float64": true, "int": true, "int8": true, "int16": true, "int32": true,
	"int64": true, "rune": true, "string": true, "uint": true, "uint8": true,
	"uint16": true, "uint32": true, "uint64": true, "uintptr": true,

	"false": true, "iota": true, "nil": true, "true": true,

	"append": true, "cap": true, "clear": true, "close": true,
	"complex": true, "copy": true, "delete": true, "imag": true, "len": true,
	"make": true, "max": true, "min": true, "new": true, "panic": true,
	"print": true, "println": true, "real": true, "recover": true,
}

// tokenSpan is a half-open byte range of src that should be wrapped in class.
type tokenSpan struct {
	start, end int
	class      string
}

// highlightLines splits src into one HTML fragment per line, matching
// strings.Split(src, "\n") element for element. Go sources get their tokens
// wrapped in class spans; anything else — and any source the scanner chokes
// on — falls back to plain escaped text.
func highlightLines(rel, src string) []template.HTML {
	if !strings.HasSuffix(rel, ".go") {
		return plainLines(src)
	}
	spans, ok := goTokenSpans(src)
	if !ok {
		return plainLines(src)
	}
	return assembleLines(src, spans)
}

func plainLines(src string) []template.HTML {
	raw := strings.Split(src, "\n")
	out := make([]template.HTML, len(raw))
	for i, l := range raw {
		out[i] = template.HTML(template.HTMLEscapeString(l))
	}
	return out
}

// scanned is one token, with the source range it actually occupies.
type scanned struct {
	tok        token.Token
	lit        string
	start, end int
}

// goTokenSpans tokenizes src and returns the ranges worth colouring, in
// source order and never overlapping. It reports false when the scanner
// finds a syntax error, so a source that cannot be tokenized cleanly renders
// as plain text instead of being mis-coloured.
func goTokenSpans(src string) ([]tokenSpan, bool) {
	toks, ok := scanTokens(src)
	if !ok {
		return nil, false
	}
	names := funcNameIndexes(toks)

	spans := make([]tokenSpan, 0, len(toks))
	for i, t := range toks {
		class := ""
		switch {
		case t.tok == token.COMMENT:
			class = classComment
		case t.tok == token.STRING, t.tok == token.CHAR:
			class = classString
		case t.tok == token.INT, t.tok == token.FLOAT, t.tok == token.IMAG:
			class = classNumber
		case t.tok.IsKeyword():
			class = classKeyword
		case names[i]:
			class = classFuncName
		case t.tok == token.IDENT && predeclared[t.lit]:
			class = classPredeclare
		}
		if class == "" {
			continue
		}
		spans = append(spans, tokenSpan{t.start, t.end, class})
	}
	return spans, true
}

func scanTokens(src string) ([]scanned, bool) {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	bad := false
	var s scanner.Scanner
	s.Init(file, []byte(src), func(token.Position, string) { bad = true }, scanner.ScanComments)

	var toks []scanned
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF || bad {
			break
		}
		// Semicolons the scanner inserts at end of line are not in the source.
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}

		text := lit
		if text == "" {
			text = tok.String()
		}
		start := file.Offset(pos)
		end := start + len(text)
		if tok == token.COMMENT {
			// Comment literals have carriage returns stripped, so their length
			// can disagree with the source; measure the delimiters instead.
			end = commentEnd(src, start)
		}
		if start < 0 || start >= len(src) || end <= start {
			continue
		}
		if end > len(src) {
			end = len(src)
		}
		// A literal whose reported length undershot the source would otherwise
		// leave this token's range running into the previous one.
		if n := len(toks); n > 0 && toks[n-1].end > start {
			toks[n-1].end = start
		}
		toks = append(toks, scanned{tok, lit, start, end})
	}
	if bad {
		return nil, false
	}
	return toks, true
}

// funcNameIndexes marks the identifiers that name a declared function or
// method. A declaration's name is the identifier after `func` — or after a
// parenthesized receiver — and is always followed by the parameter list, or
// by a type parameter list. That last check is what keeps the return type of
// a func *type* (`var f func(int) int`) from being mistaken for a name.
func funcNameIndexes(toks []scanned) map[int]bool {
	names := map[int]bool{}
	for i, t := range toks {
		if t.tok != token.FUNC {
			continue
		}
		j := i + 1
		if j < len(toks) && toks[j].tok == token.LPAREN {
			j = skipParens(toks, j)
		}
		if j+1 >= len(toks) || toks[j].tok != token.IDENT {
			continue
		}
		if next := toks[j+1].tok; next == token.LPAREN || next == token.LBRACK {
			names[j] = true
		}
	}
	return names
}

// skipParens returns the index just past the parenthesis group opening at i.
func skipParens(toks []scanned, i int) int {
	depth := 0
	for ; i < len(toks); i++ {
		switch toks[i].tok {
		case token.LPAREN:
			depth++
		case token.RPAREN:
			if depth--; depth == 0 {
				return i + 1
			}
		}
	}
	return i
}

// commentEnd returns the offset just past the comment starting at start.
func commentEnd(src string, start int) int {
	if strings.HasPrefix(src[start:], "//") {
		if i := strings.IndexByte(src[start:], '\n'); i >= 0 {
			return start + i
		}
		return len(src)
	}
	if i := strings.Index(src[start+2:], "*/"); i >= 0 {
		return start + 2 + i + 2
	}
	return len(src)
}

// assembleLines walks src once, emitting escaped text and wrapping the spans,
// and cuts a new line at every newline. Tokens that span lines — raw strings
// and block comments — are wrapped separately on each line they cover, since
// every line is its own row in the report.
func assembleLines(src string, spans []tokenSpan) []template.HTML {
	out := make([]template.HTML, 0, strings.Count(src, "\n")+1)
	var b strings.Builder

	emit := func(text, class string) {
		for {
			chunk, rest := text, ""
			cut := false
			if i := strings.IndexByte(text, '\n'); i >= 0 {
				chunk, rest, cut = text[:i], text[i+1:], true
			}
			if chunk != "" {
				if class != "" {
					b.WriteString(`<span class="` + class + `">`)
				}
				b.WriteString(template.HTMLEscapeString(chunk))
				if class != "" {
					b.WriteString(`</span>`)
				}
			}
			if !cut {
				return
			}
			out = append(out, template.HTML(b.String()))
			b.Reset()
			text = rest
		}
	}

	off := 0
	for _, sp := range spans {
		if sp.start < off {
			continue
		}
		emit(src[off:sp.start], "")
		emit(src[sp.start:sp.end], sp.class)
		off = sp.end
	}
	emit(src[off:], "")
	return append(out, template.HTML(b.String()))
}
